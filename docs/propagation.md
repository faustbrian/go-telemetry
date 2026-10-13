# Propagation

[Documentation map](index.md) · [Repository README](../README.md)
[Next: FAQ](faq.md)

## Default policy

W3C `traceparent` and `tracestate` are accepted within an 8 KiB combined
bound. Baggage is disabled. Outbound injection replaces stale propagation
headers rather than appending to them.

```go
config.Propagation = propagation.DefaultConfig()
```

Malformed or oversized trace input leaves existing span context unchanged;
it does not necessarily clear an existing parent or fail the request. Carrier
Get/Set and their allocations/blocking behavior remain caller-owned.

## Trusted baggage

Enable baggage only for authenticated, controlled peers and define a finite
allow-list:

```go
config.Propagation.BaggageEnabled = true
config.Propagation.TrustedBaggageKeys = []string{"tenant.tier"}
config.Propagation.MaxBaggageItems = 4
config.Propagation.MaxHeaderBytes = 1024
```

Public HTTP handlers remain untrusted. Mark a fixed internal handler only when
the network/authentication boundary proves the peer is trusted:

```go
handler, err := nethttp.NewHandler(next, nethttp.ServerConfig{
    Operation: "internal.orders.list",
    Route: "/internal/orders",
    TrustedInbound: true,
    TrustInbound: func(request *http.Request) bool {
        return authenticatedWorkload(request.Context())
    },
    Propagator: runtime.Propagator(),
})
```

`TrustedInbound` requires `TrustInbound` to prove authentication for each
request. A false proof uses untrusted extraction and clears baggage.
If the proof callback panics, the request ends with a categorical HTTP 500
without invoking the business handler or returning the panic value.
`TrustedInbound` has no effect on propagators that do not implement trusted
extraction; they use the standard interface. Unknown baggage keys are always
dropped by the runtime Policy, not by arbitrary custom propagators. Never allow
user IDs, request IDs, emails, tokens, or arbitrary tenant input.

## Manual extraction

`Policy.Extract` is untrusted and clears baggage. `Policy.ExtractTrusted`
filters trusted baggage. Use the explicit method at messaging or RPC boundaries
that are not covered by the HTTP adapter.

Outbound baggage uses the same allow-list and bounds. If the serialized value
exceeds the byte budget, the entire baggage value is cleared with Set(field, "").
Whether an empty field is deleted from the wire depends on the carrier. Item
bounds apply after in-budget baggage parsing, not to the number of parsed
members. Allowed values/properties are not redacted, and trusted extraction
performs no authentication of its own.
