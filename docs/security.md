# Security

The reporting process is in the repository-level [security policy](../SECURITY.md).
This guide describes deployment controls.

Historical source behavior, corrected boundaries and disclosure limits are
documented in the [published security disposition](security/release-disposition.md).

## Threat model

The versioned repository threat model, trust boundaries, abuse cases, and
accepted residual risks are maintained in [Threat model](threat-model.md).

## Controls

- Validate all configuration before construction.
- Keep default construction inactive: signals, plaintext transport, and global
  registration require separate explicit choices.
- Bound propagation headers, exporter headers, resource attributes, metric
  views, histogram boundaries, queues, batches, retries, timeouts,
  cardinality, and shutdown.
- Use attribute allow-lists and fixed operation names.
- Exclude raw payloads, identifiers, URLs, SQL, cache keys, queue messages, and
  errors from default instrumentation.
- Verify Collector TLS identity and use mTLS or workload authentication where
  appropriate.
- Source headers and private keys from a secret manager or mounted Secret.
- Restrict egress to approved Collector endpoints.
- Apply Collector redaction, authentication, memory limits, queues, and backend
  authorization as independent defenses.

## Trust boundaries

W3C trace context is accepted within a byte bound so distributed traces can
continue across public endpoints. Baggage has a stronger policy: disabled for
untrusted input and filtered at explicitly trusted boundaries. Network location
alone is not authentication. HTTP trusted extraction also requires a
request-specific proof callback. A callback panic fails closed with a
categorical HTTP 500 response; the business handler does not run, and the panic
value is not returned. The callback must return promptly because arbitrary
in-process Go code cannot be preempted safely.

## TLS

Secure transports require TLS 1.2 or later. Configured CA, client certificate,
and private key paths are resolved during `Init` or direct OTLP exporter
construction only through an explicitly supplied `otlp.TLSFileReader`;
construction fails on unreadable, oversized, or malformed material. Client
certificate and key must be supplied together. Protect key file permissions
and rotate credentials through a controlled application restart.

`ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error)`
receives the constructor's finite timeout and caller cancellation, and an
inclusive 1 MiB budget for each CA, certificate, or private key result.
It must enforce that budget before retaining data and propagate the context to
blocking I/O; the package checks cancellation and revalidates every result.
It does not detach work into a background goroutine to mask a blocked reader.
Reader and parser errors never disclose paths or key material by default.

An explicitly supplied CA defines the complete custom trust pool; system roots
are not added. Without a custom CA, standard TLS server verification delegates
platform trust-root handling to Go's TLS implementation.

Exporter endpoints, URL paths, and configured TLS material paths are capped at
4 KiB each; server names are capped at 253 bytes. Metric attribute allow-list
keys use the same 255-byte ceiling as resource keys. These byte limits apply
before SDK parsing, provider calls, or key hashing. They do not grant filesystem
access or promise path containment: the supplied reader owns access policy.

## Dependency and code safety

CI runs `govulncheck`, lint security checks, a Go/OpenTelemetry compatibility
matrix, protocol failure tests, and selected risk checks. Production code is
scanned for cgo, `unsafe`, and `go:linkname`.
