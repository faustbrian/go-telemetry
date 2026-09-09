// Package telemetrypostgres provides privacy-preserving pgx instrumentation.
//
// It is the target-oriented successor to instrumentation/gopostgres and
// preserves that package's released type and instrumentation identities.
package telemetrypostgres

import (
	"context"

	gopostgres "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"
)

type Config = gopostgres.Config
type Tracer = gopostgres.Tracer

func New(config Config) (*Tracer, error) {
	return gopostgres.New(config)
}

func ContextWithOperation(ctx context.Context, operation string) context.Context {
	return gopostgres.ContextWithOperation(ctx, operation)
}
