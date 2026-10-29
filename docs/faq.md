# FAQ

[Documentation map](index.md) · [Repository README](../README.md)
[Next: Architecture](architecture.md)

## Does this replace OpenTelemetry?

No. It constructs and returns standard OpenTelemetry providers, tracers,
meters, propagators, exporters, samplers, and views.

## Does it support a specific vendor?

It supports vendors through a standard OTLP Collector pipeline. Vendor SDK
wrappers and proprietary direct-ingestion defaults are out of scope.

## Why is initialization explicit?

Configuration values should be safe to build in tests and dependency wiring.
Network clients, goroutines, SDK providers, and registered providers and
propagators appear only after `Init` succeeds. The one exception is diagnostics:
`Init` first replaces the process-wide OpenTelemetry error handler and internal
logger with content-free ones once `Config` validates, even when
`RegisterGlobal` is false or construction later fails, so SDK error text never
reaches stderr. Exporters built directly with the `otlp` package without
`Init` do not get this protection.

## Must I register globals?

No. Set `RegisterGlobal = false` and pass returned providers explicitly. Global
registration is convenient for compatible libraries but requires one process
owner.

## Why is baggage disabled?

Baggage crosses service boundaries and can leak or amplify untrusted data.
Enable it only for authenticated peers with allow-listed keys and hard bounds.

## Can I record raw errors or SQL in development?

The supplied adapters do not. Add application-owned diagnostic logging only
with deliberate redaction and access controls; do not change production
telemetry contracts for convenience.

## Why not retry forever?

An unavailable telemetry backend must not consume unbounded memory, network,
CPU, or shutdown time. Collector queues handle longer buffering with explicit
operational limits.

## Are logs supported?

Not as a stable signal. See [log stability](logs.md). Structured application
logging remains independent.

## How do I test without a Collector?

Use `testtelemetry` for deterministic providers. OTLP protocol tests already
run in-process; use `make -f verification/package.mk integration` when changing
transport behavior. This is not deployed Collector or live PostgreSQL proof.

## How is test coverage assessed?

Coverage follows the changed behavior and its material risk, without a
universal statement percentage. Protocol, privacy, failure, and vulnerability
checks cover properties that statement execution cannot prove. Race, fuzz,
and benchmark gates add evidence when a change has the corresponding risk.
