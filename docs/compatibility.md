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

GitHub Actions test every Go and OpenTelemetry combination. The module's
`go.mod` pins the newest tested SDK line; consumers may select another listed
line through minimal version selection.

Stable compatibility covers exported root, `otlp`, `trace`, `metric`,
`propagation`, instrumentation, and `testtelemetry` APIs; default values;
resource and metric names; propagation and privacy policies; and lifecycle
error behavior.

## Version 2 migration

The current source uses `github.com/faustbrian/go-telemetry/v2`. Consumers
must stay on the version 1 import path until the immutable `v2.0.0` tag is
published and resolves through the public Go proxy.

After publication, migrate imports to `/v2`, explicitly enable required
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

Direct-consumer migrations remain blocked on publication for `go-webhook`,
`go-queue-control-plane`, and `go-scheduler`. Source, compatibility, or fixture
consumers in `go-authorization`, `go-cloudevents`, `go-correlation`,
`go-idempotency`, `go-library-tools`, `go-service`, and `go-tenancy` must also
move their telemetry imports or fixtures only after version 2 is published.

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
