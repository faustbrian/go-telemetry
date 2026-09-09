// Package telemetrycache provides dependency-neutral cache instrumentation.
//
// It is the target-oriented successor to instrumentation/gocache and preserves
// that package's released type and instrumentation identities.
package telemetrycache

import gocache "github.com/faustbrian/go-telemetry/instrumentation/gocache"

type Operation = gocache.Operation

const (
	OperationGet    = gocache.OperationGet
	OperationSet    = gocache.OperationSet
	OperationDelete = gocache.OperationDelete
	OperationLoad   = gocache.OperationLoad
	OperationOther  = gocache.OperationOther
)

type Outcome = gocache.Outcome

const (
	OutcomeSuccess = gocache.OutcomeSuccess
	OutcomeHit     = gocache.OutcomeHit
	OutcomeMiss    = gocache.OutcomeMiss
	OutcomeStale   = gocache.OutcomeStale
	OutcomeError   = gocache.OutcomeError
	OutcomeOther   = gocache.OutcomeOther
)

type Config = gocache.Config
type EndFunc = gocache.EndFunc
type Instrumenter = gocache.Instrumenter

func New(config Config) (*Instrumenter, error) {
	return gocache.New(config)
}
