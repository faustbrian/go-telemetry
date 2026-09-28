package otlp

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStandaloneConfigEnforcesHeaderBudgets(t *testing.T) {
	for _, oversized := range []map[string]string{
		{"authorization": strings.Repeat("a", (64<<10)+1)},
	} {
		config := validConfig(ProtocolGRPC)
		config.Headers = oversized
		if err := config.Validate(); err == nil {
			t.Fatal("standalone exporter accepted oversized header payload")
		}
	}
	config := validConfig(ProtocolGRPC)
	config.Headers = make(map[string]string)
	for index := 0; index < 65; index++ {
		config.Headers[strings.Repeat("a", index+1)] = "value"
	}
	if err := config.Validate(); err == nil {
		t.Fatal("standalone exporter accepted oversized header inventory")
	}
}

func TestStandaloneConfigDoesNotEchoUntrustedEnums(t *testing.T) {
	const sensitive = "password=private_payload"
	config := validConfig(ProtocolGRPC)
	config.Protocol = Protocol(sensitive)
	config.Compression = Compression(sensitive)
	if err := config.Validate(); err == nil || strings.Contains(err.Error(), sensitive) {
		t.Fatalf("validation error disclosed configuration: %v", err)
	}
}

func TestTLSConstructionRejectsCancellationBeforeFileWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	config := validConfig(ProtocolGRPC)
	config.TLS = TLSConfig{CAFile: "password=private_payload"}
	if _, err := NewTraceExporter(ctx, config); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled exporter error = %v, want cancellation before file work", err)
	}
}

func TestTLSConstructionDoesNotDiscloseFileDiagnostics(t *testing.T) {
	config := validConfig(ProtocolGRPC)
	config.TLS = TLSConfig{CAFile: "password=private_payload", FileReader: fixtureTLSReader{}}
	if _, err := NewTraceExporter(context.Background(), config); err == nil || strings.Contains(err.Error(), "private_payload") {
		t.Fatalf("TLS diagnostic disclosed path: %v", err)
	}
}

func TestTLSConstructionRejectsOversizedMaterialBeforeParsing(t *testing.T) {
	if _, err := buildTLSConfig(TLSConfig{CAFile: "caller-owned-ca", FileReader: oversizedTLSReader{}}); err == nil || !strings.Contains(err.Error(), "TLS material limit exceeded") {
		t.Fatalf("oversized TLS material error = %v, want admission before parsing", err)
	}
}

// The fixture reader is used only for known task-owned regular certificate
// files. Production readers must additionally own cancellation of their I/O.
type fixtureTLSReader struct{}

func (fixtureTLSReader) ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// #nosec G304 -- path is a caller-owned test fixture and the read is capped
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
}

type oversizedTLSReader struct{}

func (oversizedTLSReader) ReadFile(context.Context, string, int) ([]byte, error) {
	return []byte(strings.Repeat("a", (1<<20)+1)), nil
}

type blockingTLSReader struct{}

func (blockingTLSReader) ReadFile(ctx context.Context, _ string, _ int) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTLSReaderReceivesFiniteConstructorDeadline(t *testing.T) {
	config := validConfig(ProtocolGRPC)
	config.TLS = TLSConfig{CAFile: "caller-owned-ca", FileReader: blockingTLSReader{}}
	if _, err := NewTraceExporter(context.Background(), config); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked material reader error = %v, want finite deadline", err)
	}
}

func TestStandaloneConfigBoundsStringsBeforeExporterConstruction(t *testing.T) {
	for _, field := range []struct {
		name  string
		limit int
		set   func(*Config, string)
	}{
		{"endpoint", 4096, func(c *Config, value string) { c.Endpoint = value }},
		{"URL path", 4096, func(c *Config, value string) { c.URLPath = value }},
		{"server name", 253, func(c *Config, value string) { c.TLS.ServerName = value }},
		{"CA path", 4096, func(c *Config, value string) { c.TLS.CAFile = value }},
		{"certificate path", 4096, func(c *Config, value string) { c.TLS.CertificateFile = value; c.TLS.PrivateKeyFile = "key" }},
		{"key path", 4096, func(c *Config, value string) { c.TLS.PrivateKeyFile = value; c.TLS.CertificateFile = "certificate" }},
	} {
		t.Run(field.name, func(t *testing.T) {
			config := validConfig(ProtocolGRPC)
			config.TLS = TLSConfig{FileReader: fixtureTLSReader{}}
			field.set(&config, strings.Repeat("a", field.limit))
			if err := config.Validate(); err != nil {
				t.Fatalf("inclusive string ceiling rejected: %v", err)
			}
			for _, length := range []int{field.limit + 1, 1 << 20} {
				field.set(&config, strings.Repeat("a", length))
				if err := config.Validate(); err == nil {
					t.Fatalf("%d-byte string admitted", length)
				}
			}
			for _, value := range []string{"password=private_payload" + strings.Repeat("a", field.limit), "\xff"} {
				field.set(&config, value)
				if err := config.Validate(); err == nil || strings.Contains(err.Error(), value) {
					t.Fatalf("invalid string admitted or disclosed: %v", err)
				}
			}
		})
	}
}
