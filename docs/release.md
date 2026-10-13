# Release process

[Documentation map](index.md) · [Repository README](../README.md)
[Next: FAQ](faq.md)

## Preconditions

- `CHANGELOG.md` contains every user-visible change.
- Compatibility, upgrade, security, and operations docs match behavior.
- `make check-full`, lint, `govulncheck`, and every race, fuzz, or benchmark gate
  selected for a material release risk pass.
- The Go/OpenTelemetry matrix and all GitHub Actions gates are green.
- No cardinality, privacy, race, leak, deadlock, timeout, or unbounded-resource
  blocker remains.

Ordinary PR, main and scheduled shared CI use bounded local verification, not
full qualification. Use the explicit `release_dry_run` workflow input and
module-selected `golib release check --module .` / `golib release dry-run --module .`
for release validation. These commands do not publish; retain actual
OpenTelemetry compatibility and selected Collector/database fixture evidence
where the release claim depends on them.

## Versioning

Use semantic versioning. Before v1, call out configuration/API changes clearly.
After v1, changing exported types, defaults, metric names/units/attributes,
resource identity, propagation policy, or error behavior is a compatibility
change.

## Tagging

Create a signed `v*` tag only from verified main after the hosted release
rehearsal passes. Publish GitHub release notes from that exact tag. The CI
workflow is not a tag-triggered release publisher; never use a tag to bypass
a failing branch or rehearsal check.

## Post-release

Build the service and worker examples against the tag, confirm module proxy
availability, inspect release notes and documentation links, and monitor early
adopters for exporter failures, memory, cardinality, and compatibility issues.

Move released entries from `Unreleased` to a dated version and create a new
empty `Unreleased` section in the same release change.
