package telemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type timeoutMetricExporter struct{ closed atomic.Bool }

func (*timeoutMetricExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (*timeoutMetricExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (*timeoutMetricExporter) Export(ctx context.Context, _ *metricdata.ResourceMetrics) error {
	<-ctx.Done()
	return ctx.Err()
}

func (*timeoutMetricExporter) ForceFlush(context.Context) error { return nil }

func (e *timeoutMetricExporter) Shutdown(context.Context) error {
	e.closed.Store(true)
	return nil
}

func TestShutdownMetricTimeoutDoesNotSkipTraceCleanup(t *testing.T) {
	for _, callerDeadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured-budget", true: "caller-budget"}[callerDeadline], func(t *testing.T) {
			metrics := &timeoutMetricExporter{}
			traces := &recordingSpanExporter{}
			config := enabledConfig("shutdown-budget", "test")
			config.RegisterGlobal = false
			config.Metrics.ExportInterval = time.Hour
			config.Metrics.ExportTimeout = 100 * time.Millisecond
			config.ShutdownTimeout = 100 * time.Millisecond
			runtime, err := Init(context.Background(), config, WithMetricExporter(metrics), WithTraceExporter(traces))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if callerDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			started := time.Now()
			result := runtime.Shutdown(ctx)
			if !errors.Is(result, context.DeadlineExceeded) {
				t.Fatalf("Shutdown() = %v, want metric timeout", result)
			}
			if time.Since(started) > config.ShutdownTimeout+100*time.Millisecond {
				t.Fatal("shutdown exceeded bounded budget")
			}
			if !metrics.closed.Load() || traces.shutdownCount() != 1 {
				t.Fatalf("closed metrics/traces = %v/%d, want true/1", metrics.closed.Load(), traces.shutdownCount())
			}
			if repeated := runtime.Shutdown(context.Background()); !errors.Is(repeated, result) {
				t.Fatalf("repeat result = %v, want original %v", repeated, result)
			}
			if traces.shutdownCount() != 1 {
				t.Fatal("repeat closed trace exporter again")
			}
		})
	}
}

func TestRejectedInitializationReservesTraceCleanupTime(t *testing.T) {
	ownerConfig := enabledConfig("shutdown-owner", "test")
	ownerConfig.Metrics.Enabled = false
	owner, err := Init(context.Background(), ownerConfig, WithTraceExporter(&recordingSpanExporter{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown(context.Background()) })

	metrics := &timeoutMetricExporter{}
	traces := &recordingSpanExporter{}
	config := enabledConfig("rejected-owner", "test")
	config.ShutdownTimeout = 100 * time.Millisecond
	config.Metrics.ExportInterval = time.Hour
	config.Metrics.ExportTimeout = 100 * time.Millisecond
	rejected, err := Init(context.Background(), config, WithMetricExporter(metrics), WithTraceExporter(traces))
	if rejected != nil || !errors.Is(err, ErrAlreadyInitialized) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Init() = %v, %v, want rejected ownership and cleanup timeout", rejected, err)
	}
	if !metrics.closed.Load() || traces.shutdownCount() != 1 {
		t.Fatalf("closed rejected metrics/traces = %v/%d, want true/1", metrics.closed.Load(), traces.shutdownCount())
	}
}
