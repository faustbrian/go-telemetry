package gopostgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/faustbrian/go-telemetry/v2/instrumentation/gopostgres"
	postgres "github.com/faustbrian/go-telemetry/v2/instrumentation/postgres"
	"github.com/faustbrian/go-telemetry/v2/testtelemetry"
)

// These driver-dispatch checks require an explicitly supplied disposable
// PostgreSQL fixture. Ordinary package tests do not contact a database.
func pgxFixtureConfig(t *testing.T) *pgx.ConnConfig {
	t.Helper()
	dsn := os.Getenv("TELEMETRY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TELEMETRY_TEST_POSTGRES_DSN to a disposable PostgreSQL fixture")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid disposable PostgreSQL fixture configuration")
	}
	return config
}

func pgxFixtureHarness(t *testing.T) *testtelemetry.Harness {
	t.Helper()
	harness := testtelemetry.New()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := harness.Shutdown(ctx); err != nil {
			t.Error("telemetry fixture shutdown failed")
		}
	})
	return harness
}

func TestPGXDriverClosesCachedDeallocationFailure(t *testing.T) {
	config := pgxFixtureConfig(t)
	harness := pgxFixtureHarness(t)
	tracer, err := gopostgres.New(gopostgres.Config{
		TracerProvider: harness.TracerProvider(), MeterProvider: harness.MeterProvider(),
		Operations: []string{"fixture.query"},
	})
	if err != nil {
		t.Fatal(err)
	}
	config.Tracer = tracer
	config.StatementCacheCapacity = 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("disposable PostgreSQL connection failed")
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if err := conn.Close(closeCtx); err != nil {
			t.Error("disposable PostgreSQL connection cleanup failed")
		}
	})
	// The second prepared query evicts the first cache entry. The next Exec
	// must deallocate that entry before executing its SQL.
	for _, sql := range []string{"select $1::text", "select $1::text || 'private-pgx-sql'"} {
		var value string
		if err := conn.QueryRow(ctx, sql, "private-pgx-argument").Scan(&value); err != nil {
			t.Fatal("cached-query fixture setup failed")
		}
	}
	harness.ResetSpans()
	parentCtx, parent := harness.TracerProvider().Tracer("fixture.parent").Start(ctx, "fixture.parent")
	defer parent.End()
	queryCtx, queryCancel := context.WithCancel(gopostgres.ContextWithOperation(parentCtx, "fixture.query"))
	queryCancel()
	_, err = conn.Exec(queryCtx, "select $1::text || 'private-pgx-target'", "private-pgx-argument")
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "failed to deallocate cached statement(s)") {
		t.Fatal("fixture did not reach canceled cached-statement deallocation")
	}
	spans := harness.Spans()
	if len(spans) != 1 {
		t.Fatalf("ended target query spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name != "fixture.query" || span.Status.Code != codes.Error || span.Parent.SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("cleanup failure lost trusted operation, parent linkage, or error status")
	}
	metrics, err := harness.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pgxCounter(t, metrics, "db.client.operation.count", "error") != 1 || pgxDurationCount(t, metrics, "error") != 1 {
		t.Fatal("cleanup failure must produce exactly one error count and duration observation")
	}
	pgxAssertPrivate(t, fmt.Sprint(spans, metrics))
}

