# Upgrade guide

[Documentation map](index.md) · [Repository README](../README.md)
[Next: Architecture](architecture.md)

Version 3 uses the `/v3` module path. Follow the
[version 3 migration](compatibility.md#version-3-migration) for removed packages,
component methods and changed instrumentation-scope selectors.

## Before upgrading

1. Read `CHANGELOG.md` for changed defaults, metrics, attributes, and errors.
2. Run `go test`, `go test -race`, and protocol integration tests.
3. Compare benchmark and cardinality baselines.
4. Deploy through a canary Collector pipeline and watch exporter failures,
   queue pressure, process memory, and backend ingestion.

## OpenTelemetry dependencies

Select a compatible OTel API, SDK, and exporter tuple and validate both
transports. The [compatibility guide](compatibility.md#supported-matrix)
describes the current tested tuple. To qualify a uniform version selection,
run the compatibility script from the repository root:

```sh
./scripts/test-otel-version.sh v1.45.0
```

The script uses disposable module files, verifies that every selected API,
SDK, and exporter resolves to the requested version without replacements,
and leaves `go.mod` and `go.sum` unchanged. Its 1.45/1.46 rows exercise common
behavior with ordinary integer counters and zero-threshold histograms. They
do not claim the selected 1.47 lossless large-counter and nonzero-threshold
repairs. Ordinary tests retain those strict selected-version assertions;
the uniform script selection may differ from the current mixed tuple.
CI does not run a version matrix.

## Configuration changes

Treat endpoint, TLS, retry, timeout, sampling, metric views, cardinality,
propagation allow-lists, and instrumentation operation names as operational
contracts. Roll them out independently from application behavior where
possible.

The runtime does not read configured TLS paths from the filesystem itself. If
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

Restore the previous application/module version, configuration and operational
scope selectors together. OpenTelemetry wire data remains OTLP. A rollback is
a return to an earlier release, not an in-process compatibility layer or dual
instrumentation engine.
