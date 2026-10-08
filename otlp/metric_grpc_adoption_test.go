package otlp

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// The collector owns each request until the handler returns. Tests retaining a
// payload clone it rather than sharing mutable protobuf state across RPCs.
type adoptionMetricCollector struct {
	collectormetric.UnimplementedMetricsServiceServer
	export func(context.Context, *collectormetric.ExportMetricsServiceRequest) error
}

func (c *adoptionMetricCollector) Export(ctx context.Context, request *collectormetric.ExportMetricsServiceRequest) (*collectormetric.ExportMetricsServiceResponse, error) {
	if err := c.export(ctx, request); err != nil {
		return nil, err
	}
	return &collectormetric.ExportMetricsServiceResponse{}, nil
}

func metricGRPCCollector(t *testing.T, export func(context.Context, *collectormetric.ExportMetricsServiceRequest) error) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	collectormetric.RegisterMetricsServiceServer(server, &adoptionMetricCollector{export: export})
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
	return listener.Addr().String()
}

func TestGRPCMetricExporterPreservesZeroThreshold(t *testing.T) {
	t.Parallel()
	requests := make(chan *collectormetric.ExportMetricsServiceRequest, 1)
	endpoint := metricGRPCCollector(t, func(_ context.Context, request *collectormetric.ExportMetricsServiceRequest) error {
		requests <- proto.Clone(request).(*collectormetric.ExportMetricsServiceRequest)
		return nil
	})
	data := metricHTTPFixture(t)
	now := time.Now()
	data.ScopeMetrics[0].Metrics = []metricdata.Metrics{{
		Name: "adoption.exponential", Unit: "1",
		Data: metricdata.ExponentialHistogram[float64]{
			Temporality: metricdata.CumulativeTemporality,
			DataPoints: []metricdata.ExponentialHistogramDataPoint[float64]{{
				StartTime: now.Add(-time.Second), Time: now,
				Count: 2, Sum: 2.1, Scale: 0, ZeroCount: 1, ZeroThreshold: 0.5,
				PositiveBucket: metricdata.ExponentialBucket{Offset: 0, Counts: []uint64{1}},
			}},
		},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := exportMetricHTTPFixture(ctx, integrationConfig(ProtocolGRPC, endpoint), &data); err != nil {
		t.Fatalf("metric export: %v", err)
	}
	select {
	case request := <-requests:
		if len(request.ResourceMetrics) != 1 || len(request.ResourceMetrics[0].ScopeMetrics) != 1 || len(request.ResourceMetrics[0].ScopeMetrics[0].Metrics) != 1 {
			t.Fatalf("unexpected metric shape: %v", request)
		}
		metric := request.ResourceMetrics[0].ScopeMetrics[0].Metrics[0]
		histogram := metric.GetExponentialHistogram()
		if metric.Name != "adoption.exponential" || metric.Unit != "1" || histogram == nil || len(histogram.DataPoints) != 1 {
			t.Fatalf("exponential histogram = %v", metric)
		}
		point := histogram.DataPoints[0]
		if point.ZeroThreshold != 0.5 {
			t.Fatalf("zero threshold = %v, want 0.5", point.ZeroThreshold)
		}
		if point.ZeroCount != 1 || point.Count != 2 || point.GetSum() != 2.1 || point.Scale != 0 || point.Positive.Offset != 0 || len(point.Positive.BucketCounts) != 1 || point.Positive.BucketCounts[0] != 1 {
			t.Fatalf("histogram payload = %v", point)
		}
	default:
		t.Fatal("successful export delivered no histogram")
	}
}

func TestGRPCMetricExporterFailureModes(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name               string
		code               codes.Code
		failures, attempts int
		retry, wantErr     bool
	}{
		{name: "retry transient rejection", code: codes.Unavailable, failures: 2, attempts: 3, retry: true},
		{name: "permanent rejection", code: codes.InvalidArgument, failures: 10, attempts: 1, retry: true, wantErr: true},
		{name: "retry disabled", code: codes.Unavailable, failures: 10, attempts: 1, wantErr: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var mu sync.Mutex
			var requests []*collectormetric.ExportMetricsServiceRequest
			endpoint := metricGRPCCollector(t, func(_ context.Context, request *collectormetric.ExportMetricsServiceRequest) error {
				mu.Lock()
				defer mu.Unlock()
				requests = append(requests, proto.Clone(request).(*collectormetric.ExportMetricsServiceRequest))
				if len(requests) <= scenario.failures {
					return status.Error(scenario.code, "controlled rejection")
				}
				return nil
			})
			config := integrationConfig(ProtocolGRPC, endpoint)
			config.Retry.Enabled = scenario.retry
			data := metricHTTPFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := exportMetricHTTPFixture(ctx, config, &data)
			if (err != nil) != scenario.wantErr {
				t.Fatalf("export error = %v, want error %v", err, scenario.wantErr)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != scenario.attempts {
				t.Fatalf("attempts = %d, want %d", len(requests), scenario.attempts)
			}
			for _, request := range requests[1:] {
				if !proto.Equal(requests[0], request) {
					t.Fatal("metric payload changed between retries")
				}
			}
			point := requests[0].ResourceMetrics[0].ScopeMetrics[0].Metrics[0].GetSum().DataPoints[0]
			if point.GetAsInt() != 9007199254740993 {
				t.Fatalf("exported integer = %d", point.GetAsInt())
			}
		})
	}
}

func TestGRPCMetricExporterHonorsCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	for _, cancelCaller := range []bool{false, true} {
		name := "configured timeout"
		if cancelCaller {
			name = "caller cancellation"
		}
		t.Run(name, func(t *testing.T) {
			started, ended := make(chan struct{}), make(chan struct{})
			endpoint := metricGRPCCollector(t, func(ctx context.Context, _ *collectormetric.ExportMetricsServiceRequest) error {
				close(started)
				<-ctx.Done()
				close(ended)
				return status.FromContextError(ctx.Err()).Err()
			})
			config := integrationConfig(ProtocolGRPC, endpoint)
			config.Retry.Enabled = false
			if !cancelCaller {
				config.Timeout = 250 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			data := metricHTTPFixture(t)
			done := make(chan error, 1)
			go func() { done <- exportMetricHTTPFixture(ctx, config, &data) }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("metric RPC never reached collector")
			}
			if cancelCaller {
				cancel()
			}
			select {
			case err := <-done:
				wantCode, wantContext := codes.DeadlineExceeded, context.DeadlineExceeded
				if cancelCaller {
					wantCode, wantContext = codes.Canceled, context.Canceled
				}
				if status.Code(err) != wantCode && !errors.Is(err, wantContext) {
					t.Fatalf("export error = %v, want %v", err, wantCode)
				}
				if !cancelCaller && ctx.Err() != nil {
					t.Fatal("outer caller expired before exporter timeout")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("export ignored cancellation or timeout")
			}
			select {
			case <-ended:
			case <-time.After(time.Second):
				t.Fatal("collector RPC remained active")
			}
		})
	}
}
