// Package goruntime exports bounded Go runtime metrics without starting a
// background collector or registering global providers.
package goruntime

import (
	"context"
	"errors"
	"math"
	"runtime"
	"sync"

	"go.opentelemetry.io/otel/metric"
)

const scopeName = "github.com/faustbrian/go-telemetry/instrumentation/goruntime"

// Instrumenter owns the registered Go runtime callback.
type Instrumenter struct {
	registration metric.Registration
	closeOnce    sync.Once
	closeErr     error
}

// New registers observable Go heap, allocation, goroutine, GC-cycle, and
// cumulative GC-pause instruments on provider. The caller owns the returned
// registration and must close it before shutting down the provider.
func New(provider metric.MeterProvider) (*Instrumenter, error) {
	if provider == nil {
		return nil, errors.New("go runtime meter provider is required")
	}
	meter := provider.Meter(scopeName)
	heap, err := meter.Int64ObservableGauge("go.memory.heap.used", metric.WithUnit("By"))
	if err != nil {
		return nil, err
	}
	allocated, err := meter.Int64ObservableCounter("go.memory.allocated", metric.WithUnit("By"))
	if err != nil {
		return nil, err
	}
	goroutines, err := meter.Int64ObservableGauge("go.goroutine.count", metric.WithUnit("{goroutine}"))
	if err != nil {
		return nil, err
	}
	gcCycles, err := meter.Int64ObservableCounter("go.gc.cycles", metric.WithUnit("{cycle}"))
	if err != nil {
		return nil, err
	}
	gcPause, err := meter.Float64ObservableCounter("go.gc.pause.time", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	registration, err := meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			observer.ObserveInt64(heap, boundedUint64(stats.HeapAlloc))
			observer.ObserveInt64(allocated, boundedUint64(stats.TotalAlloc))
			observer.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))
			observer.ObserveInt64(gcCycles, int64(stats.NumGC))
			observer.ObserveFloat64(gcPause, float64(stats.PauseTotalNs)/1e9)
			return nil
		},
		heap,
		allocated,
		goroutines,
		gcCycles,
		gcPause,
	)
	if err != nil {
		return nil, err
	}
	return &Instrumenter{registration: registration}, nil
}

// Close unregisters the runtime callback. It is concurrency-safe and
// idempotent; subsequent calls return the first unregister result.
func (instrumenter *Instrumenter) Close() error {
	if instrumenter == nil || instrumenter.registration == nil {
		return nil
	}
	instrumenter.closeOnce.Do(func() {
		instrumenter.closeErr = instrumenter.registration.Unregister()
	})
	return instrumenter.closeErr
}

func boundedUint64(value uint64) int64 {
	if value>>63 != 0 {
		return math.MaxInt64
	}
	return int64(value & uint64(math.MaxInt64))
}
