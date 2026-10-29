package otlp

import (
	"context"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// hostileOTLPEnvironment sets the OTEL_EXPORTER_OTLP_* variables the SDK
// exporters read (generic and per-signal) to values that contradict the
// explicit Config, including readable TLS material.
func hostileOTLPEnvironment(t *testing.T, elsewhere string) {
	t.Helper()
	certificate := writeCertificatePEM(t)
	values := map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":                              elsewhere,
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE":        "delta",
		"OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION": "base2_exponential_bucket_histogram",
	}
	for _, prefix := range []string{"OTEL_EXPORTER_OTLP_", "OTEL_EXPORTER_OTLP_TRACES_", "OTEL_EXPORTER_OTLP_METRICS_"} {
		values[prefix+"INSECURE"] = "true"
		values[prefix+"HEADERS"] = "x-leak=environment"
		values[prefix+"COMPRESSION"] = "gzip"
		values[prefix+"TIMEOUT"] = "1"
		values[prefix+"CERTIFICATE"] = certificate
	}
	values["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"] = elsewhere + "/traces"
	values["OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"] = elsewhere + "/metrics"
	for key, value := range values {
		t.Setenv(key, value)
	}
}

// writeCertificatePEM writes a real certificate so the SDK actually builds
// environment TLS material instead of ignoring an unreadable file.
func writeCertificatePEM(t *testing.T) string {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHTTPExportersIgnoreTheOTLPEnvironment(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("an exporter followed an environment endpoint")
	}))
	defer elsewhere.Close()
	hostileOTLPEnvironment(t, elsewhere.URL)

	var mu sync.Mutex
	var paths []string
	var temporality metricpb.AggregationTemporality
	histograms := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if encoding := request.Header.Get("content-encoding"); encoding != "" {
			t.Errorf("content-encoding = %q, want none", encoding)
		}
		if request.Header.Get("x-leak") != "" {
			t.Error("environment headers reached the collector")
		}
		body := readOTLPBody(t, request)
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, request.URL.Path)
		writer.Header().Set("content-type", "application/x-protobuf")
		if request.URL.Path == defaultMetricPath {
			message := &collectormetric.ExportMetricsServiceRequest{}
			if err := proto.Unmarshal(body, message); err != nil {
				t.Errorf("unmarshal metrics: %v", err)
			}
			for _, resource := range message.ResourceMetrics {
				for _, scope := range resource.ScopeMetrics {
					for _, metric := range scope.Metrics {
						if sum := metric.GetSum(); sum != nil {
							temporality = sum.AggregationTemporality
						}
						if metric.GetExponentialHistogram() != nil {
							t.Error("histogram used the environment's exponential aggregation")
						}
						if metric.GetHistogram() != nil {
							histograms++
						}
					}
				}
			}
			response, _ := proto.Marshal(&collectormetric.ExportMetricsServiceResponse{})
			_, _ = writer.Write(response)
			return
		}
		response, _ := proto.Marshal(&collectortrace.ExportTraceServiceResponse{})
		_, _ = writer.Write(response)
	}))
	defer server.Close()

	config := integrationConfig(ProtocolHTTPProtobuf, strings.TrimPrefix(server.URL, "http://"))
	config.Compression = CompressionNone
	exportTrace(t, config)
	exportMetric(t, config)
	exportHistogram(t, config)

	mu.Lock()
	defer mu.Unlock()
	if len(paths) < 2 || paths[0] != defaultTracePath {
		t.Fatalf("collector paths = %v, want traces first", paths)
	}
	for _, path := range paths[1:] {
		if path != defaultMetricPath {
			t.Fatalf("collector paths = %v, want only the default signal paths", paths)
		}
	}
	if temporality != metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
		t.Fatalf("sum temporality = %s, want cumulative", temporality)
	}
	if histograms == 0 {
		t.Fatal("no explicit-bucket histogram reached the collector")
	}
}

