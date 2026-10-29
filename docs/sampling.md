# Sampling

[Documentation map](index.md) · [Repository README](../README.md)
[Next: FAQ](faq.md)

## Modes

The `trace` package supports `always_on`, `always_off`, and deterministic
trace-ID ratio sampling. `ParentBased` wraps any root policy.

```go
config.Traces.Sampler.Mode = telemetrytrace.ModeRatio
config.Traces.Sampler.Ratio = 0.10
config.Traces.Sampler.ParentBased = true
```

Ratios range from 0 through 1. The default is parent-based 10% root sampling.

## Cross-service consistency

Keep parent-based sampling enabled for normal distributed traces. A service
must not override a sampled parent's decision based on its local ratio. The
test suite proves sampled and unsampled remote decisions across hops.

## Untrusted callers

`ParentDropOnly` is for services whose callers are not trusted to choose the
sampling decision. An unsampled remote parent still drops the trace, but a
sampled remote parent is sampled again by an independent random draw at the
local ratio. Ratio sampling of the trace ID is not used for that draw, because
the caller chooses the trace ID and could force sampling with a low one. Local
parents keep their decision, so traces stay whole inside the process.

The cost is holes in upstream traces: a caller's sampled trace is not always
continued here. Use it only where the collector-volume cap matters more than
complete cross-service traces. It excludes `ParentBased`, which
`DefaultConfig` enables.

## Choosing a ratio

Estimate spans per request, request rate, retention, and backend cost. Start
conservatively, then increase only with measured Collector and backend headroom.
Always-on is appropriate for local tests and low-volume critical flows, not as
an unreviewed production default.

Head sampling cannot select traces based on their eventual outcome. Put
tail-sampling in the Collector when outcome-aware policies are required. Keep
the application sampler parent-based so downstream services honor the
Collector-compatible trace decision.

## Changes

Sampling changes affect volume and incident visibility immediately. Roll out
through a canary, monitor accepted/dropped spans and cost, and document the
effective policy. Sampling is not a security control: unsampled data must still
be safe to generate.
