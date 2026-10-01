package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestBuildResourceOwnsServiceIdentity(t *testing.T) {
	t.Parallel()

	config := DefaultConfig("orders", "1.2.3")
	config.Service.Namespace = "commerce"
	config.Service.Instance = "orders-7d9f"
	config.Environment = "production"
	config.Resource["region"] = "eu-north-1"

	res, err := BuildResource(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildResource() error = %v", err)
	}

	if res.SchemaURL() != semconv.SchemaURL {
		t.Errorf("SchemaURL() = %q, want %q", res.SchemaURL(), semconv.SchemaURL)
	}
	want := map[attribute.Key]string{
		semconv.ServiceNameKey:               "orders",
		semconv.ServiceVersionKey:            "1.2.3",
		semconv.ServiceNamespaceKey:          "commerce",
		semconv.ServiceInstanceIDKey:         "orders-7d9f",
		semconv.DeploymentEnvironmentNameKey: "production",
		attribute.Key("region"):              "eu-north-1",
		semconv.TelemetrySDKLanguageKey:      "go",
		semconv.TelemetrySDKNameKey:          "opentelemetry",
		semconv.TelemetrySDKVersionKey:       selectedSDKVersion(t),
	}
	for key, value := range want {
		got, ok := res.Set().Value(key)
		if !ok {
			t.Errorf("resource attribute %q is missing", key)
			continue
		}
		if got.Type() != attribute.STRING || got.AsString() != value {
			t.Errorf("resource attribute %q = %q, want %q", key, got.AsString(), value)
		}
	}
}

func TestConfigValidationRejectsReservedResourceAttributes(t *testing.T) {
	t.Parallel()

	for _, key := range []attribute.Key{semconv.ServiceNameKey, semconv.TelemetrySDKVersionKey} {
		t.Run(string(key), func(t *testing.T) {
			config := DefaultConfig("orders", "1.2.3")
			config.Resource[string(key)] = "attacker-controlled"
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want reserved attribute error")
			}
		})
	}
}

func TestBuildResourceIgnoresReservedCustomAttributes(t *testing.T) {
	t.Parallel()

	config := DefaultConfig("orders", "1.2.3")
	config.Resource[string(semconv.ServiceNameKey)] = "untrusted"
	config.Resource[string(semconv.TelemetrySDKVersionKey)] = "poison-sdk-version"
	config.Resource["zz.region"] = "eu-north-1"
	res, err := BuildResource(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildResource() error = %v", err)
	}
	value, _ := res.Set().Value(semconv.ServiceNameKey)
	if value.AsString() != "orders" {
		t.Fatalf("service.name = %q, want owned identity", value.AsString())
	}
	for key, want := range map[attribute.Key]string{
		semconv.TelemetrySDKLanguageKey: "go",
		semconv.TelemetrySDKNameKey:     "opentelemetry",
		semconv.TelemetrySDKVersionKey:  selectedSDKVersion(t),
	} {
		got, ok := res.Set().Value(key)
		if !ok || got.Type() != attribute.STRING || got.AsString() != want {
			t.Errorf("owned SDK attribute %q = %v/%t, want %q", key, got, ok, want)
		}
	}
	value, ok := res.Set().Value(attribute.Key("zz.region"))
	if !ok || value.AsString() != "eu-north-1" {
		t.Fatalf("zz.region = %q/%t, want post-reserved custom attribute", value.AsString(), ok)
	}
}

// selectedSDKVersion obtains the identity from the SDK-owned detector rather
// than the independently versioned OpenTelemetry API.
func selectedSDKVersion(t *testing.T) string {
	t.Helper()
	res, err := resource.New(context.Background(), resource.WithTelemetrySDK())
	if err != nil {
		t.Fatalf("SDK resource detector error = %v", err)
	}
	value, ok := res.Set().Value(semconv.TelemetrySDKVersionKey)
	if !ok || value.Type() != attribute.STRING || value.AsString() == "" {
		t.Fatalf("SDK resource detector version = %v/%t", value, ok)
	}
	return value.AsString()
}
