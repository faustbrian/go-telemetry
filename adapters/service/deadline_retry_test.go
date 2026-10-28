package telemetryservice_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/faustbrian/go-telemetry/v4"
	telemetryservice "github.com/faustbrian/go-telemetry/v4/adapters/service"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRetainedShutdownConservesDeadlines(t *testing.T) {
	for _, test := range []struct {
		name               string
		caller, configured time.Duration
		cancelDuring       bool
	}{
		{name: "caller-shorter", caller: 4 * time.Second, configured: 12 * time.Second},
		{name: "configuration-shorter", caller: 20 * time.Second, configured: 2 * time.Second},
		{name: "canceled-during-export", caller: 20 * time.Second, configured: 12 * time.Second, cancelDuring: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				exporter := &deadlineRetryMetricExporter{entered: make(chan struct{})}
				config := localConfig()
				config.Metrics.Enabled = true
				config.Metrics.ExportInterval = time.Hour
				config.Metrics.ExportTimeout = time.Minute
				config.ShutdownTimeout = test.configured
				adapter, err := telemetryservice.New(telemetryservice.Options{
					Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
					RetainRejectedShutdown: true,
					RuntimeOptions:         []telemetry.Option{telemetry.WithMetricExporter(exporter)},
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := adapter.Component().Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				owned, _ := adapter.Runtime()
				defer func() { _ = owned.Shutdown(context.Background()) }()
				rejected, reject := context.WithCancel(context.Background())
				reject()
				if err := adapter.Component().Stop(rejected); !errors.Is(err, context.Canceled) {
					t.Fatalf("rejected shutdown = %v", err)
				}
				start := time.Now()
				ceiling := start.Add(min(test.caller, test.configured))
				retry, cancel := context.WithTimeout(context.Background(), test.caller)
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- adapter.Component().Stop(retry) }()
				want := context.DeadlineExceeded
				if test.cancelDuring {
					<-exporter.entered
					cancel()
					want = context.Canceled
				}
				result := <-done
				if !errors.Is(result, want) {
					t.Fatalf("admitted shutdown = %v, want %v", result, want)
				}
				if time.Now().After(ceiling) {
					t.Fatal("shutdown exceeded the original caller/configuration ceiling")
				}
				exporter.mu.Lock()
				deadlines := append([]time.Time(nil), exporter.deadlines...)
				exportCanceledAt := exporter.exportCanceledAt
				exporter.mu.Unlock()
				for _, deadline := range deadlines {
					if deadline.IsZero() || deadline.After(ceiling) {
						t.Fatalf("exporter received deadline %v beyond %v", deadline, ceiling)
					}
				}
				if len(deadlines) == 0 || exporter.stops.Load() != 1 {
					t.Fatal("cooperative exporter was not canceled and closed")
				}
				if exportCanceledAt.IsZero() || exportCanceledAt.After(ceiling) {
					t.Fatal("periodic export was not canceled within the shutdown ceiling")
				}
				if _, active := adapter.Runtime(); active {
					t.Fatal("admitted timeout remained retryable")
				}
				if err := adapter.Component().Stop(context.Background()); !errors.Is(err, result) || exporter.stops.Load() != 1 {
					t.Fatalf("admitted result replayed or changed: %v", err)
				}
			})
		})
	}
}

type deadlineRetryMetricExporter struct {
	retryMetricExporter
	entered          chan struct{}
	once             sync.Once
	mu               sync.Mutex
	deadlines        []time.Time
	exportCanceledAt time.Time
}

func (e *deadlineRetryMetricExporter) Export(ctx context.Context, _ *metricdata.ResourceMetrics) error {
	e.once.Do(func() { close(e.entered) })
	<-ctx.Done()
	e.mu.Lock()
	e.exportCanceledAt = time.Now()
	e.mu.Unlock()
	return ctx.Err()
}

func (e *deadlineRetryMetricExporter) Shutdown(ctx context.Context) error {
	deadline, _ := ctx.Deadline()
	e.mu.Lock()
	e.deadlines = append(e.deadlines, deadline)
	e.mu.Unlock()
	return e.retryMetricExporter.Shutdown(ctx)
}
