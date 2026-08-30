# Telemetry specification conformance

The root module owns observable profiles for OTLP 1.10.0, W3C Trace Context,
W3C Baggage, and OpenTelemetry Semantic Conventions 1.40.0. The
[decision register](../docs/specification-decisions.md) defines the exact claim
boundaries. [`manifest.tsv`](manifest.tsv) pins normative and maintained-peer
sources; [`monitoring.json`](monitoring.json) monitors later publications.

| Decision | Observable profile | Evidence |
| --- | --- | --- |
| TELEMETRY-DEC-001 | Trace and metric OTLP protobuf over gRPC or HTTP | `TestHTTPCollectorInteroperability`, `TestGRPCCollectorInteroperabilityAndRetry` |
| TELEMETRY-DEC-002 | Bounded retry, timeout, compression, and TLS policy | `TestConfigValidationRejectsInvalidTransportSettings`, `TestHTTPExporterFailureModes` |
| TELEMETRY-DEC-003 | Bounded W3C Trace Context extraction | `TestPolicyIgnoresOversizedTraceContext`, `FuzzPropagationHeaders` |
| TELEMETRY-DEC-004 | Replacement of stale outbound propagation | `TestPolicyReplacesOutboundHeadersAndFiltersBaggage` |
| TELEMETRY-DEC-005 | Trusted allow-listed baggage profile | `TestPolicySeparatesTrustedAndUntrustedInboundBaggage`, `FuzzUntrustedMetadata` |
| TELEMETRY-DEC-006 | Owned resource identity and v1.40.0 schema | `TestBuildResourceOwnsServiceIdentity`, `FuzzResourceAttributes` |
| TELEMETRY-DEC-007 | Privacy-minimized semantic instrumentation | HTTP, PostgreSQL, queue, and cache privacy tests |

The OTLP tests prove provider agreement with the pinned OpenTelemetry Go and
protobuf dependencies, not interoperability with every Collector vendor. The
baggage and instrumentation profiles are deliberate defensive subsets and do
not claim full W3C Baggage or complete semantic-convention emission.
