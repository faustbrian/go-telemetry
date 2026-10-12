// Package nethttp instruments net/http servers and clients without recording
// raw URLs, hosts, headers, bodies, client addresses, or arbitrary methods as
// wrapper-generated telemetry attributes. Fixed names/routes must be safe caller
// inputs; custom handlers, providers and propagators can emit additional data.
// Providers/transports are borrowed, not shut down; application deadlines,
// egress authorization, backend retention and SDK limits remain caller-owned.
// See TELEMETRY-DEC-007 and http_test.go for the selected semantic subset.
package nethttp

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	otelpropagation "go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const scopeName = "github.com/faustbrian/go-telemetry/instrumentation/nethttp"

var operationPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

// ServerConfig defines fixed, low-cardinality server instrumentation.
type ServerConfig struct {
	// Operation is a fixed span name matching [A-Za-z][A-Za-z0-9_.-]{0,127}.
	// Grammar does not prove it is low-cardinality or free of sensitive content.
	Operation string
	// Route is optional and fixed for every request; at most 256 bytes, starts
	// with / and contains no ? or #. Validation does not prove it is a route template.
	Route string
	// TrustedInbound enables per-request TrustInbound proof. Trusted extraction
	// is used only if the propagator implements ExtractTrusted and proof succeeds.
	TrustedInbound bool
	// TrustInbound proves that the individual request crossed an authenticated
	// trust boundary. It is required when TrustedInbound is true, is called for
	// every request, must return promptly, and must be safe for concurrent use.
	// A panic rejects the request with a categorical server error. It must not
	// grant trust from caller-controlled request metadata alone.
	TrustInbound func(*http.Request) bool
	// TracerProvider is borrowed; nil selects a no-op, not the process global.
	// The caller retains provider/exporter lifecycle and privacy policy.
	TracerProvider trace.TracerProvider
	// MeterProvider is borrowed; nil selects a no-op, not the process global.
	// The caller must configure any stream/cardinality limits and own shutdown.
	MeterProvider metric.MeterProvider
	// Propagator is borrowed; nil uses SDK TraceContext, not this module's
	// bounded baggage policy. Custom propagators own parsing/privacy and concurrency.
	Propagator otelpropagation.TextMapPropagator
}

// ClientConfig defines fixed, low-cardinality client instrumentation.
type ClientConfig struct {
	// Operation is the fixed span name, validated by the same grammar as ServerConfig.
	Operation string
	// TracerProvider is borrowed; nil selects a no-op, not the process global.
	// The caller retains provider/exporter lifecycle and privacy policy.
	TracerProvider trace.TracerProvider
	// MeterProvider is borrowed; nil selects a no-op, not the process global.
	// The caller must configure any stream/cardinality limits and own shutdown.
	MeterProvider metric.MeterProvider
	// Propagator is borrowed; nil uses SDK TraceContext. Supply the runtime
	// policy for bounded propagation and stale baggage replacement.
	Propagator otelpropagation.TextMapPropagator
}

type serverHandler struct {
	next         http.Handler
	operation    string
	route        string
	tracer       trace.Tracer
	propagator   otelpropagation.TextMapPropagator
	trusted      bool
	trustInbound func(*http.Request) bool
	duration     metric.Float64Histogram
	requests     metric.Int64Counter
	active       metric.Int64UpDownCounter
	requestSize  metric.Int64Histogram
	responseSize metric.Int64Histogram
}

