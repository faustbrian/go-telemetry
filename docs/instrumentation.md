# Instrumentation

All adapters accept standard providers and borrow their lifecycle. Nil providers
on the HTTP/cache/queue/pgx constructors choose no-ops, not process globals. Pass
runtime providers explicitly even when globals are registered. The runtime
callback adapter requires a non-nil provider and must be closed before its owner
shuts down that provider.

## Go runtime

```go
runtimeMetrics, err := goruntime.New(runtime.MeterProvider())
defer runtimeMetrics.Close()
```

The runtime adapter registers no global provider and starts no background
goroutine. It exports heap usage, cumulative allocation volume, goroutine
count, completed GC cycles, and cumulative GC pause time. Derive allocation
rate, GC frequency, and average GC pause from the cumulative counters in the
telemetry backend. CPU, RSS, file descriptors, and container restarts belong
to the process and container platform rather than the Go runtime and must be
collected by the deployment platform.

## net/http server

```go
handler, err := nethttp.NewHandler(next, nethttp.ServerConfig{
    Operation: "orders.show",
    Route: "/orders/{order}",
    TracerProvider: runtime.TracerProvider(),
    MeterProvider: runtime.MeterProvider(),
    Propagator: runtime.Propagator(),
})
```

Operation and route are fixed at construction. Raw request target, host,
headers, body, and client address are never attributes. Trusted baggage is an
explicit per-handler option described in [propagation](propagation.md).

The server adapter records request count and duration, active requests, and
request and response body sizes. Payload sizes are numeric measurements only;
payload content is never inspected or retained. Requests without a known
`Content-Length` omit the request-size measurement rather than buffering the
body.

## net/http and http-client clients

```go
transport, err := nethttp.NewTransport(http.DefaultTransport, nethttp.ClientConfig{
    Operation: "payments.request",
    TracerProvider: runtime.TracerProvider(),
    MeterProvider: runtime.MeterProvider(),
    Propagator: runtime.Propagator(),
})
client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
```

`instrumentation/httpclient.NewTransport` exposes the same contract at the
RoundTripper seam used by `http-client`. With the runtime propagation policy,
outbound injection replaces stale fields on cloned request headers. A nil
propagator uses SDK TraceContext, not the bounded baggage policy. Request
bodies remain shared, the returned response body remains caller-owned, and
spans/duration end when RoundTrip returns rather than after the body is
consumed. The wrapper adds no request timeout or CloseIdleConnections
forwarding; callers own the base transport.

## postgres

```go
tracer, err := gopostgres.New(gopostgres.Config{
    TracerProvider: runtime.TracerProvider(),
    MeterProvider: runtime.MeterProvider(),
    Operations: []string{"users.by_id", "users.create"},
})
pgxConfig.Tracer = tracer

ctx = gopostgres.ContextWithOperation(ctx, "users.by_id")
```

Unknown operation names collapse to `postgresql.query`. SQL and arguments are
ignored. Wrapper telemetry records fixed outcomes and five-byte PgError.Code
values, not raw errors; the code length is checked, not SQLSTATE grammar.
Driver errors remain application-visible. Query/acquire end callbacks require
the matching start context exactly once; cancellation is not automatic
observation cleanup.

The same tracer implements `pgxpool.AcquireTracer`. When it is assigned to
`poolConfig.ConnConfig.Tracer`, pgxpool records acquisition duration and
outcome, current waiting acquisitions, and acquired/idle/total/max pool
snapshots. This keeps pool contention distinct from PostgreSQL query execution
without recording connection identity or raw acquisition errors.

## cache

```go
ctx, end := cacheTelemetry.Begin(ctx, gocache.OperationGet)
result, err := cache.Get(ctx, key)
end(gocache.OutcomeHit, err)
```

Map the target cache's result state to a finite `Outcome`. The adapter API has
no key or value parameter. Always call the one-shot end function on completion
and panic paths; context cancellation does not end the observation itself.

## queue

```go
instrumented := goqueue.WrapHandler(queueTelemetry, handler)
queue.WithFn(instrumented)
```

The generic wrapper preserves the target message type but never inspects it.
Backend is a finite construction-time enum; raw errors and panics are preserved
for application behavior but replaced with a fixed telemetry status.
