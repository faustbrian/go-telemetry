// Package telemetryservice adapts the telemetry runtime to service lifecycle.
//
// It is the target-oriented successor to telemetryservice and preserves that
// package's released type, sentinel, lifecycle, and error identities.
package telemetryservice

import legacy "github.com/faustbrian/go-telemetry/telemetryservice"

var ErrInvalidOptions = legacy.ErrInvalidOptions

type FailurePolicy = legacy.FailurePolicy

const (
	FailureRequired   = legacy.FailureRequired
	FailureBestEffort = legacy.FailureBestEffort
)

type Options = legacy.Options
type OptionsError = legacy.OptionsError
type InitializationError = legacy.InitializationError
type Adapter = legacy.Adapter

func New(options Options) (*Adapter, error) {
	return legacy.New(options)
}
