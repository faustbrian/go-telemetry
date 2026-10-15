// Package telemetry owns explicitly configured OpenTelemetry trace and metric
// providers, OTLP exporters, propagation and optional process globals.
// Defaults enable no signals and register no globals. Call Init to construct a
// runtime, drain application work before Shutdown, and retain the shutdown error.
//
// Export is best effort: bounded queues, sampling, failed exports, cancellation
// and process exit can lose telemetry. Queue and stream-count budgets do not
// bound arbitrary attributes, instrument count or total process memory. Standard
// OpenTelemetry APIs remain available, so applications own privacy, attribute
// vocabulary, authentication, backend retention and deployment resource limits.
// Configuration must not be mutated concurrently with validation/construction.
//
// The owned profile is OTLP 1.10.0 trace/metric protobuf, W3C Trace Context Level 1
// (2021-11-23), defensive Baggage (2024-05-30), and Semantic Conventions 1.40.0.
// See specification/manifest.tsv and docs/specification-decisions.md for pinned
// authorities and TELEMETRY-DEC-001 through TELEMETRY-DEC-007; these profiles do
// not qualify every Collector or implement every semantic attribute.
package telemetry

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	telemetrymetric "github.com/faustbrian/go-telemetry/v3/metric"
	telemetryotlp "github.com/faustbrian/go-telemetry/v3/otlp"
	telemetrypropagation "github.com/faustbrian/go-telemetry/v3/propagation"
	telemetrytrace "github.com/faustbrian/go-telemetry/v3/trace"
)

const (
	maxResourceAttributes     = 128
	maxResourceAttributeBytes = 64 * 1_024
)

// Protocol selects an OTLP transport.
type Protocol string

const (
	// ProtocolGRPC exports OTLP using gRPC.
	ProtocolGRPC Protocol = "grpc"
	// ProtocolHTTPProtobuf exports OTLP using HTTP and protobuf.
	ProtocolHTTPProtobuf Protocol = "http/protobuf"
)

// Compression selects exporter payload compression.
type Compression string

const (
	// CompressionNone disables payload compression.
	CompressionNone Compression = "none"
	// CompressionGZIP enables gzip payload compression.
	CompressionGZIP Compression = "gzip"
)

// Config describes the complete runtime. Constructing it has no side effects.
type Config struct {
	// Service supplies explicit resource identity; Name is required.
	Service ServiceConfig
	// Environment is the optional deployment.environment.name resource value.
	Environment string
	// Resource adds custom string attributes, at most 128 entries and 64 KiB
	// of combined keys/values. Reserved service/SDK keys are rejected by Validate.
	// Values are exported as supplied, not redacted; Init does not deep-copy Config.
	Resource map[string]string
	// Traces enables and configures the trace provider; disabled settings are not validated.
	Traces TraceConfig
	// Metrics enables and configures the metric provider; disabled settings are not validated.
	Metrics MetricConfig
	// Propagation is validated even when both signals are disabled.
	Propagation telemetrypropagation.Config
	// RegisterGlobal opts into process-wide provider/propagator replacement.
	// Only one runtime registered through this package may be active at a time.
	RegisterGlobal bool
	// ShutdownTimeout must be positive and caps each flush/shutdown call
	// subject to cooperating providers and exporters; it is not a hard preemption limit.
	ShutdownTimeout time.Duration
}

// ServiceConfig identifies the process producing telemetry.
type ServiceConfig struct {
	// Name is the required service.name; valid UTF-8, at most 255 bytes.
	Name string
	// Version is optional service.version; valid UTF-8, at most 255 bytes.
	Version string
	// Namespace is optional service.namespace; valid UTF-8, at most 255 bytes.
	Namespace string
	// Instance is optional service.instance.id; valid UTF-8, at most 255 bytes.
	// It is not generated and must not contain secrets.
	Instance string
}

// TraceConfig controls the trace provider and exporter.
type TraceConfig struct {
	// Enabled constructs a batching SDK provider; false exposes a no-op provider.
	Enabled bool
	// Exporter configures OTLP and is validated even when WithTraceExporter overrides it.
	Exporter ExporterConfig
	// Batch bounds SDK queue slots, batch size and export timing, not span payload bytes.
	Batch BatchConfig
	// Sampler selects the root policy and optional parent-decision inheritance.
	Sampler SamplerConfig
}

