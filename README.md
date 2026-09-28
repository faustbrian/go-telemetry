# telemetry

[![CI](https://github.com/faustbrian/go-telemetry/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/faustbrian/go-telemetry/actions/workflows/ci.yml)
[![CodeQL](https://img.shields.io/badge/CodeQL-required-blue)](https://github.com/faustbrian/go-telemetry/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-risk_based-blue)](CONTRIBUTING.md#verification)
[![Documentation](https://img.shields.io/badge/docs-checked_in_CI-blue)](docs/)
[![Go Reference](https://pkg.go.dev/badge/github.com/faustbrian/go-telemetry/v2.svg)](https://pkg.go.dev/github.com/faustbrian/go-telemetry/v2)
[![Release](https://img.shields.io/github/v/release/faustbrian/go-telemetry?sort=semver)](https://github.com/faustbrian/go-telemetry/releases)
[![Go](https://img.shields.io/badge/go-1.27.0-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`telemetry` is a vendor-neutral OpenTelemetry runtime for Go services. It
owns resource identity, trace and metric providers, OTLP exporters,
propagation, sampling, global registration, flush, and shutdown while returning
the standard OpenTelemetry APIs.

The package targets an OpenTelemetry Collector. It does not wrap vendor SDKs,
replace OpenTelemetry types, read configuration implicitly, or add log-signal
stability promises.

## Requirements

- Go 1.27
- OpenTelemetry Go 1.45.x or later compatible releases
- an OTLP-compatible Collector for production export

## Installation

```sh
go get github.com/faustbrian/go-telemetry/v2@v2.0.0
```

The v2 module has breaking security defaults. Resolve the public `v2.0.0`
tag before adopting it; until that tag is available, remain on the released
v1 module. See the [migration notes](docs/compatibility.md#version-2-migration).

## Quick start

```go
package main

import (
	"context"
	"log"

	telemetry "github.com/faustbrian/go-telemetry/v2"
)

func main() {
	config := telemetry.DefaultConfig("orders", "2.0.0")
	config.Environment = "local"
	config.Traces.Enabled = true
	config.Metrics.Enabled = true
	config.Traces.Exporter.Endpoint = "localhost:4317"
	config.Metrics.Exporter.Endpoint = "localhost:4317"
	config.Traces.Exporter.TLS.Insecure = true
	config.Metrics.Exporter.TLS.Insecure = true

	runtime, err := telemetry.Init(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}

	ctx, span := runtime.Tracer("orders").Start(context.Background(), "orders.list")
	defer span.End()
	_ = ctx

	if err := runtime.Shutdown(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

`DefaultConfig` only builds a value. Network clients, providers, globals, and
goroutines are created by `Init`. `Runtime` exposes standard tracer, meter, and
propagator interfaces. `Shutdown` is idempotent, propagates the shorter caller
or configured deadline to providers and exporters, restores only globals still
owned by the runtime, and joins provider and exporter failures. As with the
standard OpenTelemetry interfaces, a custom exporter must cooperate with
context cancellation for that deadline to bound its call.

## Version 2 safe defaults

| Setting | Default |
| --- | --- |
| signals | traces and metrics disabled |
| transport | OTLP/gRPC to `localhost:4317` |
| transport security | TLS; plaintext requires explicit opt-in |
| compression | gzip |
| trace sampling | parent-based 10% ratio |
| span queue / batch | 2,048 / 512 |
| metric cardinality | 1,000 points per instrument |
| baggage | disabled |
| shutdown timeout | 10 seconds |

Every default is represented in `Config` and can be inspected or overridden.
Enabling a signal creates its exporter and background SDK processing during
`Init`. Global registration and plaintext transport are separate explicit
choices. Production clusters should configure TLS; plaintext is intended only
for an authenticated, policy-protected local or same-cluster connection.

## Packages

- root: configuration, resources, provider lifecycle, globals, and errors
- `otlp`: explicit OTLP/gRPC and OTLP/HTTP exporter construction
- `trace`: always-on, always-off, ratio, and parent-based samplers
- `metric`: views, histogram boundaries, attribute allow-lists, and cardinality
- `propagation`: bounded W3C trace context and trusted baggage policies
- `instrumentation/nethttp`: private-by-default `net/http` server and client
- `instrumentation/httpclient`: `http-client` RoundTripper adapter
- `instrumentation/postgres`: pgx query tracer for `postgres`
- `instrumentation/cache`: dependency-neutral `cache` observations
- `instrumentation/queue`: dependency-neutral `queue` handler wrapper
- `instrumentation/runtime`: caller-owned Go runtime metrics registration
- `adapters/service`: explicit `service` lifecycle initialization and shutdown
- `testtelemetry`: deterministic in-memory providers and snapshots

The former `instrumentation/go{cache,httpclient,postgres,queue,runtime}` and
`telemetryservice` paths remain source-compatible. The HTTP client path is a
forwarding facade; packages with released named-type or sentinel identity stay
the explicit compatibility implementations used by their target-oriented
successors. New code should use the target-oriented paths above; see the
[compatibility guide](docs/compatibility.md) for the migration map.

Instrumentation never records raw URL paths, queries, hosts, headers, client
addresses, SQL, query arguments, database error text, cache keys or values,
queue messages, raw handler errors, or panic values by default.

## Service lifecycle

`adapters/service.New` constructs and owns a runtime as a
`service.Component`. Callers explicitly choose required or best-effort
initialization and retain control of `Config.RegisterGlobal`, exporters,
sampling, and propagation. The adapter exposes the concrete runtime, performs
no retries, and delegates deadline-propagated flush, shutdown, and conditional
global restoration to `Runtime.Shutdown`.

Required initialization failures stop service startup. Best-effort failures
permit startup and remain available through `InitializationError`; no
readiness check is added because telemetry availability does not determine
whether the service can accept business work.

## Documentation

Start with the [documentation index](docs/README.md) for the public contract,
architecture, compatibility, upgrade, contribution, and security material.
Observable OTLP, W3C propagation, baggage, and semantic-convention choices are
recorded in the [specification decision register](docs/specification-decisions.md).

Shared construction, ownership, lifecycle, and composition expectations are in
the versioned [Golib ecosystem index](https://github.com/faustbrian/go-library-tools/blob/v1.4.0/docs/ecosystem/README.md)
and its [Observability family](https://github.com/faustbrian/go-library-tools/blob/v1.4.0/docs/ecosystem/design-language.md#package-families-and-selection).

Runnable commands are in [`examples/service`](examples/service) and
[`examples/worker`](examples/worker).

## Development

```sh
make check
make race        # changes affecting concurrent lifecycle behavior
make fuzz        # changes affecting hostile parsing boundaries
make benchmark   # changes making performance or resource claims
```

CI also runs the applicable linting, vulnerability, example, Collector
protocol, race, and supported Go/OpenTelemetry checks. Coverage is evaluated
against changed behavior and material risk, not a universal percentage.

## Stability

Trace and metric APIs use stable OpenTelemetry interfaces. The log signal is
intentionally absent from the stable runtime; see [log stability](docs/logs.md).
Major releases may refine configuration, but changes are documented in
[`CHANGELOG.md`](CHANGELOG.md) and follow semantic versioning.

## License

MIT. See [`LICENSE`](LICENSE).
