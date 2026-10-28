package telemetry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/faustbrian/go-telemetry/v4"
)

func TestShutdownAttemptReportsAdmissionWithoutChangingResult(t *testing.T) {
	config := telemetry.DefaultConfig("admission", "test")
	config.RegisterGlobal = false
	config.Traces.Enabled = false
	config.Metrics.Enabled = false
	runtime, err := telemetry.Init(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if started, err := runtime.ShutdownAttempt(ctx); started || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rejected attempt = %v, %v", started, err)
	}
	if started, err := runtime.ShutdownAttempt(context.Background()); !started || err != nil {
		t.Fatalf("admitted attempt = %v, %v", started, err)
	}
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("legacy shutdown did not share result: %v", err)
	}
	if started, err := runtime.ShutdownAttempt(ctx); !started || err != nil {
		t.Fatalf("completed attempt = %v, %v", started, err)
	}
}
