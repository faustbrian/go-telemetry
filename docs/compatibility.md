# Compatibility

V2 callers configuring CA, client-certificate, or private-key paths
must supply `TLS.FileReader`, implementing `otlp.TLSFileReader` with
`ReadFile(ctx context.Context, path string, maxBytes int) ([]byte, error)`.
The runtime forwards that reader to both trace and metric exporters; direct
`otlp` callers set `otlp.TLSConfig.FileReader`. Readers must enforce the
inclusive 1 MiB per-result budget before retaining material and honor
cancellation during blocking I/O. The library rechecks returned lengths and
context state but cannot preempt a non-cooperative reader. Custom CA
material now supplies the entire trust pool rather than augmenting system
roots; include every required authority in that material before upgrading.

Trace queues must not exceed 65,536 entries; metric cardinality must not exceed
100,000 series per stream. Trusted baggage configuration is capped at 128 keys
of 255 bytes each, 128 items, and 1 MiB of propagation-header data. These are
inclusive ceilings; existing smaller deployment budgets remain unchanged.

Exporter endpoint, URL-path, and TLS-path strings are limited to 4 KiB;
server names to 253 bytes; and metric attribute keys to 255 bytes. Keys and
configuration strings must remain valid UTF-8. Server-name limits do not
rewrite IDNA names or replace Go's TLS hostname verification.

## Supported matrix

| Component | Supported |
| --- | --- |
| Go | 1.27.x |
| OpenTelemetry Go API/SDK/exporters | 1.45.x minimum |
| OTLP | gRPC and HTTP/protobuf Collector endpoints |
| PostgreSQL adapter | pgx/v5 5.10.x |

The current dependency tuple uses OpenTelemetry API/SDK and the HTTP metric
exporter at 1.47.0, the gRPC metric exporter at 1.47.0, and OTLP trace
exporters at 1.45.0. GitHub Actions target that tuple on Go 1.27 Linux/amd64,
including HTTP/protobuf and gRPC collector tests; they do not run every
Go/OpenTelemetry combination. Other selections through minimal version
selection need their own validation.

The historical 1.45 minimum does not imply bug-for-bug equivalence with the
selected tuple. SDK 1.45 and 1.46 round some integer counter values above the
exact float64 integer range during aggregation; lossless large counters require
SDK 1.47. This upstream defect is tracked by
[OpenTelemetry issue 8980](https://github.com/open-telemetry/opentelemetry-go/issues/8980)
and fixed in 1.47. The large-integer collector regression passes on the selected
tuple and deliberately fails when the SDK is downgraded to 1.45; do not treat
that downgrade as full-suite validation of the minimum.

Selected SDK 1.47 defaults limit nested span, event, link, and scope attributes
to depth 64, not resource attributes. The selected HTTP metric exporter retains
the 4 MiB decompressed Collector response limit already used in 1.46. Normal
success responses and ordinary shallow attributes remain compatible. The experimental upstream
`OTEL_GO_X_METRIC_EXPORT_BATCH_SIZE` setting is no longer honored, and this
runtime does not expose the replacement reader option. Applications using that
setting must reassess export sizing. These supplier-specific bounds describe
the selected tuple, not identical defaults across every supported selection.

Stable compatibility covers exported root, `otlp`, `trace`, `metric`,
`propagation`, instrumentation, and `testtelemetry` APIs; default values;
resource and metric names; propagation and privacy policies; and lifecycle
error behavior.

## Version 2 migration

Version 2 is published as `github.com/faustbrian/go-telemetry/v2` and
resolves through the public Go proxy. When upgrading from version 1,
migrate imports to `/v2`, explicitly enable required
signals and global registration, and opt into plaintext only for a protected
local or same-trust-zone Collector:

```go
config := telemetry.DefaultConfig("orders", "2.0.0")
config.Traces.Enabled = true
config.Metrics.Enabled = true
config.RegisterGlobal = true
config.Traces.Exporter.TLS.Insecure = true
config.Metrics.Exporter.TLS.Insecure = true
```

An HTTP handler that accepts trusted baggage must additionally prove trust for
each request after authentication:

```go
handler, err := nethttp.NewHandler(next, nethttp.ServerConfig{
    Operation:      "orders.list",
    TrustedInbound: true,
    TrustInbound: func(request *http.Request) bool {
        return authenticatedPeerFromContext(request.Context())
    },
})
```

`TrustInbound` runs for every request and must be concurrency-safe. It must not
grant trust from caller-controlled headers, query parameters, or source address
alone. A panic returns a categorical HTTP 500 without running the business
handler; the callback must return promptly because it cannot be preempted. A
pre-canceled shutdown call no longer consumes the single owned
shutdown attempt; after shutdown begins, every call shares the terminal result.

Update telemetry imports and module requirements together in each consuming
module, including optional compatibility and integration fixtures. Version 1
and `/v2` are distinct module paths; publication does not migrate consumers
automatically.

## Target-oriented package migration

The following released paths remain deprecated compatibility paths. They
preserve public signatures, named-type and sentinel identity, defaults,
instrumentation identity, ownership, concurrency, and error behavior. The HTTP
client path forwards to its target-oriented implementation. Where moving a
released named type or sentinel would change reflection or error identity, the
released path remains the explicit compatibility implementation and the
target-oriented path delegates to it.

| Legacy path | Preferred path |
| --- | --- |
| `instrumentation/gocache` | `instrumentation/cache` |
| `instrumentation/gohttpclient` | `instrumentation/httpclient` |
| `instrumentation/gopostgres` | `instrumentation/postgres` |
| `instrumentation/goqueue` | `instrumentation/queue` |
| `instrumentation/goruntime` | `instrumentation/runtime` |
| `telemetryservice` | `adapters/service` |

The migration changes imports only. Cache integrations may additionally move
from `Instrumenter.Start` to its target-oriented successor
`Instrumenter.Begin`; `Start` delegates to `Begin` and remains compatible.

Collector vendors are compatible when they implement standard OTLP. Vendor
extensions, proprietary authentication helpers, and direct ingestion APIs are
outside the compatibility promise.

OpenTelemetry logs are excluded because the Go log API/SDK stability is not
yet included in this package's promise. See [logs](logs.md).
