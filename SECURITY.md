# Security policy

## Reporting

Do not open public issues for suspected vulnerabilities or telemetry data
leaks. Use GitHub private vulnerability reporting for this repository. Include
the affected version, configuration, reproduction, expected trust boundary,
and whether secrets or untrusted identifiers were exported.

Maintainers follow the immutable ecosystem
[vulnerability management policy](https://github.com/faustbrian/go-library-tools/blob/5ad0adb193a46306b4500b8b59cee9ec110fdee3/docs/ecosystem/security/vulnerability-management.md)
for severity, acknowledgement and remediation targets, private evidence,
embargo, advisories and coordinated affected-module releases. Targets begin
when sufficient private evidence exists to reproduce or confidently bound
the report. Maintainers communicate evidence gaps and target changes; private
reporter data and credentials do not enter public artifacts.

## Supported versions

Security fixes target the latest published major. Version 2.0.x is the
supported line following publication of v2.0.0; v1 receives no further
security fixes. Consumers that
cannot migrate must assess the documented v2 behavior changes and their
exposure on the unsupported v1 line.

## Security model

`telemetry` treats inbound headers, HTTP metadata, SQL, cache keys, queue
messages, error strings, and payloads as untrusted. Default instrumentation
does not export them. Baggage is disabled until a trusted allow-list and bounds
are configured. Export credentials remain caller-owned configuration.

The project assumes no production access and contains no remote management,
dynamic code loading, cgo, `unsafe`, or `go:linkname` paths.
