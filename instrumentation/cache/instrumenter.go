// Package telemetrycache provides dependency-neutral cache instrumentation.
//
// It is the target-oriented successor to instrumentation/gocache and preserves
// that package's released type and instrumentation identities.
package telemetrycache

import gocache "github.com/faustbrian/go-telemetry/instrumentation/gocache"

// Operation is a bounded cache operation label.
type Operation = gocache.Operation

const (
	// OperationGet represents a cache lookup.
	OperationGet = gocache.OperationGet
	// OperationSet represents a cache write.
	OperationSet = gocache.OperationSet
	// OperationDelete represents cache invalidation.
	OperationDelete = gocache.OperationDelete
	// OperationLoad represents cache-aside loading.
	OperationLoad = gocache.OperationLoad
	// OperationOther collapses unknown operations.
	OperationOther = gocache.OperationOther
)

// Outcome is a bounded cache result label.
type Outcome = gocache.Outcome

const (
	// OutcomeSuccess represents a successful mutation.
	OutcomeSuccess = gocache.OutcomeSuccess
	// OutcomeHit represents a fresh cache hit.
	OutcomeHit = gocache.OutcomeHit
	// OutcomeMiss represents a cache miss.
	OutcomeMiss = gocache.OutcomeMiss
	// OutcomeStale represents a stale cache hit.
	OutcomeStale = gocache.OutcomeStale
	// OutcomeError represents a failed cache operation.
	OutcomeError = gocache.OutcomeError
	// OutcomeOther collapses unknown outcomes.
	OutcomeOther = gocache.OutcomeOther
)

// Config selects standard OpenTelemetry providers.
type Config = gocache.Config

// EndFunc completes one cache observation at most once.
type EndFunc = gocache.EndFunc

// Instrumenter creates bounded cache spans and metrics.
type Instrumenter = gocache.Instrumenter

// New constructs a dependency-neutral cache instrumenter.
func New(config Config) (*Instrumenter, error) {
	return gocache.New(config)
}
