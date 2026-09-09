# Compatibility

## Supported matrix

| Component | Supported |
| --- | --- |
| Go | 1.25.x, 1.26.x |
| OpenTelemetry Go API/SDK/exporters | 1.43.x, 1.44.x |
| OTLP | gRPC and HTTP/protobuf Collector endpoints |
| PostgreSQL adapter | pgx/v5 5.10.x |

GitHub Actions test every Go and OpenTelemetry combination. The module's
`go.mod` pins the newest tested SDK line; consumers may select another listed
line through minimal version selection.

Stable compatibility covers exported root, `otlp`, `trace`, `metric`,
`propagation`, instrumentation, and `testtelemetry` APIs; default values;
resource and metric names; propagation and privacy policies; and lifecycle
error behavior.

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
