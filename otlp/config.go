// Package otlp constructs explicitly configured OTLP trace and metric
// exporters using standard OpenTelemetry SDK implementations. Wrappers supply
// every transport option explicitly, so OTEL_EXPORTER_OTLP_* environment
// variables cannot change endpoints, TLS, headers, compression or metric
// temporality and aggregation. Proxy variables apply without HTTPClient.
// The owned profile is OTLP 1.10.0 trace/metric protobuf (TELEMETRY-DEC-001/002),
// not all signals or guaranteed Collector/backend delivery. Exporter ownership
// transfers to the caller, who must shut it down or attach it to one runtime.
package otlp

import (
	"context"
	"errors"
	"net/http"
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
	// Protocol explicitly selects gRPC or HTTP/protobuf.
	Protocol Protocol
	// Endpoint is the SDK host[:port] target, not a full URL; at most 4096 UTF-8 bytes.
	// This validation does not authorize the target or restrict network egress.
	Endpoint string
	// URLPath optionally replaces the HTTP signal path; ignored for gRPC.
	// It is limited to 4096 UTF-8 bytes, not validated as a safe application route.
	URLPath string
	// Headers adds exporter request metadata; at most 64 entries and 64 KiB
	// combined keys/values. Constructors copy the map; callers own credential policy.
	Headers map[string]string
	// Compression explicitly selects none or gzip.
	Compression Compression
	// TLS supplies transport security and explicit TLS material access.
	TLS TLSConfig
	// Retry controls SDK retry policy; finite positive intervals are required when enabled.
	Retry RetryConfig
	// Timeout must be positive and bounds construction and SDK export attempts;
	// with HTTPClient it bounds construction only. A custom TLS reader must
	// cooperate with the constructor context.
	Timeout time.Duration
	// HTTPClient optionally replaces the SDK's HTTP client for HTTP/protobuf, so
	// the caller owns proxy, dialing, redirect and TLS policy; the SDK client
	// otherwise reads proxies from the environment, as does a client whose
	// Transport is nil. The SDK then ignores Timeout for export attempts, so the
	// client's own Timeout must be positive. TLS.Insecure still selects the http
	// scheme; any other TLS setting, and gRPC, are rejected. The exporter keeps
	// the client for its lifetime and does not close its idle connections.
	HTTPClient *http.Client
}

// TLSConfig controls server verification and optional client certificates.
type TLSConfig struct {
	// FileReader must honor ctx and maxBytes before retaining file contents.
	// It is required for configured file paths; no ambient filesystem is read.
	FileReader TLSFileReader
	// Insecure selects plaintext and must not be combined with other TLS settings.
	Insecure bool
	// CAFile selects PEM roots; a custom trust pool replaces system roots.
	CAFile string
	// CertificateFile selects PEM client identity and requires PrivateKeyFile.
	CertificateFile string
	// PrivateKeyFile selects the matching PEM private key and requires CertificateFile.
	PrivateKeyFile string
	// ServerName overrides the TLS peer name; at most 253 UTF-8 bytes.
	ServerName string
	// InsecureSkipVerify explicitly bypasses peer certificate/name verification.
	InsecureSkipVerify bool
}

// TLSFileReader is the caller-owned cancellation-aware TLS material boundary.
// Implementations must apply the inclusive byte budget before allocation,
// propagate ctx to blocking I/O, and return promptly when it is canceled.
type TLSFileReader interface {
	// ReadFile reads caller-authorized material within the inclusive byte budget.
	// Construction requests at most 1 MiB per file and rechecks the result; it
	// cannot force a blocked implementation to return. Paths need not be contained
	// by this package. Returned material is parsed, not securely erased.
	ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error)
}

// RetryConfig bounds retry backoff and elapsed time.
type RetryConfig struct {
	// Enabled delegates retryable-failure handling to the selected SDK exporter.
	// Retries are not durable delivery or an exactly-once guarantee.
	Enabled bool
	// InitialInterval is the positive initial backoff when Enabled.
	InitialInterval time.Duration
	// MaxInterval is the positive backoff ceiling when Enabled.
	// Validation does not require it to exceed InitialInterval.
	MaxInterval time.Duration
	// MaxElapsedTime is the positive retry horizon when Enabled.
	MaxElapsedTime time.Duration
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
	if c.HTTPClient != nil && (c.Protocol != ProtocolHTTPProtobuf || c.TLS.CAFile != "" || c.TLS.CertificateFile != "" ||
		c.TLS.PrivateKeyFile != "" || c.TLS.ServerName != "" || c.TLS.InsecureSkipVerify) {
		errs = append(errs, errors.New("OTLP HTTP client requires HTTP/protobuf and owns TLS beyond the scheme"))
	}
	if c.HTTPClient != nil && c.HTTPClient.Timeout <= 0 {
		errs = append(errs, errors.New("OTLP HTTP client timeout must be positive"))
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
