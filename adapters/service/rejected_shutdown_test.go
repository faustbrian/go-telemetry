package telemetryservice_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/faustbrian/go-telemetry/v4"
	telemetryservice "github.com/faustbrian/go-telemetry/v4/adapters/service"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestRejectedShutdownRetainsRuntimeForBoundedRetry(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "canceled"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			spans := &retrySpanExporter{}
			metrics := &retryMetricExporter{}
			config := localConfig()
			config.Traces.Enabled = true
			config.Metrics.Enabled = true
			config.Metrics.ExportInterval = time.Hour
			config.RegisterGlobal = true
			previousTrace, previousMetric, previousPropagation := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
			adapter, err := telemetryservice.New(telemetryservice.Options{
				Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
				RetainRejectedShutdown: true,
				RuntimeOptions: []telemetry.Option{
					telemetry.WithTraceExporter(spans), telemetry.WithMetricExporter(metrics),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			component := adapter.Component()
			if err := component.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			owned, _ := adapter.Runtime()
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = owned.Shutdown(ctx)
			})
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if expired {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			if err := component.Stop(ctx); !errors.Is(err, want) {
				t.Fatalf("rejected stop = %v, want %v", err, want)
			}
			if current, active := adapter.Runtime(); !active || current != owned {
				t.Fatal("rejected stop discarded the unclosed runtime")
			}
			if spans.stops.Load() != 0 || metrics.stops.Load() != 0 {
				t.Fatal("rejected stop unexpectedly consumed provider shutdown")
			}
			if otel.GetTracerProvider() != owned.TracerProvider() || otel.GetMeterProvider() != owned.MeterProvider() || otel.GetTextMapPropagator() != owned.Propagator() {
				t.Fatal("rejected stop released owned globals")
			}
			retry, retryCancel := context.WithTimeout(context.Background(), time.Second)
			defer retryCancel()
			if err := component.Stop(retry); err != nil {
				t.Fatalf("bounded retry = %v", err)
			}
			if _, active := adapter.Runtime(); active {
				t.Fatal("completed stop retained its runtime")
			}
			if spans.stops.Load() != 1 || metrics.stops.Load() != 1 {
				t.Fatalf("provider shutdowns = trace %d metric %d", spans.stops.Load(), metrics.stops.Load())
			}
			if otel.GetTracerProvider() != previousTrace || otel.GetMeterProvider() != previousMetric || otel.GetTextMapPropagator() != previousPropagation {
				t.Fatal("completed stop did not restore previous globals")
			}
			if err := component.Stop(ctx); err != nil {
				t.Fatalf("completed stop did not return the shared result: %v", err)
			}
		})
	}
}