func TestPGXDriverDispatchPreservesPrivacyAndAcquireAccounting(t *testing.T) {
	config := pgxFixtureConfig(t)
	harness := pgxFixtureHarness(t)
	tracer, err := postgres.New(postgres.Config{
		TracerProvider: harness.TracerProvider(), MeterProvider: harness.MeterProvider(),
		Operations: []string{"fixture.query"},
	})
	if err != nil {
		t.Fatal(err)
	}
	poolConfig, err := pgxpool.ParseConfig(os.Getenv("TELEMETRY_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal("invalid disposable PostgreSQL pool configuration")
	}
	poolConfig.ConnConfig = config
	poolConfig.ConnConfig.Tracer = tracer
	poolConfig.MaxConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal("disposable PostgreSQL pool creation failed")
	}
	t.Cleanup(pool.Close)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal("disposable PostgreSQL acquisition failed")
	}
	defer conn.Release()
	queryCtx := postgres.ContextWithOperation(ctx, "fixture.query")
	var value string
	if err := conn.QueryRow(queryCtx, "select $1::text", "private-pgx-argument").Scan(&value); err != nil || value != "private-pgx-argument" {
		t.Fatal("driver query did not preserve its application result")
	}
	unknownCtx := postgres.ContextWithOperation(ctx, "private-pgx-operation")
	err = conn.QueryRow(unknownCtx, "select 1 / 0 /* private-pgx-sql */").Scan(&value)
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) || databaseError.Code != "22012" {
		t.Fatal("application did not receive the original division-by-zero SQLSTATE")
	}
	canceledCtx, stop := context.WithCancel(ctx)
	stop()
	if _, err := pool.Acquire(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled acquisition did not preserve context cancellation")
	}
	spans := harness.Spans()
	if len(spans) != 4 {
		t.Fatalf("ended driver query/acquire spans = %d, want 4", len(spans))
	}
	if spans[1].Name != "fixture.query" || spans[1].Status.Code == codes.Error || spans[2].Name != "postgresql.query" || spans[2].Status.Code != codes.Error {
		t.Fatal("driver query dispatch changed trusted/fallback operation or status")
	}
	if !strings.Contains(fmt.Sprint(spans[2].Attributes), "22012") {
		t.Fatal("bounded SQLSTATE is missing from database error telemetry")
	}
	metrics, err := harness.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pgxCounter(t, metrics, "db.client.operation.count", "ok") != 1 || pgxCounter(t, metrics, "db.client.operation.count", "error") != 1 {
		t.Fatal("driver queries must produce exactly one success and one error count")
	}
	if pgxCounter(t, metrics, "db.client.connection.acquire.count", "ok") != 1 || pgxCounter(t, metrics, "db.client.connection.acquire.count", "cancelled") != 1 || pgxCounter(t, metrics, "db.client.connection.waiting", "") != 0 {
		t.Fatal("driver acquisitions must produce exact outcomes and balanced waiting")
	}
	stats := pool.Stat()
	for state, want := range map[string]int64{"acquired": int64(stats.AcquiredConns()), "idle": int64(stats.IdleConns()), "total": int64(stats.TotalConns()), "max": int64(stats.MaxConns())} {
		if pgxPoolGauge(t, metrics, state) != want {
			t.Errorf("pool snapshot %s differs from public pool statistics", state)
		}
	}
	pgxAssertPrivate(t, fmt.Sprint(spans, metrics))
}

func pgxCounter(t *testing.T, metrics metricdata.ResourceMetrics, name, outcome string) int64 {
	t.Helper()
	var value int64
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				for _, point := range metric.Data.(metricdata.Sum[int64]).DataPoints {
					label, _ := point.Attributes.Value("error.type")
					if outcome == "" || label.AsString() == outcome {
						value += point.Value
					}
				}
			}
		}
	}
	return value
}

func pgxDurationCount(t *testing.T, metrics metricdata.ResourceMetrics, outcome string) uint64 {
	t.Helper()
	var count uint64
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "db.client.operation.duration" {
				for _, point := range metric.Data.(metricdata.Histogram[float64]).DataPoints {
					label, _ := point.Attributes.Value("error.type")
					if label.AsString() == outcome {
						count += point.Count
					}
				}
			}
		}
	}
	return count
}

func pgxPoolGauge(t *testing.T, metrics metricdata.ResourceMetrics, state string) int64 {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "db.client.connection.count" {
				for _, point := range metric.Data.(metricdata.Gauge[int64]).DataPoints {
					label, _ := point.Attributes.Value("pool.state")
					if label.AsString() == state {
						return point.Value
					}
				}
			}
		}
	}
	t.Fatalf("pool snapshot %s is missing", state)
	return 0
}

func pgxAssertPrivate(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, "private-pgx") || strings.Contains(text, "failed to deallocate") || strings.Contains(text, "division by zero") {
		t.Fatal("driver telemetry contains private SQL, argument, operation, or error details")
	}
}
