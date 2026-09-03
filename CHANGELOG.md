# Changelog

All user-visible changes to this project are documented here. The format is
based on Keep a Changelog, and releases follow Semantic Versioning.

## [Unreleased]

## [1.1.1] - 2026-09-03

### Changed

- Require gRPC v1.83.1 or later so fragmented HTTP/2 DATA frames cannot
  exhaust server heap through unbounded receive-buffer bookkeeping.
- Normalize the missing Go runtime meter-provider validation error to begin
  with lowercase text.

- Publish complete schema-v2 cohesion metadata and versioned Golib ecosystem
  navigation for the telemetry runtime, instrumentation adapters, lifecycle
  adapter, and supporting packages.
- Adopt the checksum-verified `go-library-tools` v1.3.0 CLI, add the local
  `make cohesion` gate to the complete repository contract, and pin reusable
  CI cohesion enforcement to its immutable revision.

- Establish an auditable [specification decision
  register](docs/specification-decisions.md) for OTLP, W3C Trace Context,
  defensive baggage, and OpenTelemetry semantic-convention profiles with
  pinned sources, maintained peer evidence, change monitoring, and CI
  enforcement:
  `TELEMETRY-DEC-001 sha256:527d76eaf1a10a709ea47f001363c1694340bf45396cc16985b2e7b52dd8a587`,
  `TELEMETRY-DEC-002 sha256:10bf687e9f5306fd7f76ad20dc2f81171a8ebaf486b43e52bdc628d1e72b8bed`,
  `TELEMETRY-DEC-003 sha256:cb639d30e8f960f72d78bd8690430d7325e33c706197cbf02a3992b109e72e29`,
  `TELEMETRY-DEC-004 sha256:f8f94fae9bf0f73cffa4e6852727b245262206c298b8b031aa0cedf2482c12b1`,
  `TELEMETRY-DEC-005 sha256:689aa1b13177ce2d06a7aa2b7279495bd1141337f8b224dc464fb09b59c2ddf9`,
  `TELEMETRY-DEC-006 sha256:a0c2ea2a37a914ba1d6ec9e6c39976f59965bbc012c2c9d3cdfb554ac919c0c0`,
  and
  `TELEMETRY-DEC-007 sha256:e46f296581c864efa8dffa9c07d16716d117a7b97d15d720da9330b3621098f1`.

- Replace the copied repository verification implementation with the
  checksum-verified `go-library-tools` v1.2.0 specification-governance
  contract.

## [1.1.0] - 2026-08-28

### Added

- Export Go heap usage, cumulative allocation volume, goroutine count, GC
  cycles, and cumulative GC pause time through an explicitly owned runtime
  registration.

- Record PostgreSQL pool acquisition duration, waiting acquisitions, outcomes,
  and acquired/idle/total/max connection snapshots alongside query metrics.

- Record active HTTP server requests and request and response body sizes so
  services can observe saturation and payload pressure without retaining
  payload content.

### Changed

- Align isolated dependency checks with standalone package module paths.

### Documentation

- Remove completed implementation plans and the archived monorepo portal.

## [1.0.0] - 2026-08-25

### Fixed

- Bind the reviewed zero-mutant Go HTTP client instrumentation facade to its
  exact standalone source identity.

### Changed

- Exclude intentional nested modules from root local-proxy archives so local,
  bootstrap, CI, and public module checksums describe the same source
  boundary.

- Track the pinned documentation-tool lockfile so clean CI checkouts install
  the exact validated cspell dependency.

- Reconcile standalone dependency checksums against deterministic current
  module archives so CI, local verification, and release consumers resolve
  identical content.

- Harden standalone documentation validation with deterministic spelling and
  link checks, package-specific documentation gates, and repository-local
  contributor guidance.

### Documentation

- Replace obsolete repository links and workflow claims with standalone
  package targets and current release guidance.

- Add package discovery documentation.

### Changed

- Publish the module from its standalone `github.com/faustbrian/go-telemetry` identity while preserving its documented API and behavior.
- Replace obsolete owned-module pseudo-version pins with the monorepo's local
  `v0.0.0` source-proxy coordinates; release tooling continues to emit exact
  `v1.0.0` dependency versions.
- Remove unused CLI-related indirect dependencies from canonical module
  metadata.
- Pin owned sibling modules to exact resolvable main pseudo-versions so
  standalone and clean external consumers use immutable dependency content.

### Added

- A `telemetryservice` lifecycle adapter for explicit required or best-effort
  runtime initialization, caller-selected global provider registration, and
  bounded single flush and shutdown.
- Explicit trace and metric runtime lifecycle with standard OpenTelemetry APIs.
- OTLP/gRPC and OTLP/HTTP exporters with endpoints, TLS, mTLS, headers, gzip,
  retry, timeout, batching, and bounded queues.
- Owned service resources, parent-based sampling, metric views, histogram
  boundaries, hard cardinality limits, and trusted propagation policies.
- Idempotent bounded shutdown with global restoration and cross-SDK exporter
  failure aggregation.
- Private-by-default adapters for `net/http`, `http-client`, pgx/
  `postgres`, `cache`, and `queue`.
- Explicit per-handler trusted inbound baggage extraction with safe fallback to
  the standard propagator contract.
- Deterministic in-memory test providers.
- In-process OTLP HTTP and gRPC Collector interoperability and failure tests.
- Race matrices, fuzz targets, allocation benchmarks, exact coverage
  enforcement, vulnerability scanning, compatibility matrices, and release CI.
- Runnable service and worker examples plus complete adoption and operations
  documentation.

### Fixed

- Normalize standard OTLP endpoint URLs in the runnable service and worker
  examples so explicit exporter configuration does not emit misleading SDK
  parse errors or mis-handle transport security and HTTP path prefixes.
- Resolve the unreleased service platform and its sibling modules from their
  main-branch pseudo-versions so clean consumers can install telemetry.
- Select each independently versioned OpenTelemetry module explicitly in the
  compatibility matrix instead of resolving nested modules as root packages.
- Run compatibility dependency updates against writable isolated module files
  without leaking module-only flags into child OpenTelemetry tooling.
- Use deterministic execution counts for default fuzz smoke campaigns to avoid
  treating the Go harness deadline as an application failure.
- Contradictory plaintext and TLS settings now fail closed instead of silently
  ignoring Collector identity or client credentials.
- Failed initialization cleanup now uses a fresh bounded deadline and never
  holds global runtime ownership while an exporter shuts down.
- Non-finite sampling ratios and malformed or oversized service identity now
  fail validation before provider construction.
- PostgreSQL hooks end only their owned query span, and panicking HTTP handlers
  retain accurate duration metrics.

### Security

- Upgrade gRPC to 1.82.1 to remove the reachable `GO-2026-6061` xDS RBAC and
  HTTP/2 transport vulnerability from OTLP/gRPC consumers.
- Untrusted baggage is rejected by default and trusted baggage is allow-listed,
  item-bounded, and byte-bounded.
- Default instrumentation excludes payloads, secrets, raw identifiers, SQL,
  cache keys, queue messages, and error text.
- Standalone module resolution now selects patched `x/net` and `x/text`
  releases instead of relying on higher versions supplied by the repository
  workspace.

[Unreleased]: https://github.com/faustbrian/go-telemetry/compare/v1.1.1...HEAD
[1.1.1]: https://github.com/faustbrian/go-telemetry/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/faustbrian/go-telemetry/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/faustbrian/go-telemetry/releases/tag/v1.0.0