// NewHandler wraps a non-nil handler with fixed-attribute tracing, metrics and
// extraction, preserving httpsnoop-supported ResponseWriter interfaces. It does
// not start a server, own provider shutdown or enforce request timeouts. A proof
// panic produces HTTP 500 without calling the handler. Handler panics are measured
// as server failures and re-panicked; panic values are not recorded. Request size
// uses known ContentLength, not body reads; response size counts captured writes.
func NewHandler(handler http.Handler, config ServerConfig) (http.Handler, error) {
	if handler == nil {
		return nil, errors.New("HTTP handler is required")
	}
	if err := validateOperation(config.Operation); err != nil {
		return nil, err
	}
	if len(config.Route) > 256 || strings.ContainsAny(config.Route, "?#") || (config.Route != "" && !strings.HasPrefix(config.Route, "/")) {
		return nil, errors.New("HTTP route must be a fixed path template")
	}
	if config.TrustedInbound && config.TrustInbound == nil {
		return nil, errors.New("trusted inbound propagation requires request proof")
	}
	tracerProvider, meterProvider, propagator := providers(
		config.TracerProvider,
		config.MeterProvider,
		config.Propagator,
	)
	meter := meterProvider.Meter(scopeName)
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	requests, err := meter.Int64Counter("http.server.request.count", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	requestSize, err := meter.Int64Histogram("http.server.request.body.size", metric.WithUnit("By"))
	if err != nil {
		return nil, err
	}
	responseSize, err := meter.Int64Histogram("http.server.response.body.size", metric.WithUnit("By"))
	if err != nil {
		return nil, err
	}
	return &serverHandler{
		next:         handler,
		operation:    config.Operation,
		route:        config.Route,
		tracer:       tracerProvider.Tracer(scopeName),
		propagator:   propagator,
		trusted:      config.TrustedInbound,
		trustInbound: config.TrustInbound,
		duration:     duration,
		requests:     requests,
		active:       active,
		requestSize:  requestSize,
		responseSize: responseSize,
	}, nil
}

func (handler *serverHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	carrier := otelpropagation.HeaderCarrier(request.Header)
	ctx := request.Context()
	trusted := false
	if handler.trusted {
		var panicked bool
		trusted, panicked = handler.trustsInbound(request)
		if panicked {
			http.Error(writer, "trusted inbound proof failed", http.StatusInternalServerError)
			return
		}
	}
	if extractor, ok := handler.propagator.(trustedExtractor); trusted && ok {
		ctx = extractor.ExtractTrusted(ctx, carrier)
	} else {
		ctx = handler.propagator.Extract(ctx, carrier)
	}
	attributes := []attribute.KeyValue{attribute.String("http.request.method", normalizeMethod(request.Method))}
	if handler.route != "" {
		attributes = append(attributes, attribute.String("http.route", handler.route))
	}
	ctx, span := handler.tracer.Start(
		ctx,
		handler.operation,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attributes...),
	)
	request = request.WithContext(ctx)

	started := time.Now()
	handler.active.Add(ctx, 1, metric.WithAttributes(attributes...))
	defer handler.active.Add(ctx, -1, metric.WithAttributes(attributes...))
	var captured httpsnoop.Metrics
	defer func() {
		panicValue := recover()
		status := captured.Code
		if status == 0 {
			status = http.StatusInternalServerError
		}
		resultAttributes := append(
			append([]attribute.KeyValue(nil), attributes...),
			attribute.Int("http.response.status_code", status),
		)
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "HTTP server error")
		}
		duration := captured.Duration
		if duration <= 0 {
			duration = time.Since(started)
		}
		handler.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(resultAttributes...))
		handler.requests.Add(ctx, 1, metric.WithAttributes(resultAttributes...))
		if request.ContentLength >= 0 {
			handler.requestSize.Record(ctx, request.ContentLength, metric.WithAttributes(resultAttributes...))
		}
		handler.responseSize.Record(ctx, captured.Written, metric.WithAttributes(resultAttributes...))
		span.End()
		if panicValue != nil {
			panic(panicValue)
		}
	}()

	captured = httpsnoop.CaptureMetrics(handler.next, writer, request)
}

func (handler *serverHandler) trustsInbound(request *http.Request) (trusted, panicked bool) {
	defer func() {
		if recover() != nil {
			trusted = false
			panicked = true
		}
	}()

	return handler.trustInbound(request), false
}

type trustedExtractor interface {
	ExtractTrusted(context.Context, otelpropagation.TextMapCarrier) context.Context
}

