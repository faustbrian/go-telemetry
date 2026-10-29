package otlp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// hostileOTLPEnvironment sets every OTEL_EXPORTER_OTLP_* variable an SDK
// exporter would honor to a value that contradicts the explicit Config.
func hostileOTLPEnvironment(t *testing.T, elsewhere string) {
	t.Helper()
	for key, value := range map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":                              elsewhere,
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":                       elsewhere + "/traces",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT":                      elsewhere + "/metrics",
		"OTEL_EXPORTER_OTLP_INSECURE":                              "true",
		"OTEL_EXPORTER_OTLP_HEADERS":                               "x-leak=environment",
		"OTEL_EXPORTER_OTLP_COMPRESSION":                           "gzip",
		"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION":                    "gzip",
		"OTEL_EXPORTER_OTLP_METRICS_COMPRESSION":                   "gzip",
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE":        "delta",
		"OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION": "base2_exponential_bucket_histogram",
	} {
		t.Setenv(key, value)
	}
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
		{Config{Endpoint: "collector:4317", URLPath: "/ignored"}, "", "https://collector:4317"},
	} {
		if got := endpointURL(test.config, test.defaultPath); got != test.want {
			t.Errorf("endpointURL(%+v) = %q, want %q", test.config, got, test.want)
		}
	}
	if grpcCompressor(CompressionGZIP) != "gzip" || grpcCompressor(CompressionNone) != "none" {
		t.Fatal("gRPC compressor names do not match the configured compression")
	}
}
