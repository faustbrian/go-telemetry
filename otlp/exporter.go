package otlp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	metricexport "go.opentelemetry.io/otel/sdk/metric"
	traceexport "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

// NewTraceExporter constructs an owned standard OpenTelemetry span exporter.
// It validates configuration, checks caller cancellation, bounds TLS reads and
// constructor work by Timeout, and copies Headers. No implicit file reader is
// provided; secure mode uses TLS 1.2 or later unless HTTPClient owns TLS.
// Successful construction is not a delivery probe. TLS reader/parser diagnostics are categorical, but SDK
// constructor/export failures need not be redacted. The caller owns Shutdown.
func NewTraceExporter(ctx context.Context, config Config) (traceexport.SpanExporter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	tlsConfig, err := buildTLSConfigContext(ctx, config.TLS)
	if err != nil {
		return nil, err
	}
	if config.Protocol == ProtocolGRPC {
		options := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpointURL(endpointURL(config, "")),
			otlptracegrpc.WithHeaders(cloneHeaders(config.Headers)),
			otlptracegrpc.WithTimeout(config.Timeout),
			otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig(config.Retry)),
			otlptracegrpc.WithCompressor(grpcCompressor(config.Compression)),
		}
		if config.TLS.Insecure {
			options = append(options, otlptracegrpc.WithInsecure())
		} else {
			options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		}
		return otlptracegrpc.New(ctx, options...)
	}

	compression := otlptracehttp.NoCompression
	if config.Compression == CompressionGZIP {
		compression = otlptracehttp.GzipCompression
	}
	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpointURL(config, defaultTracePath)),
		otlptracehttp.WithHeaders(cloneHeaders(config.Headers)),
		otlptracehttp.WithTimeout(config.Timeout),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig(config.Retry)),
		otlptracehttp.WithCompression(compression),
	}
	if !config.TLS.Insecure {
		options = append(options, otlptracehttp.WithTLSClientConfig(tlsConfig))
	}
	if config.HTTPClient != nil {
		options = append(options, otlptracehttp.WithHTTPClient(config.HTTPClient))
	}
	return otlptracehttp.New(ctx, options...)
}

// NewMetricExporter constructs an owned standard OpenTelemetry metric exporter.
// It validates configuration, checks caller cancellation, bounds TLS reads and
// constructor work by Timeout, and copies Headers. No implicit file reader is
// provided; secure mode uses TLS 1.2 or later unless HTTPClient owns TLS.
// Successful construction is not a delivery probe. TLS reader/parser diagnostics are categorical, but SDK
// constructor/export failures need not be redacted. The caller owns Shutdown.
func NewMetricExporter(ctx context.Context, config Config) (metricexport.Exporter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	tlsConfig, err := buildTLSConfigContext(ctx, config.TLS)
	if err != nil {
		return nil, err
	}
	if config.Protocol == ProtocolGRPC {
		options := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpointURL(endpointURL(config, "")),
			otlpmetricgrpc.WithHeaders(cloneHeaders(config.Headers)),
			otlpmetricgrpc.WithTimeout(config.Timeout),
			otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig(config.Retry)),
			otlpmetricgrpc.WithCompressor(grpcCompressor(config.Compression)),
			otlpmetricgrpc.WithTemporalitySelector(metricexport.DefaultTemporalitySelector),
			otlpmetricgrpc.WithAggregationSelector(metricexport.DefaultAggregationSelector),
		}
		if config.TLS.Insecure {
			options = append(options, otlpmetricgrpc.WithInsecure())
		} else {
			options = append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		}
		return otlpmetricgrpc.New(ctx, options...)
	}

	compression := otlpmetrichttp.NoCompression
	if config.Compression == CompressionGZIP {
		compression = otlpmetrichttp.GzipCompression
	}
	options := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpointURL(endpointURL(config, defaultMetricPath)),
		otlpmetrichttp.WithHeaders(cloneHeaders(config.Headers)),
		otlpmetrichttp.WithTimeout(config.Timeout),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig(config.Retry)),
		otlpmetrichttp.WithCompression(compression),
		otlpmetrichttp.WithTemporalitySelector(metricexport.DefaultTemporalitySelector),
		otlpmetrichttp.WithAggregationSelector(metricexport.DefaultAggregationSelector),
	}
	if !config.TLS.Insecure {
		options = append(options, otlpmetrichttp.WithTLSClientConfig(tlsConfig))
	}
	if config.HTTPClient != nil {
		options = append(options, otlpmetrichttp.WithHTTPClient(config.HTTPClient))
	}
	return otlpmetrichttp.New(ctx, options...)
}

// The SDK exporters read OTEL_EXPORTER_OTLP_* from the process environment
// first and apply explicit options second, so every setting an environment
// variable can carry is passed explicitly: the endpoint URL fixes host, path
// and scheme (and so TLS), and compression and metric temporality and
// aggregation are always set. Config is then the only input.
const (
	defaultTracePath  = "/v1/traces"
	defaultMetricPath = "/v1/metrics"
)

// endpointURL renders Endpoint as a URL whose scheme selects TLS, with the
// configured HTTP signal path or the given default. gRPC passes no path.
func endpointURL(config Config, defaultPath string) string {
	scheme := "https"
	if config.TLS.Insecure {
		scheme = "http"
	}
	path := config.URLPath
	if path == "" || defaultPath == "" {
		path = defaultPath
	}
	return scheme + "://" + config.Endpoint + path
}

func grpcCompressor(compression Compression) string {
	if compression == CompressionGZIP {
		return "gzip"
	}
	return "none"
}

const maxTLSMaterialBytes = 1 << 20

func buildTLSConfig(config TLSConfig) (*tls.Config, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return buildTLSConfigContext(ctx, config)
}

func buildTLSConfigContext(ctx context.Context, config TLSConfig) (*tls.Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.Insecure {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: config.ServerName,
		// #nosec G402 -- Explicit caller-selected compatibility setting.
		InsecureSkipVerify: config.InsecureSkipVerify,
	}
	if config.CAFile != "" {
		contents, err := readTLSMaterial(ctx, config, config.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(contents) {
			return nil, errors.New("parse OTLP CA material failed")
		}
		tlsConfig.RootCAs = pool
	}
	if config.CertificateFile != "" {
		certificateBytes, err := readTLSMaterial(ctx, config, config.CertificateFile)
		if err != nil {
			return nil, err
		}
		keyBytes, err := readTLSMaterial(ctx, config, config.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
		certificate, err := tls.X509KeyPair(certificateBytes, keyBytes)
		if err != nil {
			return nil, errors.New("parse OTLP client certificate failed")
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, ctx.Err()
}

// readTLSMaterial trusts the reader to enforce preallocation/I/O bounds and
// rechecks cancellation and result length before parsing. Buffers are not wiped.
func readTLSMaterial(ctx context.Context, config TLSConfig, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.FileReader == nil {
		return nil, errors.New("OTLP TLS files require an explicit reader")
	}
	contents, err := config.FileReader.ReadFile(ctx, path, maxTLSMaterialBytes)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errors.New("read OTLP TLS material failed")
	}
	if len(contents) > maxTLSMaterialBytes {
		return nil, errors.New("TLS material limit exceeded")
	}
	return contents, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}
