# Contributing

## Before Editing

1. Read [`AGENTS.md`](AGENTS.md) and the affected package's docs.
2. Run focused standalone Go tests for the affected package.
3. Identify owned dependencies and reverse dependants in `modules.json`.
4. Preserve unrelated work and generated/corpus provenance.

## Changes

Keep commits focused and conventional. Update every affected changelog with
the behavior and migration impact. Public API changes require compatibility
evidence and documentation. Specification behavior requires a decision record,
fixture coverage, and interoperability evidence.

New direct dependencies and dependency updates must follow the
[dependency governance policy](AGENTS.md#dependencies-and-supply-chain). Package-local
update bots are forbidden; the root policy owns every module and action update.

Specification-backed changes must follow the
[specification governance contract](AGENTS.md#design), update
the affected stable decision entries, and complete the Specification Decisions
section of the pull request template. An unresolved interpretation or stale
source pin is release-blocking; peer behavior cannot silently select policy.
The [specification decision register](docs/specification-decisions.md) and
[conformance matrix](specification/README.md) are the review entry points.

When mutation is selected for a named material risk, it must finish with zero
surviving viable mutants.

Do not add package-local workflows, permanent replacements, machine-specific
paths, bypass flags, broad mutation exclusions, or aggregate quality metrics
that hide a failing package.

## Verification

Use Go 1.27.2 for verification; the public module minimum remains Go 1.27.0.
Standalone tests require neither sibling repositories nor building the verifier
release from source:

```bash
GOWORK=off go test ./... -count=1 -timeout=3m
```

`make check` and `make ci` use the checksum-qualified public CLI and bounded
`--local --all` verification: deterministic tests, saved fuzz seeds and the
named local risk check, without active fuzz exploration. Committed-range CI
adds `--base-sha` to select active smoke; the explicit single-target command
below also explores. No goal ledger is a contributor prerequisite.
The manifest declares no services or test tags. OTLP protocol tests use local
HTTP/gRPC servers, not a deployed Collector. PostgreSQL driver integration
tests skip unless `TELEMETRY_TEST_POSTGRES_DSN` points to a disposable fixture;
ordinary tests do not certify that live database boundary.

The selected lifecycle race runs once within two minutes. Concurrent shutdown
callers must share the actual terminal exporter failure instead of replacing
that result with a pre-canceled caller's context error:

```bash
GOWORK=off go test -race . -run '^TestPreCanceledConcurrentShutdownSharesStartedResult$' -count=1 -timeout=2m
```

Normal Go tests execute saved fuzz seeds, not active exploration. For a changed
hostile-input boundary, select one target for two seconds; optional local
mutation stays in one affected package:

```bash
GOWORK=off go test ./propagation -run '^$' -fuzz '^FuzzPropagationHeaders$' -fuzztime=2s -timeout=2m
golib mutation --module . --package ./propagation
```

Agent-run Go commands must use and remove a task-owned disposable `GOCACHE`.
Shared PR, main and scheduled CI are bounded. Explicit `make check-full` and
the `release_dry_run` input retain full qualification, including the configured
example build, OTLP interoperability/failure suite and Go examples (each with
20-minute limits), full race/API/security checks, and all four native fuzz
targets at 10,000 iterations with 20-minute timeouts. Existing `make race`,
`make fuzz` and `make benchmark` remain explicit full or named-risk commands;
benchmark measurement retains its 100ms budget. Use
`make -f verification/package.mk compatibility` for the explicit minimum
OpenTelemetry profile check, and actual deployment/fixture tests when changing
external behavior.

`make specification-check` is offline; `make specification-check-online` and
full qualification retain online authority. See [docs/testing.md](docs/testing.md)
and [docs/release.md](docs/release.md) for focused lifecycle checks and deliberate
release qualification. Report unavailable or failed selected commands without
claiming the bounded route establishes release readiness or deployed behavior.

## Selected Weekly Assurance

The owner-level [selected-assurance caller](.github/workflows/assurance.yml) is
configured for main only, on Thursdays at 04:19 UTC. It selects one of the
4 current declared root-module fuzz targets using the shared UTC-week
rotation, with 30 seconds of active exploration.
The selected risks are resource construction after validation, configuration
validation, bounded trace-context propagation and bounded untrusted baggage.
Targets run in `.` and `./propagation`; they do not certify exporter shutdown,
live Collector/PostgreSQL interoperability or deployed resource behavior.
Keep the lifecycle race, OTLP suites and applicable disposable live fixtures.

Base-free ordinary checks execute deterministic seeds, not active exploration;
committed-range CI selects affected active smoke. Manual assurance requires an
exact anchored selector from the root fuzz operation in `.golib.yaml`, for
example `^FuzzPropagationHeaders$`. Empty, unknown and malformed selections refuse.
The shared job bounds setup and execution to ten minutes, its native session
to five minutes, active workers and CPU to two, owned memory to 2 GiB, native
output to 1 MiB and private-cache lifetime. Failure, cancellation, exhausted
budgets and cleanup refusal remain attributable failures. This lane does not
run mutation or replace Required, original full fuzz budgets, explicit
`make check-full`, nested/service contracts or relevant release qualification.

## Adding A Module

Follow [repository structure policy](AGENTS.md#repository-structure). New modules
require an explicit purpose, ownership boundary, dependency review, package
catalog entry, full quality gates, documentation, changelog, license, security
policy, compatibility plan, and release dry-run.

## Source Documentation

Install the checksum-pinned public verifier declared in `.golib.yaml`.
`make source-docs-check` validates objective Go comment, marker, link and
generated-producer structure; `make check`, `make check-full` and `make ci`
require it before their existing checks. Shared CI runs the same strict
contract and includes its result in Required. This is structural enforcement,
not a substitute for source-truth review or behavioral verification.
