# Testing and failure injection

## Fast deterministic tests

`testtelemetry.New` provides sequential IDs, an in-memory span recorder, and a
manual metric reader. Use it for exact span names, parents, attributes, status,
metric values, and privacy assertions.

## Local gates

```sh
make check       # ordinary format, vet, test, coverage, safety, and examples
make race        # changes affecting concurrent lifecycle behavior
make fuzz        # changes affecting hostile parsing boundaries
make benchmark   # changes making performance or resource claims
```

`make check` is the default local gate. Select race, fuzz, and benchmark
campaigns only for the material risk exercised by the change.

## OTLP failures

The OTLP integration suite runs HTTP and gRPC servers in-process. It injects
temporary unavailability, HTTP rate limiting, slow endpoints, malformed
responses, custom paths, compression, and authentication metadata. It validates
protobuf requests for both traces and metrics.

## Lifecycle failures

Internal constructor seams inject resource, propagator, sampler, and metric-view
failures. Tests prove partial exporter cleanup, duplicate global registration,
external global replacement, concurrent initialization, repeated shutdown, and
cross-SDK error aggregation.

## Privacy and cardinality

Adapter tests place the word `secret` in URLs, SQL, arguments, errors, cache
keys, messages, bodies, and panic values, then inspect recorded telemetry.
Metric tests exceed configured cardinality and assert both point count and
attribute allow-lists.

Mocks alone are insufficient for protocol or concurrency claims. Add an
in-process protocol server, real SDK reader/provider, or supported integration
environment for every new external behavior.
