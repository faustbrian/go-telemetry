package otlp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	collectormetric "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestHTTPMetricExporterPreservesNumericPayload(t *testing.T) {
	t.Parallel()
	requests := make(chan *collectormetric.ExportMetricsServiceRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/adoption/metrics" || request.Header.Get("content-encoding") != "gzip" {
			t.Error("metric path or compression was not preserved")
		}
		message := &collectormetric.ExportMetricsServiceRequest{}
		if err := proto.Unmarshal(readOTLPBody(t, request), message); err != nil {
			t.Errorf("metric protobuf: %v", err)
			return
		}
		requests <- message
		writer.Header().Set("content-type", "application/x-protobuf")
	}))
	defer server.Close()
	config := integrationConfig(ProtocolHTTPProtobuf, strings.TrimPrefix(server.URL, "http://"))
	config.URLPath = "/adoption/metrics"
	data := metricHTTPFixture(t)
	if err := exportMetricHTTPFixture(context.Background(), config, &data); err != nil {
		t.Fatalf("metric export: %v", err)
	}
	select {
	case request := <-requests:
		if len(request.ResourceMetrics) != 1 {
			t.Fatalf("resource count = %d, want 1", len(request.ResourceMetrics))
		}
		rm := request.ResourceMetrics[0]
		if len(rm.Resource.Attributes) != 1 || rm.Resource.Attributes[0].Key != "service.name" || rm.Resource.Attributes[0].Value.GetStringValue() != "metric-adoption" {
			t.Fatalf("resource identity = %v", rm.Resource)
		}
		if len(rm.ScopeMetrics) != 1 || rm.ScopeMetrics[0].Scope.Name != "metric-adoption" || len(rm.ScopeMetrics[0].Metrics) != 1 {
			t.Fatalf("scope metrics = %v", rm.ScopeMetrics)
		}
		instrument := rm.ScopeMetrics[0].Metrics[0]
		sum := instrument.GetSum()
		if instrument.Name != "adoption.operations" || instrument.Unit != "1" || sum == nil || !sum.IsMonotonic || len(sum.DataPoints) != 1 {
			t.Fatalf("metric shape = %v", instrument)
		}
		if value := sum.DataPoints[0].GetAsInt(); value != 9007199254740993 {
			t.Fatalf("integer metric = %d, want 9007199254740993", value)
		}
	default:
		t.Fatal("successful export delivered no metric request")
	}
}

func TestHTTPMetricExporterFailureModes(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name     string
		status   int
		failures int64
		body     []byte
		wantErr  bool
	}{
		{name: "retry rate limit", status: http.StatusTooManyRequests, failures: 2},
		{name: "permanent rejection", status: http.StatusBadRequest, failures: 10, wantErr: true},
		{name: "malformed response", status: http.StatusOK, body: []byte{0xff}, wantErr: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var attempts atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				attempt := attempts.Add(1)
				writer.Header().Set("content-type", "application/x-protobuf")
				if attempt <= scenario.failures {
					writer.Header().Set("Retry-After", "0")
					writer.WriteHeader(scenario.status)
					return
				}
				_, _ = writer.Write(scenario.body)
			}))
			defer server.Close()
			config := integrationConfig(ProtocolHTTPProtobuf, strings.TrimPrefix(server.URL, "http://"))
			data := metricHTTPFixture(t)
			err := exportMetricHTTPFixture(context.Background(), config, &data)
			if (err != nil) != scenario.wantErr {
				t.Fatalf("metric export error = %v, want error %v", err, scenario.wantErr)
			}
			wantAttempts := int64(1)
			if !scenario.wantErr {
				wantAttempts += scenario.failures
			}
			if attempts.Load() != wantAttempts {
				t.Fatalf("attempts = %d, want %d", attempts.Load(), wantAttempts)
			}
		})
	}
}

func TestHTTPMetricExporterHonorsCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	for _, cancelCaller := range []bool{false, true} {
		name := "configured timeout"
		if cancelCaller {
			name = "caller cancellation"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				close(started)
				select {
				case <-request.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			config := integrationConfig(ProtocolHTTPProtobuf, strings.TrimPrefix(server.URL, "http://"))
			config.Retry.Enabled = false
			if !cancelCaller {
				config.Timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			data := metricHTTPFixture(t)
			done := make(chan error, 1)
			go func() { done <- exportMetricHTTPFixture(ctx, config, &data) }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("metric request never reached collector")
			}
			if cancelCaller {
				cancel()
			}
			select {
			case err := <-done:
				want := context.DeadlineExceeded
				if cancelCaller {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Fatalf("metric export error = %v, want %v", err, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("metric export ignored cancellation or configured timeout")
			}
		})
	}
}

func metricHTTPFixture(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader), metric.WithResource(resource.NewSchemaless(attribute.String("service.name", "metric-adoption"))))
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("fixture shutdown: %v", err)
		}
	}()
	counter, err := provider.Meter("metric-adoption").Int64Counter("adoption.operations", metricapi.WithUnit("1"))
	if err != nil {
		t.Fatalf("fixture counter: %v", err)
	}
	counter.Add(context.Background(), 9007199254740993)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("fixture collection: %v", err)
	}
	return data
}

func exportMetricHTTPFixture(ctx context.Context, config Config, data *metricdata.ResourceMetrics) error {
	exporter, err := NewMetricExporter(ctx, config)
	if err != nil {
		return err
	}
	exportErr := exporter.Export(ctx, data)
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return errors.Join(exportErr, exporter.Shutdown(cleanup))
}
