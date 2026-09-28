package metric

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestViewValidationDoesNotEchoUntrustedInstrumentName(t *testing.T) {
	const sensitive = "password=private_payload"
	_, err := Options(Config{CardinalityLimit: 100, Views: []ViewConfig{{Name: sensitive}}})
	if err == nil || strings.Contains(err.Error(), sensitive) {
		t.Fatalf("view error disclosed instrument name: %v", err)
	}
}

func TestMetricConfigurationRejectsExcessiveCardinality(t *testing.T) {
	if _, err := Options(Config{CardinalityLimit: 100_001}); err == nil {
		t.Fatal("metric stream cardinality exceeds 100000 retained series")
	}
	if _, err := Options(Config{CardinalityLimit: 100_000}); err != nil {
		t.Fatalf("inclusive cardinality ceiling rejected: %v", err)
	}
}

func TestMetricAllowedAttributeKeysEnforceByteBudget(t *testing.T) {
	config := Config{CardinalityLimit: 100, Views: []ViewConfig{{Name: "latency", AllowedAttributes: []attribute.Key{attribute.Key(strings.Repeat("a", 255))}}}}
	if _, err := Options(config); err != nil {
		t.Fatalf("inclusive key ceiling rejected: %v", err)
	}
	for _, length := range []int{256, 1 << 20} {
		config.Views[0].AllowedAttributes[0] = attribute.Key(strings.Repeat("a", length))
		if _, err := Options(config); err == nil {
			t.Fatalf("%d-byte key admitted", length)
		}
	}
	for _, value := range []string{"password=private_payload" + strings.Repeat("a", 255), "\xff"} {
		config.Views[0].AllowedAttributes[0] = attribute.Key(value)
		if _, err := Options(config); err == nil || strings.Contains(err.Error(), value) {
			t.Fatalf("invalid key admitted or disclosed: %v", err)
		}
	}
}
