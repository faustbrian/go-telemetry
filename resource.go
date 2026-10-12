package telemetry

import (
	"context"
	"sort"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

var reservedResourceKeys = map[string]struct{}{
	string(semconv.ServiceNameKey):               {},
	string(semconv.ServiceVersionKey):            {},
	string(semconv.ServiceNamespaceKey):          {},
	string(semconv.ServiceInstanceIDKey):         {},
	string(semconv.DeploymentEnvironmentNameKey): {},
	string(semconv.TelemetrySDKLanguageKey):      {},
	string(semconv.TelemetrySDKNameKey):          {},
	string(semconv.TelemetrySDKVersionKey):       {},
}

// BuildResource constructs the resource used by every enabled signal. Service
// identity always wins over custom attributes. Reserved generic entries are
// silently skipped here, whereas Config.Validate and Init reject them. Resource
// values are copied into SDK attributes, not sanitized. No ambient resource
// detectors are enabled. ctx cancellation is checked before construction.
// See TELEMETRY-DEC-006 and the resource/security-admission tests.
func BuildResource(ctx context.Context, config Config) (*resource.Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := config.validateResource(true); err != nil {
		return nil, err
	}
	attributes := make([]attribute.KeyValue, 0, len(config.Resource)+5)
	attributes = append(
		attributes,
		semconv.ServiceName(config.Service.Name),
		semconv.TelemetrySDKLanguageGo,
		semconv.TelemetrySDKName("opentelemetry"),
		semconv.TelemetrySDKVersion(sdk.Version()),
	)
	if config.Service.Version != "" {
		attributes = append(attributes, semconv.ServiceVersion(config.Service.Version))
	}
	if config.Service.Namespace != "" {
		attributes = append(attributes, semconv.ServiceNamespace(config.Service.Namespace))
	}
	if config.Service.Instance != "" {
		attributes = append(attributes, semconv.ServiceInstanceID(config.Service.Instance))
	}
	if config.Environment != "" {
		attributes = append(attributes, semconv.DeploymentEnvironmentNameKey.String(config.Environment))
	}

	keys := make([]string, 0, len(config.Resource))
	for key := range config.Resource {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, reserved := reservedResourceKeys[key]; reserved {
			continue
		}
		attributes = append(attributes, attribute.String(key, config.Resource[key]))
	}

	return resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(attributes...),
	)
}
