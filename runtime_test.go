package telemetry

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	telemetrymetric "github.com/faustbrian/go-telemetry/v2/metric"
	telemetrypropagation "github.com/faustbrian/go-telemetry/v2/propagation"
	telemetrytrace "github.com/faustbrian/go-telemetry/v2/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func enabledConfig(serviceName, serviceVersion string) Config {
	config := DefaultConfig(serviceName, serviceVersion)
	config.Traces.Enabled = true
	config.Metrics.Enabled = true
	config.RegisterGlobal = true
	return config
}

func TestOptionsApplyLeftToRightIgnoringNilAndUseLastExporter(t *testing.T) {
	firstTrace := &recordingSpanExporter{}
	lastTrace := &recordingSpanExporter{}
	firstMetric := &recordingMetricExporter{}
	lastMetric := &recordingMetricExporter{}

	config := enabledConfig("options", "test")
	config.RegisterGlobal = false
	runtime, err := Init(
		context.Background(),
		config,
		WithTraceExporter(firstTrace),
		nil,
		WithMetricExporter(firstMetric),
		WithTraceExporter(lastTrace),
		WithMetricExporter(lastMetric),
	)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if firstTrace.shutdowns != 0 || lastTrace.shutdowns != 1 {
		t.Fatalf("trace exporter shutdowns = first %d, last %d", firstTrace.shutdowns, lastTrace.shutdowns)
	}
	if firstMetric.shutdowns != 0 || lastMetric.shutdowns != 1 {
		t.Fatalf("metric exporter shutdowns = first %d, last %d", firstMetric.shutdowns, lastMetric.shutdowns)
	}
}

