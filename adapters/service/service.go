// Package telemetryservice adapts the telemetry runtime to service lifecycle.
//
// It is the target-oriented successor to telemetryservice and preserves that
// package's released type, sentinel, lifecycle, and error identities.
package telemetryservice

import legacy "github.com/faustbrian/go-telemetry/telemetryservice"

// ErrInvalidOptions marks invalid service adapter options.
var ErrInvalidOptions = legacy.ErrInvalidOptions

// FailurePolicy controls whether initialization failures stop the service.
type FailurePolicy = legacy.FailurePolicy

const (
	// FailureRequired makes initialization failures terminal.
	FailureRequired = legacy.FailureRequired
	// FailureBestEffort allows the service to continue without telemetry.
	FailureBestEffort = legacy.FailureBestEffort
)

// Options configures the service lifecycle adapter.
type Options = legacy.Options

// OptionsError reports invalid adapter options.
type OptionsError = legacy.OptionsError

// InitializationError reports telemetry initialization failure.
type InitializationError = legacy.InitializationError

// Adapter binds telemetry runtime ownership to service lifecycle hooks.
type Adapter = legacy.Adapter

// New constructs a service lifecycle adapter.
func New(options Options) (*Adapter, error) {
	return legacy.New(options)
}
