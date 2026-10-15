// Command service demonstrates explicit HTTP telemetry and signal-driven shutdown.
// It binds HTTP_ADDR (default :8080), reads example environment settings and
// enables both OTLP signals. Running it requires the intended Collector endpoint;
// it is not a production server/readiness or guaranteed-delivery qualification.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	telemetry "github.com/faustbrian/go-telemetry/v3"
	"github.com/faustbrian/go-telemetry/v3/examples/internal/exampleconfig"
	"github.com/faustbrian/go-telemetry/v3/instrumentation/nethttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	config := telemetry.DefaultConfig("example-service", "dev")
	config.Traces.Enabled = true
	config.Metrics.Enabled = true
	if err := applyEnvironment(&config); err != nil {
		return fmt.Errorf("configure telemetry: %w", err)
	}
	runtime, err := telemetry.Init(ctx, config)
	if err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}

	handler, err := nethttp.NewHandler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "application/json")
		_, _ = writer.Write([]byte("{\"status\":\"ok\"}\n"))
	}), nethttp.ServerConfig{
		Operation:      "health.show",
		Route:          "/health",
		TracerProvider: runtime.TracerProvider(),
		MeterProvider:  runtime.MeterProvider(),
		Propagator:     runtime.Propagator(),
	})
	if err != nil {
		return errors.Join(err, runtime.Shutdown(context.Background()))
	}

	server := &http.Server{
		Addr:              environment("HTTP_ADDR", ":8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	// This example waits for signal cancellation; an early listener failure can
	// leave the waiter alive until ctx is canceled or the process exits.
	shutdown := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		shutdown <- errors.Join(server.Shutdown(shutdownCtx), runtime.Shutdown(shutdownCtx))
	}()

	listenErr := server.ListenAndServe()
	if !errors.Is(listenErr, http.ErrServerClosed) {
		return errors.Join(listenErr, runtime.Shutdown(context.Background()))
	}
	return <-shutdown
}

func applyEnvironment(config *telemetry.Config) error {
	config.Environment = environment("DEPLOYMENT_ENVIRONMENT", "local")
	return exampleconfig.ApplyEnvironment(config)
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
