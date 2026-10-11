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

## Adding A Module

Follow [repository structure policy](AGENTS.md#repository-structure). New modules
require an explicit purpose, ownership boundary, dependency review, package
catalog entry, full quality gates, documentation, changelog, license, security
policy, compatibility plan, and release dry-run.
