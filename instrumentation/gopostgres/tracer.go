// Package gopostgres provides a privacy-preserving pgx tracing bridge for
// postgres. SQL text, arguments, and raw database errors are never recorded.
package gopostgres

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const (
	scopeName        = "github.com/faustbrian/go-telemetry/instrumentation/gopostgres"
	defaultOperation = "postgresql.query"
	maxOperations    = 128
)

var operationPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

// Config defines the finite set of query names that telemetry may record.
type Config struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Operations     []string
}

// Tracer implements pgx.QueryTracer with bounded attributes.
type Tracer struct {
	tracer          trace.Tracer
	operations      map[string]struct{}
	duration        metric.Float64Histogram
	queries         metric.Int64Counter
	acquireDuration metric.Float64Histogram
	acquires        metric.Int64Counter
	waiting         metric.Int64UpDownCounter
	connections     metric.Int64Gauge
}

// New constructs a pgx query tracer. At most 128 fixed operation names may be
// configured; all unknown names collapse to postgresql.query.
func New(config Config) (*Tracer, error) {
	if len(config.Operations) > maxOperations {
		return nil, errors.New("PostgreSQL operation allow-list exceeds 128 entries")
	}
	operations := make(map[string]struct{}, len(config.Operations))
	for _, operation := range config.Operations {
		if !operationPattern.MatchString(operation) {
			return nil, errors.New("PostgreSQL operation must be a fixed low-cardinality name")
		}
		if _, duplicate := operations[operation]; duplicate {
			return nil, errors.New("PostgreSQL operation names must be unique")
		}
		operations[operation] = struct{}{}
	}
	if config.TracerProvider == nil {
		config.TracerProvider = tracenoop.NewTracerProvider()
	}
	if config.MeterProvider == nil {
		config.MeterProvider = metricnoop.NewMeterProvider()
	}
	meter := config.MeterProvider.Meter(scopeName)
	duration, err := meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	queries, err := meter.Int64Counter("db.client.operation.count", metric.WithUnit("{operation}"))
	if err != nil {
		return nil, err
	}
	acquireDuration, err := meter.Float64Histogram("db.client.connection.acquire.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	acquires, err := meter.Int64Counter("db.client.connection.acquire.count", metric.WithUnit("{acquire}"))
	if err != nil {
		return nil, err
	}
	waiting, err := meter.Int64UpDownCounter("db.client.connection.waiting", metric.WithUnit("{acquire}"))
	if err != nil {
		return nil, err
	}
	connections, err := meter.Int64Gauge("db.client.connection.count", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	return &Tracer{
		tracer: config.TracerProvider.Tracer(scopeName), operations: operations,
		duration: duration, queries: queries, acquireDuration: acquireDuration,
		acquires: acquires, waiting: waiting, connections: connections,
	}, nil
}

type acquireState struct {
	started time.Time
	span    trace.Span
}

type acquireStateKey struct{}

// TraceAcquireStart begins a bounded pool-acquisition observation. The
// returned context owns the matching state for TraceAcquireEnd.
func (tracer *Tracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	attributes := metric.WithAttributes(attribute.String("db.system.name", "postgresql"))
	tracer.waiting.Add(ctx, 1, attributes)
	ctx, span := tracer.tracer.Start(
		ctx,
		"postgresql.acquire",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", "postgresql")),
	)
	return context.WithValue(ctx, acquireStateKey{}, acquireState{started: time.Now(), span: span})
}

// TraceAcquireEnd completes pool acquisition without recording connection
// identity or raw error details.
func (tracer *Tracer) TraceAcquireEnd(ctx context.Context, pool *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	state, ok := ctx.Value(acquireStateKey{}).(acquireState)
	if !ok {
		return
	}
	outcome := "ok"
	if data.Err != nil {
		outcome = "error"
		if errors.Is(data.Err, context.Canceled) {
			outcome = "cancelled"
		} else if errors.Is(data.Err, context.DeadlineExceeded) {
			outcome = "timeout"
		}
		state.span.SetStatus(codes.Error, "database connection acquisition failed")
	}
	attributes := []attribute.KeyValue{
		attribute.String("db.system.name", "postgresql"),
		attribute.String("error.type", outcome),
	}
	tracer.waiting.Add(ctx, -1, metric.WithAttributes(attributes[0]))
	options := metric.WithAttributes(attributes...)
	tracer.acquireDuration.Record(ctx, time.Since(state.started).Seconds(), options)
	tracer.acquires.Add(ctx, 1, options)
	state.span.End()
	if pool == nil {
		return
	}
	stats := pool.Stat()
	for state, value := range map[string]int64{
		"acquired": int64(stats.AcquiredConns()),
		"idle":     int64(stats.IdleConns()),
		"total":    int64(stats.TotalConns()),
		"max":      int64(stats.MaxConns()),
	} {
		tracer.connections.Record(ctx, value, metric.WithAttributes(attribute.String("pool.state", state)))
	}
}

type operationContextKey struct{}

// ContextWithOperation associates a trusted static query name with ctx. The
// tracer records it only when it appears in Config.Operations.
func ContextWithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, operationContextKey{}, operation)
}

type queryState struct {
	started   time.Time
	operation string
	span      trace.Span
}

type queryStateKey struct{}

// TraceQueryStart starts a client span without inspecting SQL or arguments.
func (tracer *Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	operation := defaultOperation
	if candidate, ok := ctx.Value(operationContextKey{}).(string); ok {
		if _, allowed := tracer.operations[candidate]; allowed {
			operation = candidate
		}
	}
	attributes := []attribute.KeyValue{
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.operation.name", operation),
	}
	ctx, span := tracer.tracer.Start(
		ctx,
		operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attributes...),
	)
	return context.WithValue(ctx, queryStateKey{}, queryState{
		started:   time.Now(),
		operation: operation,
		span:      span,
	})
}

// TraceQueryEnd closes the query span and records only SQLSTATE when present.
func (tracer *Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	state, ok := ctx.Value(queryStateKey{}).(queryState)
	if !ok {
		return
	}
	span := state.span
	outcome := "ok"
	if data.Err != nil {
		outcome = "error"
		span.SetStatus(codes.Error, "database query failed")
		var postgresError *pgconn.PgError
		if errors.As(data.Err, &postgresError) && len(postgresError.Code) == 5 {
			span.SetAttributes(attribute.String("db.response.status_code", postgresError.Code))
		}
	}
	attributes := []attribute.KeyValue{
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.operation.name", state.operation),
		attribute.String("error.type", outcome),
	}
	duration := time.Since(state.started).Seconds()
	tracer.duration.Record(ctx, duration, metric.WithAttributes(attributes...))
	tracer.queries.Add(ctx, 1, metric.WithAttributes(attributes...))
	span.End()
}

var _ pgx.QueryTracer = (*Tracer)(nil)
var _ pgxpool.AcquireTracer = (*Tracer)(nil)
