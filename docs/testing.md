# Testing and failure injection

## Fast deterministic tests

`testtelemetry.New` provides sequential IDs, an in-memory span recorder, and a
manual metric reader. Use it for exact span names, parents, attributes, status,
metric values, and privacy assertions.

## Local gates

```sh
GOWORK=off go test ./... -count=1 -timeout=3m
make check       # bounded public-verifier local route
make check-full  # explicit original full qualification
```

Ordinary PR, main and scheduled shared CI use the bounded route. Normal Go
tests execute saved fuzz seeds; active exploration requires one selected target,
for example:

```sh
GOWORK=off go test ./propagation -run '^$' -fuzz '^FuzzPropagationHeaders$' -fuzztime=2s -timeout=2m
```

Keep `make race`, `make fuzz` and `make benchmark` for a named material risk or
explicit full/release boundary. Full fuzz preserves four 10,000-iteration
targets; `make check-full` retains their 20-minute per-target limits and all
configured examples and interoperability operations. The ordinary manifest
declares no service or test-tag requirement. Live PostgreSQL integration still
needs `TELEMETRY_TEST_POSTGRES_DSN`; a skipped test is not database proof.

## OTLP failures

The OTLP integration suite runs HTTP and gRPC servers in-process. It injects
temporary unavailability, HTTP rate limiting, slow endpoints, malformed
responses, custom paths, compression, and authentication metadata. It validates
protobuf requests for both traces and metrics.

Run the focused owned suite with
`make -f verification/package.mk integration`. It does not start or certify a
production Collector. Use a deployment-owned fixture for Collector-facing
rollout claims.

## Lifecycle failures

Internal constructor seams inject resource, propagator, sampler, and metric-view
failures. Tests prove partial exporter cleanup, duplicate global registration,
external global replacement, concurrent initialization, repeated shutdown, and
cross-SDK error aggregation.

The bounded ordinary lifecycle selection is:

```sh
GOWORK=off go test -race . -run '^TestPreCanceledConcurrentShutdownSharesStartedResult$' -count=1 -timeout=2m
```

It asserts that concurrent shutdown callers receive the same actual terminal
exporter error, not a pre-canceled caller's context error.

## Privacy and cardinality

Adapter tests place the word `secret` in URLs, SQL, arguments, errors, cache
keys, messages, bodies, and panic values, then inspect recorded telemetry.
Metric tests exceed configured cardinality and assert both point count and
attribute allow-lists.

Mocks alone are insufficient for protocol or concurrency claims. Add an
in-process protocol server, real SDK reader/provider, or supported integration
environment for every new external behavior.
