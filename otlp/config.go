// Package otlp constructs explicitly configured OTLP trace and metric
// exporters without reading vendor-specific settings.
package otlp

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

// Protocol selects an OTLP transport.
type Protocol string

const (
	// ProtocolGRPC exports OTLP over gRPC.
	ProtocolGRPC Protocol = "grpc"
	// ProtocolHTTPProtobuf exports OTLP protobuf payloads over HTTP.
	ProtocolHTTPProtobuf Protocol = "http/protobuf"
)

// Compression selects payload compression.
type Compression string

const (
	// CompressionNone disables compression.
	CompressionNone Compression = "none"
	// CompressionGZIP enables gzip compression.
	CompressionGZIP Compression = "gzip"
)

// Config completely describes an OTLP exporter transport.
type Config struct {
	Protocol    Protocol
	Endpoint    string
	URLPath     string
	Headers     map[string]string
	Compression Compression
	TLS         TLSConfig
	Retry       RetryConfig
	Timeout     time.Duration
}

// TLSConfig controls server verification and optional client certificates.
type TLSConfig struct {
	// FileReader must honor ctx and maxBytes before retaining file contents.
	// It is required for configured file paths; no ambient filesystem is read.
	FileReader         TLSFileReader
	Insecure           bool
	CAFile             string
	CertificateFile    string
	PrivateKeyFile     string
	ServerName         string
	InsecureSkipVerify bool
}

// TLSFileReader is the caller-owned cancellation-aware TLS material boundary.
// Implementations must apply the inclusive byte budget before allocation,
// propagate ctx to blocking I/O, and return promptly when it is canceled.
type TLSFileReader interface {
	ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error)
}

// RetryConfig bounds retry backoff and elapsed time.
type RetryConfig struct {
	Enabled         bool
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

// Validate rejects incomplete, unsupported, or unbounded transport settings.
func (c Config) Validate() error {
	var errs []error
	for _, value := range []string{c.Endpoint, c.URLPath, c.TLS.CAFile, c.TLS.CertificateFile, c.TLS.PrivateKeyFile} {
		if len(value) > 4096 || !utf8.ValidString(value) {
			errs = append(errs, errors.New("OTLP endpoint or path is invalid or exceeds 4096 bytes"))
			break
		}
	}
	if len(c.TLS.ServerName) > 253 || !utf8.ValidString(c.TLS.ServerName) {
		errs = append(errs, errors.New("OTLP server name is invalid or exceeds 253 bytes"))
	}
	if c.Protocol != ProtocolGRPC && c.Protocol != ProtocolHTTPProtobuf {
		errs = append(errs, errors.New("OTLP protocol is unsupported"))
	}
	if c.Endpoint == "" {
		errs = append(errs, errors.New("OTLP endpoint is required"))
	}
	if c.Compression != CompressionNone && c.Compression != CompressionGZIP {
		errs = append(errs, errors.New("OTLP compression is unsupported"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("OTLP timeout must be positive"))
	}
	if len(c.Headers) > 64 {
		errs = append(errs, errors.New("OTLP headers exceed 64 entries"))
	} else {
		total := 0
		for key, value := range c.Headers {
			if len(key) > (64<<10)-total {
				errs = append(errs, errors.New("OTLP headers exceed 65536 bytes"))
				break
			}
			total += len(key)
			if len(value) > (64<<10)-total {
				errs = append(errs, errors.New("OTLP headers exceed 65536 bytes"))
				break
			}
			total += len(value)
		}
	}
	if (c.TLS.CAFile != "" || c.TLS.CertificateFile != "" || c.TLS.PrivateKeyFile != "") && c.TLS.FileReader == nil {
		errs = append(errs, errors.New("OTLP TLS files require an explicit reader"))
	}
	if c.Retry.Enabled && (c.Retry.InitialInterval <= 0 || c.Retry.MaxInterval <= 0 || c.Retry.MaxElapsedTime <= 0) {
		errs = append(errs, errors.New("OTLP retry intervals must be positive"))
	}
	if (c.TLS.CertificateFile == "") != (c.TLS.PrivateKeyFile == "") {
		errs = append(errs, errors.New("OTLP client certificate and private key must be configured together"))
	}
	if c.TLS.Insecure && (c.TLS.CAFile != "" || c.TLS.CertificateFile != "" ||
		c.TLS.PrivateKeyFile != "" || c.TLS.ServerName != "" || c.TLS.InsecureSkipVerify) {
		errs = append(errs, errors.New("OTLP plaintext mode cannot include TLS settings"))
	}
	return errors.Join(errs...)
}
