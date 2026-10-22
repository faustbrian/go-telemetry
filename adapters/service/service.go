// Package telemetryservice adapts an explicit telemetry runtime to the service
// lifecycle.
//
// The adapter constructs and owns its runtime. Callers select global provider
// registration through telemetry.Config and choose whether initialization is
// required or best effort. Shutdown flushes and closes the runtime once using
// the bounded policy implemented by telemetry.Runtime. The adapter performs no
// retries and exposes the concrete runtime without hiding provider APIs. Hooks
// require one ordered service lifecycle; mutexes protect accessors, not repeated
// startup/restart. Global provider policy and application drain are caller-owned.
package telemetryservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/faustbrian/go-service/v3"
	"github.com/faustbrian/go-telemetry/v4"
)

// ErrInvalidOptions identifies invalid adapter construction.
var ErrInvalidOptions = errors.New("invalid telemetry service options")

// FailurePolicy selects how initialization failure affects service startup.
type FailurePolicy string

const (
	// FailureRequired prevents service startup when telemetry initialization
	// fails.
	FailureRequired FailurePolicy = "required"
	// FailureBestEffort permits service startup while retaining the
	// initialization failure for diagnostics.
	FailureBestEffort FailurePolicy = "best-effort"
)

// Options configures one telemetry lifecycle adapter. Config maps/slices and
// option collaborators are not deep-copied; do not mutate them during startup.
type Options struct {
	// Name is the caller-supplied component label. New rejects blank names,
	// not sensitive content; the service receives this string unchanged.
	Name string
	// Config defines the runtime and explicitly selects global registration.
	Config telemetry.Config
	// RuntimeOptions are slice-copied; attached exporters follow runtime
	// ownership, while unused/overridden exporters remain caller-owned.
	RuntimeOptions []telemetry.Option
	// Failure explicitly selects required or best-effort initialization.
	Failure FailurePolicy
}

// OptionsError identifies one rejected option.
type OptionsError struct {
	// Field identifies the rejected option.
	Field string
	// Reason describes the failure category; caller-constructed values are printed verbatim.
	Reason string
}

// Error formats Field and Reason verbatim with the option sentinel. New supplies
// categorical values; manually constructed OptionsError values are not redacted.
func (err *OptionsError) Error() string {
	return fmt.Sprintf("%s: %s: %v", err.Field, err.Reason, ErrInvalidOptions)
}

// Unwrap exposes the stable option classification.
func (err *OptionsError) Unwrap() error { return ErrInvalidOptions }

// InitializationError preserves the runtime construction failure without
// formatting the potentially sensitive cause.
type InitializationError struct {
	// Cause is the runtime construction failure, accessible through Unwrap and
	// not sanitized for callers inspecting or formatting it directly.
	Cause error
}

// Error returns a secret-safe startup diagnostic.
func (*InitializationError) Error() string {
	return "telemetry service initialization failed"
}

// Unwrap preserves the runtime construction failure.
func (err *InitializationError) Unwrap() error { return err.Cause }

// Adapter owns one telemetry runtime and its lifecycle state.
type Adapter struct {
	name           string
	config         telemetry.Config
	runtimeOptions []telemetry.Option
	failure        FailurePolicy

	mu      sync.RWMutex
	runtime *telemetry.Runtime
	initErr error

	stopOnce sync.Once
	stopErr  error
}

// New validates Name/Failure and constructs an inert adapter without validating
// the runtime Config or doing I/O. It copies the RuntimeOptions slice, not the
// options or Config nested data. Service must call Start then Stop once in order;
// this adapter does not support repeated Start, Stop-before-Start or restart.
func New(options Options) (*Adapter, error) {
	if strings.TrimSpace(options.Name) == "" {
		return nil, &OptionsError{Field: "Name", Reason: "must not be blank"}
	}
	if options.Failure != FailureRequired &&
		options.Failure != FailureBestEffort {
		return nil, &OptionsError{
			Field: "Failure", Reason: "must be required or best-effort",
		}
	}

	return &Adapter{
		name:           options.Name,
		config:         options.Config,
		runtimeOptions: append([]telemetry.Option(nil), options.RuntimeOptions...),
		failure:        options.Failure,
	}, nil
}

// Component returns fresh hooks into this same adapter, not a new lifecycle.
// Start initializes once under the service lifecycle contract; Stop clears the
// active pointer before the one runtime Shutdown and retains its first result.
// A pre-canceled Stop consumes the adapter stop attempt even if Runtime rejects
// shutdown before starting. Drain later business components before invoking it.
func (adapter *Adapter) Component() service.Component {
	return service.Component{
		Name:  adapter.name,
		Start: adapter.start,
		Stop:  adapter.stop,
	}
}

// Runtime returns the borrowed runtime pointer while the adapter retains it.
// A true result is not a health/reachability probe and can become stale as Stop
// begins; callers must coordinate use with the owning service lifecycle.
func (adapter *Adapter) Runtime() (*telemetry.Runtime, bool) {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()

	return adapter.runtime, adapter.runtime != nil
}

// InitializationError returns the retained initialization failure for either
// required or best-effort policy, or nil. The wrapper hides cause text in Error
// but exposes the original cause through Unwrap; it is not a redacted cause tree.
func (adapter *Adapter) InitializationError() error {
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()

	return adapter.initErr
}

func (adapter *Adapter) start(ctx context.Context) error {
	runtime, err := telemetry.Init(
		ctx,
		adapter.config,
		adapter.runtimeOptions...,
	)
	if err != nil {
		initializationError := &InitializationError{Cause: err}
		adapter.mu.Lock()
		adapter.initErr = initializationError
		adapter.mu.Unlock()
		if adapter.failure == FailureBestEffort {
			return nil
		}

		return initializationError
	}

	adapter.mu.Lock()
	adapter.runtime = runtime
	adapter.mu.Unlock()

	return nil
}

func (adapter *Adapter) stop(ctx context.Context) error {
	adapter.stopOnce.Do(func() {
		adapter.mu.Lock()
		runtime := adapter.runtime
		adapter.runtime = nil
		adapter.mu.Unlock()
		if runtime != nil {
			adapter.stopErr = runtime.Shutdown(ctx)
		}
	})

	return adapter.stopErr
}
