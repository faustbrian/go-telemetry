# Upgrade guide

The current source is the unpublished `/v2` module. Released consumers must
remain on version 1.2.0 until a `v2` tag exists, then follow the
[planned version 2 migration](compatibility.md#planned-version-2).

## Before upgrading

1. Read `CHANGELOG.md` for changed defaults, metrics, attributes, and errors.
2. Run `go test`, `go test -race`, and protocol integration tests.
3. Compare benchmark and cardinality baselines.
4. Deploy through a canary Collector pipeline and watch exporter failures,
   queue pressure, process memory, and backend ingestion.

## OpenTelemetry dependencies

Upgrade all OTel API, SDK, and OTLP exporter modules together. Run the
compatibility script in a disposable checkout:

```sh
./scripts/test-otel-version.sh v1.45.0
```

The script modifies `go.mod`; do not run it over unrelated uncommitted module
changes. CI runs each matrix entry in an isolated checkout.

## Configuration changes

Treat endpoint, TLS, retry, timeout, sampling, metric views, cardinality,
propagation allow-lists, and instrumentation operation names as operational
contracts. Roll them out independently from application behavior where
possible.

Version 2 does not read configured TLS paths from the filesystem itself. If
either signal configures `CAFile`, `CertificateFile`, or `PrivateKeyFile`, set
that signal's `Exporter.TLS.FileReader` to a caller-owned implementation of
`otlp.TLSFileReader`; direct `otlp` callers set `TLSConfig.FileReader`.
Its method is
`ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error)`.
It must honor cancellation during blocking I/O and reject
material exceeding the inclusive `maxBytes` budget before retaining it.
Construction passes 1 MiB for each CA, certificate, and key result, rechecks
returned sizes, and uses a finite timeout. An arbitrary reader that ignores
the context can still block construction, so audit its I/O behavior before
deployment. A custom CA replaces, rather than extends, the system trust pool;
include all required authorities in the supplied PEM.

## Rollback

Restore the previous module version and configuration together. OpenTelemetry
wire data remains OTLP, so no vendor SDK migration is needed. When metric views
or names change, keep dashboards compatible with both versions during the
rollout window.
