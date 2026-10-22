package telemetrycache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/faustbrian/go-telemetry/v4/testtelemetry"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestInstrumenterRecordsOnlyFixedCacheSemantics(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	instrumenter, err := New(Config{
		TracerProvider: harness.TracerProvider(),
		MeterProvider:  harness.MeterProvider(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, end := instrumenter.Begin(context.Background(), OperationGet)
	if ctx == nil {
		t.Fatal("Begin() context = nil")
	}
	end(OutcomeMiss, errors.New("secret cache key customer:123 failed"))

	span := harness.Spans()[0]
	text := fmt.Sprint(span)
	if span.InstrumentationScope.Name != "github.com/faustbrian/go-telemetry/v4/instrumentation/cache" {
		t.Fatalf("instrumentation scope = %q, want canonical identity", span.InstrumentationScope.Name)
	}
	if span.Name != "cache.get" || span.Status.Code != codes.Error || !strings.Contains(text, "cache.result") ||
		!strings.Contains(text, "error") || strings.Contains(text, "secret") || strings.Contains(text, "customer:123") {
		t.Fatalf("span recorded unsafe cache data: %s", text)
	}
	metrics, err := harness.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	for _, item := range metrics.ScopeMetrics[0].Metrics {
		if sum, ok := item.Data.(metricdata.Sum[int64]); ok && len(sum.DataPoints) != 1 {
			t.Fatalf("operation data points = %d, want 1", len(sum.DataPoints))
		}
	}
}

func TestInstrumenterCollapsesUnknownValues(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	instrumenter, err := New(Config{TracerProvider: harness.TracerProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, end := instrumenter.Begin(context.Background(), Operation("secret-operation"))
	end(Outcome("secret-outcome"), nil)
	span := harness.Spans()[0]
	if span.Name != "cache.other" || strings.Contains(fmt.Sprint(span), "secret") {
		t.Fatalf("unknown values were not collapsed: %+v", span)
	}
}

func TestEndIsConcurrencySafeAndIdempotent(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	instrumenter, err := New(Config{TracerProvider: harness.TracerProvider(), MeterProvider: harness.MeterProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, end := instrumenter.Begin(context.Background(), OperationLoad)
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			end(OutcomeHit, nil)
		}()
	}
	wait.Wait()
	if len(harness.Spans()) != 1 {
		t.Fatalf("spans = %d, want exactly 1", len(harness.Spans()))
	}
	metrics, err := harness.Metrics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics.ScopeMetrics) != 1 || metrics.ScopeMetrics[0].Scope.Name != "github.com/faustbrian/go-telemetry/v4/instrumentation/cache" {
		t.Fatal("completion emitted a noncanonical or duplicate instrumentation scope")
	}
	var operations int64
	var durations uint64
	for _, measurement := range metrics.ScopeMetrics[0].Metrics {
		switch measurement.Name {
		case "cache.operation.count":
			for _, point := range measurement.Data.(metricdata.Sum[int64]).DataPoints {
				operations += point.Value
			}
		case "cache.operation.duration":
			if measurement.Unit != "s" {
				t.Fatal("completion duration is not measured in seconds")
			}
			for _, point := range measurement.Data.(metricdata.Histogram[float64]).DataPoints {
				durations += point.Count
			}
		}
	}
	if operations != 1 || durations != 1 {
		t.Fatalf("concurrent completion recorded operations=%d duration samples=%d", operations, durations)
	}
}

func TestNewUsesNoopProviders(t *testing.T) {
	t.Parallel()

	instrumenter, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, end := instrumenter.Begin(context.Background(), OperationSet)
	end(OutcomeSuccess, nil)
}

func TestNewReportsInstrumentConflicts(t *testing.T) {
	t.Parallel()

	want := errors.New("instrument conflict")
	provider := errorMeterProvider{MeterProvider: metricnoop.NewMeterProvider(), meter: errorMeter{
		Meter:        metricnoop.NewMeterProvider().Meter("test"),
		histogramErr: want,
	}}
	if _, err := New(Config{MeterProvider: provider}); !errors.Is(err, want) {
		t.Fatal("New() duration error = nil, want instrument conflict")
	}
	provider.meter = errorMeter{
		Meter:      metricnoop.NewMeterProvider().Meter("test"),
		counterErr: want,
	}
	if _, err := New(Config{MeterProvider: provider}); !errors.Is(err, want) {
		t.Fatal("New() counter error = nil, want instrument conflict")
	}
}

type errorMeterProvider struct {
	metric.MeterProvider
	meter metric.Meter
}

func (provider errorMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return provider.meter
}

type errorMeter struct {
	metric.Meter
	histogramErr error
	counterErr   error
}

func (meter errorMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if meter.histogramErr != nil {
		return nil, meter.histogramErr
	}
	return meter.Meter.Float64Histogram("ok")
}

func (meter errorMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if meter.counterErr != nil {
		return nil, meter.counterErr
	}
	return meter.Meter.Int64Counter("ok")
}
