package propagation

import (
	"strings"
	"testing"
)

func TestTrustedBaggageAdmissionBoundsInventoryBeforeParsing(t *testing.T) {
	config := DefaultConfig()
	config.TrustedBaggageKeys = make([]string, 129)
	if _, err := New(config); err == nil || !strings.Contains(err.Error(), "trusted baggage keys exceed") {
		t.Fatalf("oversized inventory error = %v, want bounded admission", err)
	}
}

func TestTrustedBaggageDiagnosticsDoNotEchoUntrustedKeys(t *testing.T) {
	config := DefaultConfig()
	config.TrustedBaggageKeys = []string{"password=private_payload"}
	if _, err := New(config); err == nil || strings.Contains(err.Error(), "private_payload") {
		t.Fatalf("baggage diagnostic disclosed key: %v", err)
	}
}
