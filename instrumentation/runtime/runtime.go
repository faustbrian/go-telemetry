// Package telemetryruntime provides explicit Go runtime instrumentation.
//
// It is the target-oriented successor to instrumentation/goruntime and
// preserves that package's released type and instrumentation identity.
package telemetryruntime

import (
	goruntime "github.com/faustbrian/go-telemetry/instrumentation/goruntime"
	"go.opentelemetry.io/otel/metric"
)

type Instrumenter = goruntime.Instrumenter

func New(provider metric.MeterProvider) (*Instrumenter, error) {
	return goruntime.New(provider)
}