// Transport adds wrapper-generated method/status/duration telemetry to a borrowed
// RoundTripper. It does not record request target or raw error text, but is not a
// redaction boundary for the base transport or custom providers/propagator. It
// adds no timeout or CloseIdleConnections forwarding; the caller owns the base.
type Transport struct {
	base       http.RoundTripper
	operation  string
	tracer     trace.Tracer
	propagator otelpropagation.TextMapPropagator
	duration   metric.Float64Histogram
	requests   metric.Int64Counter
}

// NewTransport wraps base with fixed-attribute tracing, metrics and injection.
// Nil base uses http.DefaultTransport. Providers and base remain caller-owned;
// concurrent requests require collaborators safe for concurrent use. Successful
// construction creates instruments but does not test the network target.
func NewTransport(base http.RoundTripper, config ClientConfig) (*Transport, error) {
	if err := validateOperation(config.Operation); err != nil {
		return nil, err
	}
	switch base.(type) {
	case nil:
		base = http.DefaultTransport
	}
	tracerProvider, meterProvider, propagator := providers(
		config.TracerProvider,
		config.MeterProvider,
		config.Propagator,
	)
	meter := meterProvider.Meter(scopeName)
	duration, err := meter.Float64Histogram("http.client.request.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	requests, err := meter.Int64Counter("http.client.request.count", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	return &Transport{
		base:       base,
		operation:  config.Operation,
		tracer:     tracerProvider.Tracer(scopeName),
		propagator: propagator,
		duration:   duration,
		requests:   requests,
	}, nil
}

// RoundTrip rejects nil requests, clones request metadata and headers, injects
// context into the clone, and delegates once. The body remains shared according
// to http.Request.Clone semantics. Duration/span end cover RoundTrip, not reading
// the response body; callers must close that body. Response/error pass through
// unchanged and raw errors are not formatted into telemetry. Custom transports
// must obey RoundTripper (non-nil response on nil error); their status code is
// recorded without range normalization. Panics propagate without metric recovery.
func (transport *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errors.New("HTTP request is required")
	}
	method := normalizeMethod(request.Method)
	attributes := []attribute.KeyValue{attribute.String("http.request.method", method)}
	ctx, span := transport.tracer.Start(
		request.Context(),
		transport.operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attributes...),
	)
	defer span.End()

	clone := request.Clone(ctx)
	clone.Header = request.Header.Clone()
	transport.propagator.Inject(ctx, otelpropagation.HeaderCarrier(clone.Header))
	started := time.Now()
	response, err := transport.base.RoundTrip(clone)
	if err != nil {
		span.SetStatus(codes.Error, "HTTP transport failed")
		attributes = append(attributes, attribute.String("error.type", "transport"))
	} else {
		status := response.StatusCode
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		attributes = append(attributes, attribute.Int("http.response.status_code", status))
		if status >= http.StatusBadRequest {
			span.SetStatus(codes.Error, "HTTP request failed")
		}
	}
	duration := time.Since(started).Seconds()
	transport.duration.Record(ctx, duration, metric.WithAttributes(attributes...))
	transport.requests.Add(ctx, 1, metric.WithAttributes(attributes...))
	return response, err
}

func providers(
	tracerProvider trace.TracerProvider,
	meterProvider metric.MeterProvider,
	propagator otelpropagation.TextMapPropagator,
) (trace.TracerProvider, metric.MeterProvider, otelpropagation.TextMapPropagator) {
	if tracerProvider == nil {
		tracerProvider = tracenoop.NewTracerProvider()
	}
	if meterProvider == nil {
		meterProvider = metricnoop.NewMeterProvider()
	}
	if propagator == nil {
		propagator = otelpropagation.TraceContext{}
	}
	return tracerProvider, meterProvider, propagator
}

func validateOperation(operation string) error {
	if !operationPattern.MatchString(operation) {
		return errors.New("HTTP operation must be a fixed low-cardinality name")
	}
	return nil
}

func normalizeMethod(method string) string {
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut,
		http.MethodTrace:
		return method
	default:
		return "_OTHER"
	}
}
