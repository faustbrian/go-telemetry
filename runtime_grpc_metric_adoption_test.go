package telemetry

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type runtimeMetricCollector struct {
	collectormetric.UnimplementedMetricsServiceServer
	requests   chan *collectormetric.ExportMetricsServiceRequest
	compressed chan struct{}
}

func (c *runtimeMetricCollector) Export(ctx context.Context, request *collectormetric.ExportMetricsServiceRequest) (*collectormetric.ExportMetricsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("authorization"); len(values) != 1 || values[0] != "test-token" {
		return nil, status.Error(codes.Unauthenticated, "missing test authorization")
	}
	select {
	case c.requests <- proto.Clone(request).(*collectormetric.ExportMetricsServiceRequest):
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return &collectormetric.ExportMetricsServiceResponse{}, nil
}

func (*runtimeMetricCollector) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}
func (c *runtimeMetricCollector) HandleRPC(_ context.Context, event stats.RPCStats) {
	if header, ok := event.(*stats.InHeader); ok && header.Compression == "gzip" {
		select {
		case c.compressed <- struct{}{}:
		default:
		}
	}
}
func (*runtimeMetricCollector) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (*runtimeMetricCollector) HandleConn(context.Context, stats.ConnStats) {}

func TestRuntimeGRPCMetricExporterPreservesPayload(t *testing.T) {
	t.Parallel()
	collector := &runtimeMetricCollector{requests: make(chan *collectormetric.ExportMetricsServiceRequest, 4), compressed: make(chan struct{}, 1)}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.StatsHandler(collector))
	collectormetric.RegisterMetricsServiceServer(server, collector)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				t.Errorf("collector stopped: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("collector did not stop")
		}
	})
	config := DefaultConfig("grpc-adoption", "1.2.3")
	config.Resource = map[string]string{"deployment.zone": "test"}
	config.Metrics.Enabled = true
	config.Metrics.ExportInterval = time.Hour
	config.Metrics.ExportTimeout = 3 * time.Second
	config.Metrics.Exporter.Endpoint = listener.Addr().String()
	config.Metrics.Exporter.TLS.Insecure = true
	config.Metrics.Exporter.Headers = map[string]string{"authorization": "test-token"}
	config.Metrics.Exporter.Timeout = time.Second
	config.ShutdownTimeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runtime, err := Init(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if err := runtime.Shutdown(cleanup); err != nil {
			t.Errorf("runtime cleanup: %v", err)
		}
	})
	counter, err := runtime.Meter("grpc-adoption", metricapi.WithInstrumentationVersion("test-v1")).Int64Counter("adoption.operations", metricapi.WithUnit("1"))
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 9007199254740993, metricapi.WithAttributes(attribute.String("operation", "test")))
	for _, want := range []int64{9007199254740993, 9007199254741000} {
		if err := runtime.ForceFlush(ctx); err != nil {
			t.Fatalf("metric flush: %v", err)
		}
		select {
		case request := <-collector.requests:
			if len(request.ResourceMetrics) != 1 {
				t.Fatalf("resource metrics = %v", request)
			}
			rm := request.ResourceMetrics[0]
			if !metricAttribute(rm.Resource.Attributes, "service.name", "grpc-adoption") || !metricAttribute(rm.Resource.Attributes, "service.version", "1.2.3") || !metricAttribute(rm.Resource.Attributes, "deployment.zone", "test") {
				t.Fatalf("resource identity = %v", rm.Resource)
			}
			if len(rm.ScopeMetrics) != 1 {
				t.Fatalf("scope metrics = %v", rm.ScopeMetrics)
			}
			sm := rm.ScopeMetrics[0]
			if sm.Scope.Name != "grpc-adoption" || sm.Scope.Version != "test-v1" || len(sm.Metrics) != 1 {
				t.Fatalf("instrumentation scope = %v", sm)
			}
			instrument := sm.Metrics[0]
			sum := instrument.GetSum()
			if instrument.Name != "adoption.operations" || instrument.Unit != "1" || sum == nil || !sum.IsMonotonic || sum.AggregationTemporality != metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE || len(sum.DataPoints) != 1 {
				t.Fatalf("counter shape = %v", instrument)
			}
			point := sum.DataPoints[0]
			if _, ok := point.Value.(*metricpb.NumberDataPoint_AsInt); !ok || point.GetAsInt() != want || !metricAttribute(point.Attributes, "operation", "test") {
				t.Fatalf("counter payload = %v, want integer %d", point, want)
			}
		case <-ctx.Done():
			t.Fatal("successful flush delivered no metric request")
		}
		counter.Add(ctx, 7, metricapi.WithAttributes(attribute.String("operation", "test")))
	}
	select {
	case <-collector.compressed:
	default:
		t.Fatal("root configuration did not select gzip")
	}
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("metric shutdown: %v", err)
	}
}

func metricAttribute(values []*commonpb.KeyValue, key, want string) bool {
	for _, value := range values {
		if value.Key == key {
			return value.Value.GetStringValue() == want
		}
	}
	return false
}