// MetricConfig controls the metric provider and exporter.
type MetricConfig struct {
	// Enabled constructs an SDK provider with a periodic reader; false exposes a no-op provider.
	Enabled bool
	// Exporter configures OTLP and is validated even when WithMetricExporter overrides it.
	Exporter ExporterConfig
	// ExportInterval must be positive and schedules periodic collection.
	ExportInterval time.Duration
	// ExportTimeout must be positive and bounds periodic collection/export.
	ExportTimeout time.Duration
	// CardinalityLimit is the SDK per-stream series budget, between 1 and 100000.
	// Overflow series do not preserve every original attribute combination.
	CardinalityLimit int
	// Views selects metric streams and attribute allow-lists; unmatched instruments
	// retain SDK behavior. See metric.ViewConfig for matching and histogram controls.
	Views []telemetrymetric.ViewConfig
}

// ExporterConfig controls an OTLP exporter.
type ExporterConfig struct {
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
	// Timeout must be positive and bounds construction and SDK export attempts.
	// A custom TLS reader must cooperate with the constructor context.
	Timeout time.Duration
}

// TLSConfig controls transport security. Insecure is intended for a local or
// same-cluster Collector connection and must be selected explicitly.
type TLSConfig struct {
	// FileReader owns explicitly configured, bounded TLS material access.
	FileReader telemetryotlp.TLSFileReader
	// Insecure explicitly selects plaintext and excludes all other TLS settings.
	Insecure bool
	// CAFile selects PEM roots through FileReader; a custom pool replaces system roots.
	CAFile string
	// CertificateFile selects the PEM client certificate; requires PrivateKeyFile.
	CertificateFile string
	// PrivateKeyFile selects the matching PEM private key; requires CertificateFile.
	PrivateKeyFile string
	// ServerName overrides TLS peer name verification; at most 253 UTF-8 bytes.
	ServerName string
	// InsecureSkipVerify disables certificate/name verification; it is an explicit
	// unsafe compatibility setting, not authentication.
	InsecureSkipVerify bool
}

// RetryConfig bounds exporter retry behavior.
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

// BatchConfig bounds trace batching and its memory use.
type BatchConfig struct {
	// MaxQueueSize bounds queued span-interface slots, from 1 through 65536.
	// The SDK batcher uses its nonblocking default and can drop spans on overflow.
	MaxQueueSize int
	// MaxExportBatchSize must be positive and no greater than MaxQueueSize.
	MaxExportBatchSize int
	// BatchTimeout must be positive and controls the scheduled batch delay.
	BatchTimeout time.Duration
	// ExportTimeout must be positive and bounds an SDK batch export.
	ExportTimeout time.Duration
}

// SamplerConfig controls parent-based ratio sampling.
type SamplerConfig struct {
	// Mode selects always-on, always-off or trace-ID ratio root sampling.
	Mode telemetrytrace.Mode
	// Ratio is checked only in ratio mode and must be finite in [0,1].
	Ratio float64
	// ParentBased delegates sampled/unsampled parent decisions to the SDK;
	// Mode and Ratio then select only the root policy. Sampling is not redaction.
	ParentBased bool
}

// DefaultConfig returns inspectable, inactive defaults. Callers must explicitly
// enable signals, global registration, and plaintext transport when required.
func DefaultConfig(serviceName, serviceVersion string) Config {
	exporter := ExporterConfig{
		Protocol:    ProtocolGRPC,
		Endpoint:    "localhost:4317",
		Headers:     make(map[string]string),
		Compression: CompressionGZIP,
		TLS:         TLSConfig{},
		Retry: RetryConfig{
			Enabled:         true,
			InitialInterval: 5 * time.Second,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  time.Minute,
		},
		Timeout: 10 * time.Second,
	}
	metricExporter := exporter
	metricExporter.Headers = make(map[string]string)

	return Config{
		Service:  ServiceConfig{Name: serviceName, Version: serviceVersion},
		Resource: make(map[string]string),
		Traces: TraceConfig{
			Enabled:  false,
			Exporter: exporter,
			Batch: BatchConfig{
				MaxQueueSize:       2_048,
				MaxExportBatchSize: 512,
				BatchTimeout:       5 * time.Second,
				ExportTimeout:      10 * time.Second,
			},
			Sampler: SamplerConfig{
				Mode:        telemetrytrace.ModeRatio,
				Ratio:       0.1,
				ParentBased: true,
			},
		},
		Metrics: MetricConfig{
			Enabled:          false,
			Exporter:         metricExporter,
			ExportInterval:   time.Minute,
			ExportTimeout:    30 * time.Second,
			CardinalityLimit: 1_000,
		},
		Propagation:     telemetrypropagation.DefaultConfig(),
		RegisterGlobal:  false,
		ShutdownTimeout: 10 * time.Second,
	}
}

