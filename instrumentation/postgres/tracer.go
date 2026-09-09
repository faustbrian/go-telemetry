// Package telemetrypostgres provides privacy-preserving pgx instrumentation.
//
// It is the target-oriented successor to instrumentation/gopostgres and
// preserves that package's released type and instrumentation identities.
package telemetrypostgres

import (
	"context"

	gopostgres "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"
)

// Config defines the finite set of query names telemetry may record.
type Config = gopostgres.Config

// Tracer implements pgx tracing with bounded attributes.
type Tracer = gopostgres.Tracer

// New constructs a pgx query tracer.
func New(config Config) (*Tracer, error) {
	return gopostgres.New(config)
}

// ContextWithOperation associates a trusted static query name with ctx.
func ContextWithOperation(ctx context.Context, operation string) context.Context {
	return gopostgres.ContextWithOperation(ctx, operation)
}
