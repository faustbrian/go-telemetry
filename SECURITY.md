# Security policy

## Reporting

Do not open public issues for suspected vulnerabilities or telemetry data
leaks. Use GitHub private vulnerability reporting for this repository. Include
the affected version, configuration, reproduction, expected trust boundary,
and whether secrets or untrusted identifiers were exported.

## Supported versions

Security fixes target the latest published major. Version 1.2.x remains the
supported line until v2.0.0 is published. After publication, v2.0.x is the
supported line and v1 receives no further security fixes. Consumers that
cannot migrate must assess the documented v2 behavior changes and their
exposure on the unsupported v1 line.

## Security model

`telemetry` treats inbound headers, HTTP metadata, SQL, cache keys, queue
messages, error strings, and payloads as untrusted. Default instrumentation
does not export them. Baggage is disabled until a trusted allow-list and bounds
are configured. Export credentials remain caller-owned configuration.

The project assumes no production access and contains no remote management,
dynamic code loading, cgo, `unsafe`, or `go:linkname` paths.
