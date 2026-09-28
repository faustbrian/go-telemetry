package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBuildResourceEnforcesDirectCallerAdmission(t *testing.T) {
	config := DefaultConfig("service", "1")
	for index := 0; index <= maxResourceAttributes; index++ {
		config.Resource[strings.Repeat("a", index+1)] = "value"
	}
	if _, err := BuildResource(context.Background(), config); err == nil {
		t.Fatal("direct resource construction accepted oversized input")
	}
}

func TestExporterValidationDoesNotEchoUntrustedEnums(t *testing.T) {
	const sensitive = "password=private_payload"
	config := DefaultConfig("service", "1")
	config.Traces.Enabled = true
	config.Traces.Exporter.Protocol = Protocol(sensitive)
	config.Traces.Exporter.Compression = Compression(sensitive)
	if err := config.Validate(); err == nil || strings.Contains(err.Error(), sensitive) {
		t.Fatalf("validation error disclosed configuration: %v", err)
	}
}

func TestTraceConfigurationRejectsExcessiveQueueAllocation(t *testing.T) {
	config := DefaultConfig("service", "1")
	config.Traces.Enabled = true
	config.Traces.Batch.MaxQueueSize = 65_537
	if err := config.Validate(); err == nil {
		t.Fatal("trace queue exceeds 65536 retained span slots")
	}
	config.Traces.Batch.MaxQueueSize = 65_536
	if err := config.Validate(); err != nil {
		t.Fatalf("inclusive queue ceiling rejected: %v", err)
	}
}

type failingTLSReader struct{}

func (failingTLSReader) ReadFile(context.Context, string, int) ([]byte, error) {
	return nil, errors.New("private TLS material failure")
}

func TestRootConfigDelegatesExporterStringAdmission(t *testing.T) {
	config := DefaultConfig("service", "1")
	config.Traces.Enabled = true
	config.Traces.Exporter.Endpoint = strings.Repeat("a", 4097)
	if err := config.Validate(); err == nil {
		t.Fatal("root exporter bypassed endpoint admission")
	}
}

func TestRootConfigRejectsSamplerModeWithoutDisclosingItsValue(t *testing.T) {
	const sensitive = "password=private_payload"
	config := DefaultConfig("service", "1")
	config.Traces.Enabled = true
	config.Traces.Sampler.Mode = sensitive
	if err := config.Validate(); err == nil || strings.Contains(err.Error(), sensitive) {
		t.Fatal("root configuration accepted or disclosed unsupported sampling mode")
	}
}