func exportHistogram(t *testing.T, config Config) {
	t.Helper()
	exporter, err := NewMetricExporter(context.Background(), config)
	if err != nil {
		t.Fatalf("NewMetricExporter() error = %v", err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exporter, metric.WithInterval(time.Hour))))
	histogram, err := provider.Meter("integration").Float64Histogram("integration.duration")
	if err != nil {
		t.Fatalf("Float64Histogram() error = %v", err)
	}
	histogram.Record(context.Background(), 0.25)
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestGRPCExportersIgnoreTheOTLPEnvironment(t *testing.T) {
	hostileOTLPEnvironment(t, "http://127.0.0.1:9")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	compressionStats := &compressionStatsHandler{}
	server := grpc.NewServer(grpc.StatsHandler(compressionStats))
	traceService := &traceCollector{}
	metricService := &metricCollector{}
	collectortrace.RegisterTraceServiceServer(server, traceService)
	collectormetric.RegisterMetricsServiceServer(server, metricService)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	config := integrationConfig(ProtocolGRPC, listener.Addr().String())
	config.Compression = CompressionNone
	exportTrace(t, config)
	exportMetric(t, config)
	if traceService.spans.Load() != 1 || metricService.metrics.Load() == 0 {
		t.Fatalf("plaintext gRPC collector received %d spans and %d metrics", traceService.spans.Load(), metricService.metrics.Load())
	}
	if compressionStats.compressed.Load() != 0 {
		t.Fatalf("compressed RPCs = %d, want none", compressionStats.compressed.Load())
	}
}

func TestHTTPExporterKeepsTLSDespiteAnInsecureEnvironment(t *testing.T) {
	hostileOTLPEnvironment(t, "http://127.0.0.1:9")
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = readOTLPBody(t, request)
		writer.Header().Set("content-type", "application/x-protobuf")
		response, _ := proto.Marshal(&collectortrace.ExportTraceServiceResponse{})
		_, _ = writer.Write(response)
	}))
	defer server.Close()

	config := integrationConfig(ProtocolHTTPProtobuf, strings.TrimPrefix(server.URL, "https://"))
	config.TLS = TLSConfig{}
	config.Retry = RetryConfig{}
	config.HTTPClient = &http.Client{Transport: server.Client().Transport, Timeout: time.Second}
	exportTrace(t, config)
}

func TestEndpointURLSelectsSchemeAndSignalPath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		config      Config
		defaultPath string
		want        string
	}{
		{Config{Endpoint: "collector:4318", TLS: TLSConfig{Insecure: true}}, defaultTracePath, "http://collector:4318/v1/traces"},
		{Config{Endpoint: "collector:4318", URLPath: "/custom/v1/metrics"}, defaultMetricPath, "https://collector:4318/custom/v1/metrics"},
		{Config{Endpoint: "collector:4318", URLPath: "custom/v1/traces"}, defaultTracePath, "https://collector:4318/custom/v1/traces"},
		{Config{Endpoint: "[::1]:4318", TLS: TLSConfig{Insecure: true}}, defaultTracePath, "http://[::1]:4318/v1/traces"},
		{Config{Endpoint: "collector", TLS: TLSConfig{Insecure: true}}, defaultTracePath, "http://collector/v1/traces"},
		{Config{Endpoint: "collector:4318", URLPath: " /custom/v1/traces "}, defaultTracePath, "https://collector:4318/custom/v1/traces"},
		{Config{Endpoint: "collector:4318", URLPath: "."}, defaultMetricPath, "https://collector:4318/v1/metrics"},
		{Config{Protocol: ProtocolGRPC, Endpoint: "collector:4317", URLPath: "/ignored"}, defaultTracePath, "https://collector:4317"},
	} {
		got, err := endpointURL(test.config, test.defaultPath)
		if err != nil || got != test.want {
			t.Errorf("endpointURL(%+v) = %q, %v, want %q", test.config, got, err, test.want)
		}
	}
	for _, config := range []Config{
		{Endpoint: "collector :4318"},
		{Endpoint: "::1:4318"},
		{Endpoint: "http://collector:4318"},
		{Endpoint: "user@collector:4318"},
	} {
		if got, err := endpointURL(config, defaultTracePath); !errors.Is(err, errInvalidEndpoint) {
			t.Errorf("endpointURL(%+v) = %q, %v, want errInvalidEndpoint", config, got, err)
		}
	}
	if grpcCompressor(CompressionGZIP) != "gzip" || grpcCompressor(CompressionNone) != "none" {
		t.Fatal("gRPC compressor names do not match the configured compression")
	}
}

func TestExportersRejectAnEndpointThatIsNotAURL(t *testing.T) {
	t.Parallel()

	config := integrationConfig(ProtocolHTTPProtobuf, "collector :4318")
	if _, err := NewTraceExporter(context.Background(), config); !errors.Is(err, errInvalidEndpoint) {
		t.Fatalf("NewTraceExporter() error = %v", err)
	}
	if _, err := NewMetricExporter(context.Background(), config); !errors.Is(err, errInvalidEndpoint) {
		t.Fatalf("NewMetricExporter() error = %v", err)
	}
}