// validateResource bounds explicit identity and custom attributes before building.
// Direct BuildResource allows reserved entries only to skip them; Init rejects them.
func (c Config) validateResource(allowReserved bool) error {
	var errs []error
	if c.Service.Name == "" {
		errs = append(errs, errors.New("service name is required"))
	}
	identity := []struct {
		name  string
		value string
	}{
		{name: "service name", value: c.Service.Name},
		{name: "service version", value: c.Service.Version},
		{name: "service namespace", value: c.Service.Namespace},
		{name: "service instance", value: c.Service.Instance},
		{name: "environment", value: c.Environment},
	}
	for _, field := range identity {
		if len(field.value) > 255 || !utf8.ValidString(field.value) {
			errs = append(errs, fmt.Errorf("%s must be valid UTF-8 and at most 255 bytes", field.name))
		}
	}
	resourceBytes := 0
	if len(c.Resource) > maxResourceAttributes {
		errs = append(errs, fmt.Errorf("resource attributes exceed %d entries", maxResourceAttributes))
	} else {
		for key, value := range c.Resource {
			resourceBytes += len(key) + len(value)
			if _, reserved := reservedResourceKeys[key]; reserved && !allowReserved {
				errs = append(errs, errors.New("resource attribute uses a reserved key"))
			}
			if key == "" || len(key) > 255 || !utf8.ValidString(key) {
				errs = append(errs, errors.New("resource attribute key is invalid"))
			}
			if len(value) > 4_096 || !utf8.ValidString(value) {
				errs = append(errs, errors.New("resource attribute value is invalid"))
			}
		}
		if resourceBytes > maxResourceAttributeBytes {
			errs = append(errs, fmt.Errorf("resource attributes exceed %d bytes", maxResourceAttributeBytes))
		}
	}
	return errors.Join(errs...)
}

// Validate aggregates resource, lifecycle, propagation and enabled-signal errors.
// Disabled exporter, batch, metric and sampler settings are not checked. String
// configuration values are not echoed by these validators; a numeric invalid
// sampling ratio may appear in the sampler error. No network or file I/O occurs.
func (c Config) Validate() error {
	var errs []error
	errs = append(errs, c.validateResource(false))
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown timeout must be positive"))
	}
	if c.Traces.Enabled {
		errs = append(errs, c.Traces.validate()...)
	}
	if c.Metrics.Enabled {
		errs = append(errs, c.Metrics.validate()...)
	}
	if _, err := telemetrypropagation.New(c.Propagation); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (c TraceConfig) validate() []error {
	errs := c.Exporter.validate("trace")
	if c.Batch.MaxQueueSize <= 0 || c.Batch.MaxQueueSize > 65_536 {
		errs = append(errs, errors.New("trace batch max queue size must be between 1 and 65536"))
	}
	if c.Batch.MaxExportBatchSize <= 0 || c.Batch.MaxExportBatchSize > c.Batch.MaxQueueSize {
		errs = append(errs, errors.New("trace batch size must be positive and no greater than queue size"))
	}
	if c.Batch.BatchTimeout <= 0 || c.Batch.ExportTimeout <= 0 {
		errs = append(errs, errors.New("trace batch timeouts must be positive"))
	}
	if _, err := telemetrytrace.NewSampler(telemetrytrace.Config{
		Mode:        c.Sampler.Mode,
		Ratio:       c.Sampler.Ratio,
		ParentBased: c.Sampler.ParentBased,
	}); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (c MetricConfig) validate() []error {
	errs := c.Exporter.validate("metric")
	if c.ExportInterval <= 0 || c.ExportTimeout <= 0 {
		errs = append(errs, errors.New("metric export interval and timeout must be positive"))
	}
	if _, err := telemetrymetric.Options(telemetrymetric.Config{
		CardinalityLimit: c.CardinalityLimit,
		Views:            c.Views,
	}); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (c ExporterConfig) validate(signal string) []error {
	if err := otlpConfig(c).Validate(); err != nil {
		return []error{fmt.Errorf("%s exporter: %w", signal, err)}
	}
	return nil
}
