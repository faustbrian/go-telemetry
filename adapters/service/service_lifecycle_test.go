package telemetryservice_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	service "github.com/faustbrian/go-service/v3"
	"github.com/faustbrian/go-telemetry/v4"
	telemetryservice "github.com/faustbrian/go-telemetry/v4/adapters/service"
)

func TestServiceOwnsOneRuntimeAndPreservesShutdownFailure(t *testing.T) {
	shutdownErr := errors.New("exporter shutdown failed")
	exporter := &failingSpanExporter{err: shutdownErr}
	config := localConfig()
	config.Traces.Enabled = true
	adapter, err := telemetryservice.New(telemetryservice.Options{
		Name: "telemetry", Config: config, Failure: telemetryservice.FailureRequired,
		RuntimeOptions: []telemetry.Option{telemetry.WithTraceExporter(exporter)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, active := adapter.Runtime(); active {
		t.Fatal("construction acquired a runtime")
	}
	runtime, err := service.New(service.Config{Components: []service.Component{adapter.Component()}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current, active := adapter.Runtime(); current == nil || !active {
		t.Fatal("started runtime was not exposed")
	}
	if err := runtime.Shutdown(context.Background()); !errors.Is(err, shutdownErr) {
		t.Fatalf("shutdown = %v", err)
	}
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() {
			if err := adapter.Component().Stop(context.Background()); !errors.Is(err, shutdownErr) {
				t.Errorf("repeated shutdown = %v", err)
			}
		})
	}
	callers.Wait()
	if err := adapter.Component().Stop(context.Background()); !errors.Is(err, shutdownErr) {
		t.Fatalf("sequential shutdown = %v", err)
	}
	if _, active := adapter.Runtime(); active {
		t.Fatal("stopped runtime remains exposed")
	}
	if exporter.shutdowns != 1 {
		t.Fatalf("exporter shutdowns = %d", exporter.shutdowns)
	}
}

func TestServicePreservesRequiredAndBestEffortInitialization(t *testing.T) {
	for _, policy := range []telemetryservice.FailurePolicy{telemetryservice.FailureRequired, telemetryservice.FailureBestEffort} {
		t.Run(string(policy), func(t *testing.T) {
			config := localConfig()
			config.ShutdownTimeout = -1
			adapter, err := telemetryservice.New(telemetryservice.Options{Name: "telemetry", Config: config, Failure: policy})
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := service.New(service.Config{Components: []service.Component{adapter.Component()}})
			if err != nil {
				t.Fatal(err)
			}
			startErr := runtime.Start(context.Background())
			if (policy == telemetryservice.FailureRequired && startErr == nil) || (policy == telemetryservice.FailureBestEffort && startErr != nil) {
				t.Fatalf("startup = %v for %s", startErr, policy)
			}
			var retained *telemetryservice.InitializationError
			if !errors.As(adapter.InitializationError(), &retained) || errors.Unwrap(retained) == nil {
				t.Fatal("initialization cause was lost")
			}
			if _, active := adapter.Runtime(); active {
				t.Fatal("failed initialization exposes a runtime")
			}
			if err := runtime.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServicePreservesCanceledInitialization(t *testing.T) {
	adapter, err := telemetryservice.New(telemetryservice.Options{Name: "telemetry", Config: localConfig(), Failure: telemetryservice.FailureRequired})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapter.Component().Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
	if !errors.Is(adapter.InitializationError(), context.Canceled) {
		t.Fatal("cancellation cause was lost")
	}
	if _, active := adapter.Runtime(); active {
		t.Fatal("canceled initialization exposes a runtime")
	}
	if err := adapter.Component().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