func TestLegacyStopStillConsumesRejectedAttempt(t *testing.T) {
	exporter := &retrySpanExporter{}
	config := localConfig()
	config.Traces.Enabled = true
	adapter, err := telemetryservice.New(telemetryservice.Options{
		Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
		RuntimeOptions: []telemetry.Option{telemetry.WithTraceExporter(exporter)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Component().Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	owned, _ := adapter.Runtime()
	t.Cleanup(func() { _ = owned.Shutdown(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{ctx, context.Background()} {
		if err := adapter.Component().Stop(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("legacy rejection result = %v", err)
		}
	}
	if _, active := adapter.Runtime(); active || exporter.stops.Load() != 0 {
		t.Fatal("legacy adapter rejection behavior changed")
	}
}

func TestRetainedStopJoinsBorrowedAndAdapterShutdown(t *testing.T) {
	for _, borrowerFirst := range []bool{false, true} {
		name := "adapter-first"
		if borrowerFirst {
			name = "borrower-first"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				exporter := &barrierRetrySpanExporter{entered: make(chan struct{}), release: make(chan struct{})}
				config := localConfig()
				config.Traces.Enabled = true
				config.ShutdownTimeout = time.Hour
				adapter, err := telemetryservice.New(telemetryservice.Options{
					Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
					RetainRejectedShutdown: true,
					RuntimeOptions:         []telemetry.Option{telemetry.WithTraceExporter(exporter)},
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := adapter.Component().Start(context.Background()); err != nil {
					t.Fatal(err)
				}
				owned, _ := adapter.Runtime()
				first := adapter.Component().Stop
				if borrowerFirst {
					first = owned.Shutdown
				}
				firstDone := make(chan error, 1)
				go func() { firstDone <- first(context.Background()) }()
				<-exporter.entered
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				joined := make(chan error, 8)
				for range cap(joined) {
					go func() { joined <- adapter.Component().Stop(ctx) }()
				}
				synctest.Wait()
				premature := len(joined) != 0
				current, active := adapter.Runtime()
				close(exporter.release)
				if err := <-firstDone; !errors.Is(err, context.Canceled) {
					t.Fatalf("started shutdown lost exporter error: %v", err)
				}
				for range cap(joined) {
					if err := <-joined; !errors.Is(err, context.Canceled) {
						t.Errorf("joiner lost terminal outcome: %v", err)
					}
				}
				if premature || !active || current != owned {
					t.Fatal("joiner returned or discarded ownership before admitted shutdown returned")
				}
				if _, active := adapter.Runtime(); active {
					t.Fatal("terminal error retained a retryable runtime")
				}
				if err := adapter.Component().Stop(context.Background()); !errors.Is(err, context.Canceled) {
					t.Fatalf("terminal error was replaced: %v", err)
				}
				if exporter.stops.Load() != 1 {
					t.Fatalf("exporter shutdown replayed %d times", exporter.stops.Load())
				}
			})
		})
	}
}

func TestRetainedStopAdoptsCompletedBorrowedShutdown(t *testing.T) {
	exporter := &retrySpanExporter{}
	config := localConfig()
	config.Traces.Enabled = true
	adapter, err := telemetryservice.New(telemetryservice.Options{
		Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
		RetainRejectedShutdown: true,
		RuntimeOptions:         []telemetry.Option{telemetry.WithTraceExporter(exporter)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Component().Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	owned, _ := adapter.Runtime()
	if err := owned.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapter.Component().Stop(ctx); err != nil {
		t.Fatalf("completed borrowed shutdown was misclassified as rejected: %v", err)
	}
	if _, active := adapter.Runtime(); active || exporter.stops.Load() != 1 {
		t.Fatal("completed borrowed shutdown was retained or replayed")
	}
}

func TestRetainedStopPreservesFailedInitializationPolicy(t *testing.T) {
	for _, policy := range []telemetryservice.FailurePolicy{telemetryservice.FailureRequired, telemetryservice.FailureBestEffort} {
		t.Run(string(policy), func(t *testing.T) {
			config := localConfig()
			config.ShutdownTimeout = -time.Second
			adapter, err := telemetryservice.New(telemetryservice.Options{
				Name: "telemetry", Config: config, Failure: policy, RetainRejectedShutdown: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = adapter.Component().Start(context.Background())
			if (err != nil) != (policy == telemetryservice.FailureRequired) || adapter.InitializationError() == nil {
				t.Fatalf("startup policy changed: %v", err)
			}
			if err := adapter.Component().Stop(context.Background()); err != nil {
				t.Fatalf("stop after failed initialization = %v", err)
			}
			if _, active := adapter.Runtime(); active {
				t.Fatal("failed initialization retained a runtime")
			}
		})
	}
}

type barrierRetrySpanExporter struct {
	entered, release chan struct{}
	stops            atomic.Int32
}

func (*barrierRetrySpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return nil
}
func (e *barrierRetrySpanExporter) Shutdown(context.Context) error {
	e.stops.Add(1)
	close(e.entered)
	<-e.release
	return context.Canceled
}

type retrySpanExporter struct{ stops atomic.Int32 }

func (*retrySpanExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (e *retrySpanExporter) Shutdown(context.Context) error                           { e.stops.Add(1); return nil }

type retryMetricExporter struct{ stops atomic.Int32 }

func (*retryMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}
func (*retryMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}
func (*retryMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error { return nil }
func (*retryMetricExporter) ForceFlush(context.Context) error                          { return nil }
func (e *retryMetricExporter) Shutdown(context.Context) error                          { e.stops.Add(1); return nil }
