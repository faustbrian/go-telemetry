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

// NewTraceExporter constructs a standard OpenTelemetry span exporter.
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
			otlptracegrpc.WithEndpoint(config.Endpoint),
			otlptracegrpc.WithHeaders(cloneHeaders(config.Headers)),
			otlptracegrpc.WithTimeout(config.Timeout),
			otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig(config.Retry)),
		}
		if config.Compression == CompressionGZIP {
			options = append(options, otlptracegrpc.WithCompressor("gzip"))
		}
		if config.TLS.Insecure {
			options = append(options, otlptracegrpc.WithInsecure())
		} else {
			options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		}
		return otlptracegrpc.New(ctx, options...)
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
		otlptracehttp.WithHeaders(cloneHeaders(config.Headers)),
		otlptracehttp.WithTimeout(config.Timeout),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig(config.Retry)),
	}
	if config.URLPath != "" {
		options = append(options, otlptracehttp.WithURLPath(config.URLPath))
	}
	if config.Compression == CompressionGZIP {
		options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}
	if config.TLS.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	} else {
		options = append(options, otlptracehttp.WithTLSClientConfig(tlsConfig))
	}
	return otlptracehttp.New(ctx, options...)
}

// NewMetricExporter constructs a standard OpenTelemetry metric exporter.
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
			otlpmetricgrpc.WithEndpoint(config.Endpoint),
			otlpmetricgrpc.WithHeaders(cloneHeaders(config.Headers)),
			otlpmetricgrpc.WithTimeout(config.Timeout),
			otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig(config.Retry)),
		}
		if config.Compression == CompressionGZIP {
			options = append(options, otlpmetricgrpc.WithCompressor("gzip"))
		}
		if config.TLS.Insecure {
			options = append(options, otlpmetricgrpc.WithInsecure())
		} else {
			options = append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		}
		return otlpmetricgrpc.New(ctx, options...)
	}

	options := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(config.Endpoint),
		otlpmetrichttp.WithHeaders(cloneHeaders(config.Headers)),
		otlpmetrichttp.WithTimeout(config.Timeout),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig(config.Retry)),
	}
	if config.URLPath != "" {
		options = append(options, otlpmetrichttp.WithURLPath(config.URLPath))
	}
	if config.Compression == CompressionGZIP {
		options = append(options, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	}
	if config.TLS.Insecure {
		options = append(options, otlpmetrichttp.WithInsecure())
	} else {
		options = append(options, otlpmetrichttp.WithTLSClientConfig(tlsConfig))
	}
	return otlpmetrichttp.New(ctx, options...)
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