func TestRuntimeProvidesStandardAPIsAndShutsDownOnce(t *testing.T) {
	exporter := &recordingSpanExporter{}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Enabled = true
	config.Metrics.Enabled = false
	config.Traces.Sampler.Ratio = 1

	runtime, err := Init(context.Background(), config, WithTraceExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if runtime.TracerProvider() == nil {
		t.Fatal("TracerProvider() = nil, want standard provider")
	}
	if runtime.Tracer("test") == nil {
		t.Fatal("Tracer() = nil, want standard tracer")
	}
	if runtime.Propagator() == nil {
		t.Fatal("Propagator() = nil, want standard propagator")
	}

	_, span := runtime.Tracer("test").Start(context.Background(), "operation")
	span.End()
	if err := runtime.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if exporter.exportCount() != 1 {
		t.Fatalf("exported spans = %d, want 1", exporter.exportCount())
	}

	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if exporter.shutdownCount() != 1 {
		t.Fatalf("exporter shutdowns = %d, want 1", exporter.shutdownCount())
	}
}

func TestTraceQueueBoundDoesNotBlockBusinessWork(t *testing.T) {
	exporter := newBlockingSpanExporter()
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Enabled = false
	config.Traces.Sampler.Ratio = 1
	config.Traces.Batch.MaxQueueSize = 2
	config.Traces.Batch.MaxExportBatchSize = 1
	config.Traces.Batch.BatchTimeout = time.Hour

	runtime, err := Init(context.Background(), config, WithTraceExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, first := runtime.Tracer("test").Start(context.Background(), "blocked-export")
	first.End()
	select {
	case <-exporter.started:
	case <-time.After(time.Second):
		t.Fatal("export did not start")
	}

	produced := make(chan struct{})
	go func() {
		defer close(produced)
		for range 100 {
			_, span := runtime.Tracer("test").Start(context.Background(), "business-work")
			span.End()
		}
	}()
	select {
	case <-produced:
	case <-time.After(time.Second):
		t.Fatal("business work blocked on a saturated trace exporter")
	}

	close(exporter.release)
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := exporter.exportCount(); got > 3 {
		t.Fatalf("exported spans = %d, want at most active export plus queue bound", got)
	}
}

func TestRuntimeRestoresGlobalsItRegistered(t *testing.T) {
	previous := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = true
	config.Metrics.ExportInterval = time.Hour

	runtime, err := Init(
		context.Background(),
		config,
		WithTraceExporter(&recordingSpanExporter{}),
		WithMetricExporter(&recordingMetricExporter{}),
	)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if otel.GetTracerProvider() != runtime.TracerProvider() {
		t.Fatal("Init() did not register its tracer provider")
	}
	if otel.GetTextMapPropagator() != runtime.Propagator() {
		t.Fatal("Init() did not register its propagator")
	}
	if otel.GetMeterProvider() != runtime.MeterProvider() {
		t.Fatal("Init() did not register its meter provider")
	}

	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if otel.GetTracerProvider() != previous {
		t.Fatal("Shutdown() did not restore the previous tracer provider")
	}
	if otel.GetTextMapPropagator() != previousPropagator {
		t.Fatal("Shutdown() did not restore the previous propagator")
	}
	if otel.GetMeterProvider() != previousMeter {
		t.Fatal("Shutdown() did not restore the previous meter provider")
	}
}

func TestRuntimeReturnsShutdownFailuresOnEveryCall(t *testing.T) {
	want := errors.New("exporter shutdown failed")
	exporter := &recordingSpanExporter{shutdownErr: want}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Enabled = false

	runtime, err := Init(context.Background(), config, WithTraceExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	for range 2 {
		if err := runtime.Shutdown(context.Background()); !errors.Is(err, want) {
			t.Fatalf("Shutdown() error = %v, want error wrapping %v", err, want)
		}
	}
}

func TestShutdownCanRetryAfterPreCanceledContext(t *testing.T) {
	t.Parallel()

	exporter := &recordingSpanExporter{}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Enabled = true
	config.Metrics.Enabled = false
	runtime, err := Init(context.Background(), config, WithTraceExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Shutdown(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown(canceled) error = %v, want %v", err, context.Canceled)
	}
	if got := exporter.shutdownCount(); got != 0 {
		t.Fatalf("shutdowns after canceled call = %d, want 0", got)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown(retry) error = %v", err)
	}
	if got := exporter.shutdownCount(); got != 1 {
		t.Fatalf("shutdowns after retry = %d, want 1", got)
	}
	if err := runtime.Shutdown(canceled); err != nil {
		t.Fatalf("Shutdown(canceled after completion) error = %v, want shared result", err)
	}
}

func TestPreCanceledConcurrentShutdownSharesStartedResult(t *testing.T) {
	exporter := newBlockingShutdownSpanExporter()
	want := errors.New("exporter shutdown failed")
	exporter.shutdownErr = want
	release := sync.OnceFunc(func() { close(exporter.release) })
	defer release()

	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Enabled = false
	runtime, err := Init(context.Background(), config, WithTraceExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- runtime.Shutdown(context.Background()) }()
	<-exporter.started

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	secondDone := make(chan error, 1)
	go func() { secondDone <- runtime.Shutdown(canceled) }()
	release()
	for _, result := range []error{<-firstDone, <-secondDone} {
		if !errors.Is(result, want) || errors.Is(result, context.Canceled) {
			t.Fatal("concurrent shutdown did not share the terminal exporter result")
		}
	}
}

func TestRuntimeReturnsMetricShutdownFailures(t *testing.T) {
	want := errors.New("metric exporter shutdown failed")
	exporter := &recordingMetricExporter{shutdownErr: want}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Enabled = false
	config.Metrics.ExportInterval = time.Hour

	runtime, err := Init(context.Background(), config, WithMetricExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Shutdown() error = %v, want %v", err, want)
	}
}

func TestJoinDistinctAggregatesIndependentFailures(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")
	err := joinDistinct(first, second)
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("joinDistinct() error = %v, want both failures", err)
	}
}

func TestJoinDistinctDoesNotDuplicateExistingFailures(t *testing.T) {
	t.Parallel()

	first := errors.New("first")
	second := errors.New("second")
	got := joinDistinct(first, nil)
	var aggregate interface{ Unwrap() []error }
	if !errors.Is(got, first) || errors.As(got, &aggregate) {
		t.Fatalf("joinDistinct(first, nil) = %v, want original error", got)
	}
	wrapped := errors.Join(first, second)
	if got := joinDistinct(wrapped, second); got.Error() != wrapped.Error() {
		t.Fatalf("joinDistinct(wrapped, second) = %v, want original aggregate", got)
	}
}

func TestRuntimeAggregatesTraceAndMetricFlushFailures(t *testing.T) {
	traceFailure := errors.New("trace export failed")
	metricFailure := errors.New("metric flush failed")
	traceExporter := &recordingSpanExporter{exportErr: traceFailure}
	metricExporter := &recordingMetricExporter{flushErr: metricFailure}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Sampler.Ratio = 1
	config.Metrics.ExportInterval = time.Hour
	runtime, err := Init(
		context.Background(),
		config,
		WithTraceExporter(traceExporter),
		WithMetricExporter(metricExporter),
	)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	_, span := runtime.Tracer("test").Start(context.Background(), "operation")
	span.End()
	if err := runtime.ForceFlush(context.Background()); !errors.Is(err, traceFailure) || !errors.Is(err, metricFailure) {
		t.Fatalf("ForceFlush() error = %v, want both failures", err)
	}
	_ = runtime.Shutdown(context.Background())
}

func TestRuntimeOwnsMetricExportAndShutdown(t *testing.T) {
	exporter := &recordingMetricExporter{}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Enabled = false
	config.Metrics.ExportInterval = time.Hour

	runtime, err := Init(context.Background(), config, WithMetricExporter(exporter))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if runtime.MeterProvider() == nil {
		t.Fatal("MeterProvider() = nil, want standard provider")
	}
	counter, err := runtime.Meter("test").Int64Counter("jobs.processed")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1)

	if err := runtime.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if exporter.exportCount() != 1 {
		t.Fatalf("metric exports = %d, want 1", exporter.exportCount())
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if exporter.shutdownCount() != 1 {
		t.Fatalf("metric exporter shutdowns = %d, want 1", exporter.shutdownCount())
	}
}

func TestInitCleansUpTraceProviderAfterMetricConstructionFailure(t *testing.T) {
	exporter := &recordingSpanExporter{}
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Exporter.TLS.Insecure = false
	config.Metrics.Exporter.TLS.CAFile = "missing-ca.pem"
	config.Metrics.Exporter.TLS.FileReader = failingTLSReader{}

	if _, err := Init(context.Background(), config, WithTraceExporter(exporter)); err == nil {
		t.Fatal("Init() error = nil, want metric construction error")
	}
	if exporter.shutdownCount() != 1 {
		t.Fatalf("trace exporter shutdowns = %d, want partial initialization cleanup", exporter.shutdownCount())
	}
}

func TestInitRejectsDuplicateGlobalRuntimeAndCleansUp(t *testing.T) {
	config := enabledConfig("orders", "1.2.3")
	config.Metrics.ExportInterval = time.Hour
	first, err := Init(
		context.Background(),
		config,
		WithTraceExporter(&recordingSpanExporter{}),
		WithMetricExporter(&recordingMetricExporter{}),
	)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	defer func() { _ = first.Shutdown(context.Background()) }()
	secondExporter := &recordingSpanExporter{}
	secondMetricExporter := &recordingMetricExporter{}
	if _, err := Init(
		context.Background(),
		config,
		WithTraceExporter(secondExporter),
		WithMetricExporter(secondMetricExporter),
	); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second Init() error = %v, want %v", err, ErrAlreadyInitialized)
	}
	if secondExporter.shutdownCount() != 1 {
		t.Fatalf("second exporter shutdowns = %d, want cleanup", secondExporter.shutdownCount())
	}
	if secondMetricExporter.shutdownCount() != 1 {
		t.Fatalf("second metric exporter shutdowns = %d, want cleanup", secondMetricExporter.shutdownCount())
	}
}

func TestInitBoundsCleanupWithoutCallerCancellation(t *testing.T) {
	exporter := newCleanupProbeSpanExporter()
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Enabled = false
	config.ShutdownTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())

	started := time.Now()
	_, err := Init(
		ctx,
		config,
		WithTraceExporter(exporter),
		optionFunc(func(options *options) {
			options.buildSampler = func(telemetrytrace.Config) (trace.Sampler, error) {
				cancel()
				return nil, errors.New("sampler construction failed")
			}
		}),
	)
	if err == nil {
		t.Fatal("Init() error = nil, want sampler construction failure")
	}
	if exporter.contextWasCanceled.Load() {
		t.Fatal("cleanup inherited caller cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Init() error = %v, want bounded cleanup deadline", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Init() cleanup took %s, want bounded return", elapsed)
	}
}

func TestDuplicateInitCleanupDoesNotHoldGlobalLock(t *testing.T) {
	config := enabledConfig("orders", "1.2.3")
	config.Metrics.Enabled = false
	active, err := Init(context.Background(), config, WithTraceExporter(&recordingSpanExporter{}))
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	blocked := newBlockingShutdownSpanExporter()
	duplicateDone := make(chan error, 1)
	go func() {
		_, duplicateErr := Init(context.Background(), config, WithTraceExporter(blocked))
		duplicateDone <- duplicateErr
	}()
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("duplicate cleanup did not start")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- active.Shutdown(context.Background()) }()
	select {
	case shutdownErr := <-shutdownDone:
		if shutdownErr != nil {
			t.Fatalf("active Shutdown() error = %v", shutdownErr)
		}
	case <-time.After(100 * time.Millisecond):
		close(blocked.release)
		<-duplicateDone
		<-shutdownDone
		t.Fatal("duplicate cleanup held the global runtime lock")
	}

	close(blocked.release)
	if duplicateErr := <-duplicateDone; !errors.Is(duplicateErr, ErrAlreadyInitialized) {
		t.Fatalf("duplicate Init() error = %v, want %v", duplicateErr, ErrAlreadyInitialized)
	}
}

func TestShutdownDoesNotReplaceExternallyChangedGlobal(t *testing.T) {
	previous := otel.GetTracerProvider()
	config := enabledConfig("orders", "1.2.3")
	config.Metrics.Enabled = false
	runtime, err := Init(context.Background(), config, WithTraceExporter(&recordingSpanExporter{}))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	external := trace.NewTracerProvider()
	otel.SetTracerProvider(external)
	defer func() {
		otel.SetTracerProvider(previous)
		_ = external.Shutdown(context.Background())
	}()

	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if otel.GetTracerProvider() != external {
		t.Fatal("Shutdown() replaced an externally installed tracer provider")
	}
}

func TestDisabledRuntimeNeedsNoExporters(t *testing.T) {
	previousTracer := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()
	config := DefaultConfig("orders", "1.2.3")

	runtime, err := Init(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, ok := runtime.TracerProvider().(tracenoop.TracerProvider); !ok {
		t.Fatalf("TracerProvider() = %T, want no-op provider", runtime.TracerProvider())
	}
	if otel.GetTracerProvider() != previousTracer || otel.GetMeterProvider() != previousMeter ||
		otel.GetTextMapPropagator() != previousPropagator {
		t.Fatal("Init(DefaultConfig) mutated process-global telemetry")
	}
	if err := runtime.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInitSkipsNilOptionsAndAppliesFollowingOptions(t *testing.T) {
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Traces.Enabled = false
	config.Metrics.Enabled = false
	applied := false

	runtime, err := Init(context.Background(), config, nil, optionFunc(func(*options) {
		applied = true
	}))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !applied {
		t.Fatal("option following nil option was not applied")
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInitRejectsInvalidConfig(t *testing.T) {
	config := enabledConfig("", "1.2.3")
	if _, err := Init(context.Background(), config); err == nil {
		t.Fatal("Init() error = nil, want validation error")
	}
}

func TestInitReportsTraceExporterConstructionFailure(t *testing.T) {
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false
	config.Metrics.Enabled = false
	config.Traces.Exporter.TLS.Insecure = false
	config.Traces.Exporter.TLS.CAFile = "missing-ca.pem"
	config.Traces.Exporter.TLS.FileReader = failingTLSReader{}
	if _, err := Init(context.Background(), config); err == nil {
		t.Fatal("Init() error = nil, want trace exporter construction error")
	}
}

func TestInitPropagatesInternalConstructionFailures(t *testing.T) {
	want := errors.New("construction failed")
	config := enabledConfig("orders", "1.2.3")
	config.RegisterGlobal = false

	t.Run("resource", func(t *testing.T) {
		_, err := Init(context.Background(), config, optionFunc(func(options *options) {
			options.buildResource = func(context.Context, Config) (*resource.Resource, error) {
				return nil, want
			}
		}))
		if !errors.Is(err, want) {
			t.Fatalf("Init() error = %v, want %v", err, want)
		}
	})

	t.Run("propagator", func(t *testing.T) {
		_, err := Init(context.Background(), config, optionFunc(func(options *options) {
			options.buildPropagator = func(telemetrypropagation.Config) (*telemetrypropagation.Policy, error) {
				return nil, want
			}
		}))
		if !errors.Is(err, want) {
			t.Fatalf("Init() error = %v, want %v", err, want)
		}
	})

	t.Run("sampler", func(t *testing.T) {
		exporter := &recordingSpanExporter{}
		samplerConfig := config
		samplerConfig.Metrics.Enabled = false
		_, err := Init(
			context.Background(),
			samplerConfig,
			WithTraceExporter(exporter),
			optionFunc(func(options *options) {
				options.buildSampler = func(telemetrytrace.Config) (trace.Sampler, error) {
					return nil, want
				}
			}),
		)
		if !errors.Is(err, want) || exporter.shutdownCount() != 1 {
			t.Fatalf("Init() error/shutdowns = %v/%d, want failure and cleanup", err, exporter.shutdownCount())
		}
	})

	t.Run("metric options", func(t *testing.T) {
		exporter := &recordingSpanExporter{}
		_, err := Init(
			context.Background(),
			config,
			WithTraceExporter(exporter),
			WithMetricExporter(&recordingMetricExporter{}),
			optionFunc(func(options *options) {
				options.buildMetricOptions = func(telemetrymetric.Config) ([]metric.Option, error) {
					return nil, want
				}
			}),
		)
		if !errors.Is(err, want) || exporter.shutdownCount() != 1 {
			t.Fatalf("Init() error/shutdowns = %v/%d, want failure and cleanup", err, exporter.shutdownCount())
		}
	})
}

type recordingSpanExporter struct {
	mu          sync.Mutex
	exports     int
	shutdowns   int
	exportErr   error
	shutdownErr error
}

type blockingSpanExporter struct {
	recordingSpanExporter
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type cleanupProbeSpanExporter struct {
	recordingSpanExporter
	contextWasCanceled atomic.Bool
}

func newCleanupProbeSpanExporter() *cleanupProbeSpanExporter {
	return &cleanupProbeSpanExporter{}
}

func (e *cleanupProbeSpanExporter) Shutdown(ctx context.Context) error {
	if ctx.Err() != nil {
		e.contextWasCanceled.Store(true)
	}
	<-ctx.Done()
	return ctx.Err()
}

type blockingShutdownSpanExporter struct {
	recordingSpanExporter
	started chan struct{}
	release chan struct{}
}

func newBlockingShutdownSpanExporter() *blockingShutdownSpanExporter {
	return &blockingShutdownSpanExporter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *blockingShutdownSpanExporter) Shutdown(ctx context.Context) error {
	close(e.started)
	select {
	case <-e.release:
		return e.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newBlockingSpanExporter() *blockingSpanExporter {
	return &blockingSpanExporter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *blockingSpanExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	e.once.Do(func() { close(e.started) })
	select {
	case <-e.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return e.recordingSpanExporter.ExportSpans(ctx, spans)
}

type recordingMetricExporter struct {
	mu          sync.Mutex
	exports     int
	shutdowns   int
	exportErr   error
	flushErr    error
	shutdownErr error
}

func (e *recordingMetricExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e *recordingMetricExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (e *recordingMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	return e.exportErr
}

func (e *recordingMetricExporter) ForceFlush(context.Context) error {
	return e.flushErr
}

func (e *recordingMetricExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdowns++
	return e.shutdownErr
}

func (e *recordingMetricExporter) exportCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exports
}

func (e *recordingMetricExporter) shutdownCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.shutdowns
}

func (e *recordingSpanExporter) ExportSpans(_ context.Context, spans []trace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports += len(spans)
	return e.exportErr
}

func (e *recordingSpanExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdowns++
	return e.shutdownErr
}

func (e *recordingSpanExporter) exportCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exports
}

func (e *recordingSpanExporter) shutdownCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.shutdowns
}
