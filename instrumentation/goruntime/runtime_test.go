package goruntime

import (
	"context"
	"errors"
	"math"
	"testing"

	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	metricexport "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestInstrumenterExportsRequiredGoRuntimeSignals(t *testing.T) {
	t.Parallel()

	reader := metricexport.NewManualReader()
	provider := metricexport.NewMeterProvider(metricexport.WithReader(reader))
	instrumenter, err := New(provider)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := instrumenter.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	want := map[string]bool{
		"go.memory.heap.used": false,
		"go.memory.allocated": false,
		"go.goroutine.count":  false,
		"go.gc.cycles":        false,
		"go.gc.pause.time":    false,
	}
	for _, scope := range metrics.ScopeMetrics {
		for _, candidate := range scope.Metrics {
			if _, ok := want[candidate.Name]; ok {
				want[candidate.Name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("runtime metric %q was not exported", name)
		}
	}
	if err := instrumenter.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := instrumenter.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestNewRejectsMissingProviderAndReportsEveryInstrumentFailure(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil")
	}
	want := errors.New("instrument conflict")
	for _, failed := range []string{
		"go.memory.heap.used",
		"go.memory.allocated",
		"go.goroutine.count",
		"go.gc.cycles",
		"go.gc.pause.time",
		"callback",
	} {
		provider := failingMeterProvider{
			MeterProvider: metricnoop.NewMeterProvider(),
			meter: failingMeter{
				Meter:  metricnoop.NewMeterProvider().Meter("test"),
				failed: failed,
				err:    want,
			},
		}
		if _, err := New(provider); !errors.Is(err, want) {
			t.Errorf("New() failure %q error = %v, want %v", failed, err, want)
		}
	}
}

func TestNilCloseAndUnsignedBounds(t *testing.T) {
	t.Parallel()

	if err := (*Instrumenter)(nil).Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if got := boundedUint64(uint64(math.MaxInt64)); got != math.MaxInt64 {
		t.Fatalf("bounded maximum = %d", got)
	}
	if got := boundedUint64(math.MaxUint64); got != math.MaxInt64 {
		t.Fatalf("bounded overflow = %d", got)
	}
}

type failingMeterProvider struct {
	metric.MeterProvider
	meter metric.Meter
}

func (provider failingMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return provider.meter
}

type failingMeter struct {
	metric.Meter
	failed string
	err    error
}

func (meter failingMeter) Int64ObservableGauge(name string, options ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Int64ObservableGauge(name, options...)
}

func (meter failingMeter) Int64ObservableCounter(name string, options ...metric.Int64ObservableCounterOption) (metric.Int64ObservableCounter, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Int64ObservableCounter(name, options...)
}

func (meter failingMeter) Float64ObservableCounter(name string, options ...metric.Float64ObservableCounterOption) (metric.Float64ObservableCounter, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Float64ObservableCounter(name, options...)
}

func (meter failingMeter) RegisterCallback(callback metric.Callback, instruments ...metric.Observable) (metric.Registration, error) {
	if meter.failed == "callback" {
		return nil, meter.err
	}
	return meter.Meter.RegisterCallback(callback, instruments...)
}
