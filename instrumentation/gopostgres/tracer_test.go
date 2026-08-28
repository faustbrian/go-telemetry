package gopostgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/faustbrian/go-telemetry/testtelemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestTracerRecordsPoolAcquireWaitAndOutcome(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	tracer, err := New(Config{MeterProvider: harness.MeterProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := tracer.TraceAcquireStart(
		context.Background(),
		nil,
		pgxpool.TraceAcquireStartData{},
	)
	metrics, err := harness.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() during acquire error = %v", err)
	}
	if got := int64MetricValue(t, metrics, "db.client.connection.waiting"); got != 1 {
		t.Fatalf("waiting acquisitions = %d, want 1", got)
	}
	tracer.TraceAcquireEnd(
		ctx,
		nil,
		pgxpool.TraceAcquireEndData{Err: context.DeadlineExceeded},
	)
	metrics, err = harness.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() after acquire error = %v", err)
	}
	if got := int64MetricValue(t, metrics, "db.client.connection.waiting"); got != 0 {
		t.Fatalf("waiting acquisitions after completion = %d, want 0", got)
	}
	if got := int64MetricValue(t, metrics, "db.client.connection.acquire.count"); got != 1 {
		t.Fatalf("acquire count = %d, want 1", got)
	}
}

func TestTracerRecordsBoundedAcquireOutcomesAndPoolStates(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	tracer, err := New(Config{MeterProvider: harness.MeterProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	poolConfig, err := pgxpool.ParseConfig("postgres://location@127.0.0.1:1/location?connect_timeout=1")
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	t.Cleanup(pool.Close)

	tracer.TraceAcquireEnd(context.Background(), pool, pgxpool.TraceAcquireEndData{})
	for _, acquireErr := range []error{
		nil,
		context.Canceled,
		errors.New("secret acquire failure"),
	} {
		ctx := tracer.TraceAcquireStart(context.Background(), pool, pgxpool.TraceAcquireStartData{})
		tracer.TraceAcquireEnd(ctx, pool, pgxpool.TraceAcquireEndData{Err: acquireErr})
	}
	metrics, err := harness.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	for _, name := range []string{
		"db.client.connection.acquire.duration",
		"db.client.connection.acquire.count",
		"db.client.connection.count",
	} {
		found := false
		for _, scope := range metrics.ScopeMetrics {
			for _, candidate := range scope.Metrics {
				found = found || candidate.Name == name
			}
		}
		if !found {
			t.Errorf("metric %q not found", name)
		}
	}
}

func int64MetricValue(t *testing.T, metrics metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, candidate := range scope.Metrics {
			if candidate.Name != name {
				continue
			}
			points := candidate.Data.(metricdata.Sum[int64]).DataPoints
			if len(points) != 1 {
				t.Fatalf("metric %q points = %d, want 1", name, len(points))
			}
			return points[0].Value
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func TestTracerNeverRecordsSQLOrArguments(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	tracer, err := New(Config{
		TracerProvider: harness.TracerProvider(),
		MeterProvider:  harness.MeterProvider(),
		Operations:     []string{"users.by_id"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := ContextWithOperation(context.Background(), "users.by_id")
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT * FROM users WHERE password = 'secret'",
		Args: []any{"secret-argument"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	spans := harness.Spans()
	if len(spans) != 1 || spans[0].Name != "users.by_id" {
		t.Fatalf("spans = %+v, want one named query span", spans)
	}
	text := fmt.Sprint(spans[0])
	if strings.Contains(text, "secret") || strings.Contains(text, "SELECT *") {
		t.Fatalf("span leaked SQL or arguments: %s", text)
	}
}

func TestTracerBoundsUnknownOperationAndDatabaseErrors(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	tracer, err := New(Config{
		TracerProvider: harness.TracerProvider(),
		Operations:     []string{"users.by_id"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := ContextWithOperation(context.Background(), "attacker-secret-id")
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "secret SQL"})
	queryErr := &pgconn.PgError{Code: "23505", Message: "secret value already exists"}
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: queryErr})

	span := harness.Spans()[0]
	if span.Name != "postgresql.query" || span.Status.Code != codes.Error {
		t.Fatalf("span name/status = %q/%v, want bounded fallback/error", span.Name, span.Status.Code)
	}
	text := fmt.Sprint(span)
	if strings.Contains(text, "secret") || !strings.Contains(text, "23505") {
		t.Fatalf("span must retain SQLSTATE without leaking error details: %s", text)
	}
}

func TestTracerPreservesOriginalErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("secret database error")
	harness := testtelemetry.New()
	tracer, err := New(Config{TracerProvider: harness.TracerProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: want})
	if strings.Contains(fmt.Sprint(harness.Spans()[0]), want.Error()) {
		t.Fatal("span recorded raw database error")
	}
}

func TestConfigRejectsUnboundedOperationContracts(t *testing.T) {
	t.Parallel()

	for _, config := range []Config{
		{Operations: []string{"invalid operation"}},
		{Operations: []string{"duplicate", "duplicate"}},
		{Operations: make([]string, 129)},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("New(%+v) error = nil, want validation error", config)
		}
	}
}

func TestConfigAcceptsMaximumOperationContract(t *testing.T) {
	t.Parallel()

	operations := make([]string, maxOperations)
	for index := range operations {
		operations[index] = fmt.Sprintf("operation.%d", index)
	}
	if _, err := New(Config{Operations: operations}); err != nil {
		t.Fatalf("New(maximum operations) error = %v", err)
	}
}

func TestTracerUsesNoopProvidersAndToleratesMissingStartState(t *testing.T) {
	t.Parallel()

	tracer, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	tracer.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{})
}

func TestTraceQueryEndOnlyEndsItsOwnedSpan(t *testing.T) {
	t.Parallel()

	harness := testtelemetry.New()
	tracer, err := New(Config{TracerProvider: harness.TracerProvider()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	parentCtx, parent := harness.TracerProvider().Tracer("test").Start(context.Background(), "parent")
	tracer.TraceQueryEnd(parentCtx, nil, pgx.TraceQueryEndData{})
	if spans := harness.Spans(); len(spans) != 0 {
		t.Fatalf("missing query state ended application span: %+v", spans)
	}

	queryCtx := tracer.TraceQueryStart(parentCtx, nil, pgx.TraceQueryStartData{})
	childCtx, child := harness.TracerProvider().Tracer("test").Start(queryCtx, "child")
	tracer.TraceQueryEnd(childCtx, nil, pgx.TraceQueryEndData{})
	spans := harness.Spans()
	if len(spans) != 1 || spans[0].Name != defaultOperation {
		t.Fatalf("ended spans = %+v, want only owned query span", spans)
	}

	child.End()
	parent.End()
}

func TestNewReportsInstrumentFailures(t *testing.T) {
	t.Parallel()

	want := errors.New("instrument failed")
	for _, failed := range []string{
		"db.client.operation.duration",
		"db.client.operation.count",
		"db.client.connection.acquire.duration",
		"db.client.connection.acquire.count",
		"db.client.connection.waiting",
		"db.client.connection.count",
	} {
		provider := errorMeterProvider{MeterProvider: metricnoop.NewMeterProvider(), meter: errorMeter{
			Meter: metricnoop.NewMeterProvider().Meter("test"), failed: failed, err: want,
		}}
		if _, err := New(Config{MeterProvider: provider}); !errors.Is(err, want) {
			t.Fatalf("New() %s error = %v, want %v", failed, err, want)
		}
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
	failed string
	err    error
}

func (meter errorMeter) Float64Histogram(name string, options ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Float64Histogram(name, options...)
}

func (meter errorMeter) Int64Counter(name string, options ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Int64Counter(name, options...)
}

func (meter errorMeter) Int64UpDownCounter(name string, options ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Int64UpDownCounter(name, options...)
}

func (meter errorMeter) Int64Gauge(name string, options ...metric.Int64GaugeOption) (metric.Int64Gauge, error) {
	if name == meter.failed {
		return nil, meter.err
	}
	return meter.Meter.Int64Gauge(name, options...)
}

var _ pgx.QueryTracer = (*Tracer)(nil)
