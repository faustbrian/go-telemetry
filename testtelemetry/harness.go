// Package testtelemetry provides isolated in-memory SDK providers for tests.
// Trace IDs are sequential deterministic test data, not unpredictable production
// identities. Completed spans accumulate until ResetSpans; metrics and instrument
// count are not given application admission bounds here. The caller owns shutdown.
package testtelemetry

import (
	"context"
	"encoding/binary"
	"errors"
	"sync/atomic"

	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	traceapi "go.opentelemetry.io/otel/trace"
)

// Harness owns deterministic trace and metric providers and their readers.
type Harness struct {
	tracer   *sdktrace.TracerProvider
	recorder *tracetest.SpanRecorder
	meter    *sdkmetric.MeterProvider
	reader   *sdkmetric.ManualReader
}

// New constructs an isolated in-memory telemetry harness.
func New() *Harness {
	recorder := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(
		sdktrace.WithIDGenerator(&sequentialIDGenerator{}),
		sdktrace.WithSpanProcessor(recorder),
	)
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	return &Harness{tracer: tracer, recorder: recorder, meter: meter, reader: reader}
}

// TracerProvider lends the standard OpenTelemetry tracer provider; Harness owns shutdown.
func (h *Harness) TracerProvider() traceapi.TracerProvider {
	return h.tracer
}

// MeterProvider lends the standard OpenTelemetry meter provider; Harness owns shutdown.
func (h *Harness) MeterProvider() metricapi.MeterProvider {
	return h.meter
}

// Spans returns SDK SpanStub snapshots of spans ended at the recorder read.
// The result excludes active spans and is not a deep immutable/security copy;
// callers must not assume nested slices or SDK interfaces have independent ownership.
func (h *Harness) Spans() tracetest.SpanStubs {
	return tracetest.SpanStubsFromReadOnlySpans(h.recorder.Ended())
}

// ResetSpans clears the recorder, including its started/ended span inventory.
// It does not reset sequential identifiers or metric aggregation; coordinate
// concurrent span completion if a strict before/after boundary is needed.
func (h *Harness) ResetSpans() {
	h.recorder.Reset()
}

// Metrics synchronously collects into a fresh ResourceMetrics value using the
// manual reader and caller context. Errors may accompany partial SDK data. The
// returned tree is ordinary mutable SDK data, not a privacy/redaction boundary.
func (h *Harness) Metrics(ctx context.Context) (metricdata.ResourceMetrics, error) {
	var metrics metricdata.ResourceMetrics
	err := h.reader.Collect(ctx, &metrics)
	return metrics, err
}

// Shutdown calls meter then tracer shutdown with the same context and joins
// failures. It does not drain test goroutines or reserve budget per provider;
// repeated-call semantics are delegated to the SDK providers.
func (h *Harness) Shutdown(ctx context.Context) error {
	return errors.Join(h.meter.Shutdown(ctx), h.tracer.Shutdown(ctx))
}

type sequentialIDGenerator struct {
	trace atomic.Uint64
	span  atomic.Uint64
}

func (g *sequentialIDGenerator) NewIDs(context.Context) (traceapi.TraceID, traceapi.SpanID) {
	var traceID traceapi.TraceID
	binary.BigEndian.PutUint64(traceID[8:], g.trace.Add(1))
	return traceID, g.newSpanID()
}

func (g *sequentialIDGenerator) NewSpanID(context.Context, traceapi.TraceID) traceapi.SpanID {
	return g.newSpanID()
}

func (g *sequentialIDGenerator) newSpanID() traceapi.SpanID {
	var spanID traceapi.SpanID
	binary.BigEndian.PutUint64(spanID[:], g.span.Add(1))
	return spanID
}
