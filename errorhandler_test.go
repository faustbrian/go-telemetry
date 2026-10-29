package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestInitDiscardsSDKErrorText(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	config := DefaultConfig("errorhandler", "1.0.0")
	runtime, err := Init(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })

	otel.Handle(errors.New(`traces export: Post "http://collector.internal:4318/v1/traces": dial tcp 10.96.0.20:4318: connection refused`))
	if output.Len() != 0 {
		t.Fatalf("SDK error text reached the process log: %q", output.String())
	}
	if _, ok := otel.GetErrorHandler().(contentFreeErrorHandler); !ok {
		t.Fatalf("error handler = %T, want the content-free handler", otel.GetErrorHandler())
	}
}
