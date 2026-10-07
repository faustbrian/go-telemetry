# Telemetry specification conformance

The root module owns observable profiles for OTLP 1.10.0, W3C Trace Context,
W3C Baggage, and OpenTelemetry Semantic Conventions 1.40.0. The
[decision register](../docs/specification-decisions.md) defines the exact claim
boundaries. [`manifest.tsv`](manifest.tsv) pins normative and maintained-peer
sources; [`monitoring.json`](monitoring.json) monitors later publications.
The OpenTelemetry Go v1.44.0 source pins are historical maintained-peer
comparators, not the current runtime dependency. Current `go.mod` and `go.sum`
pin v1.45.0; the executable provider profiles below run against that selected
dependency.

| Decision | Observable profile | Evidence |
| --- | --- | --- |
| TELEMETRY-DEC-001 | Trace and metric OTLP protobuf over gRPC or HTTP | `TestHTTPCollectorInteroperability`, `TestGRPCCollectorInteroperabilityAndRetry` |
| TELEMETRY-DEC-002 | Bounded retry, timeout, compression, and TLS policy | `TestConfigValidationRejectsInvalidTransportSettings`, `TestHTTPExporterFailureModes` |
| TELEMETRY-DEC-003 | Bounded W3C Trace Context extraction | `TestPolicyIgnoresOversizedTraceContext`, `FuzzPropagationHeaders` |
| TELEMETRY-DEC-004 | Replacement of stale outbound propagation | `TestPolicyReplacesOutboundHeadersAndFiltersBaggage` |
| TELEMETRY-DEC-005 | Trusted allow-listed baggage profile | `TestPolicySeparatesTrustedAndUntrustedInboundBaggage`, `FuzzUntrustedMetadata` |
| TELEMETRY-DEC-006 | Owned resource identity and v1.40.0 schema | `TestBuildResourceOwnsServiceIdentity`, `FuzzResourceAttributes` |
| TELEMETRY-DEC-007 | Privacy-minimized semantic instrumentation | HTTP, PostgreSQL, queue, and cache privacy tests |

The OTLP tests prove provider agreement with the selected OpenTelemetry Go
and pinned protobuf dependencies, not interoperability with every
Collector vendor. The baggage and instrumentation profiles are deliberate
defensive subsets and do not claim full W3C Baggage or complete
semantic-convention emission.

## Publication monitoring review, 2026-10-07

The eight monitored authorities were retrieved for this review. The four
pinned normative source documents and semantic-convention release feed remain
byte-identical. The OTLP release feed and both W3C publication-history pages
have different bytes; their reviewed monitoring digests are updated without
changing normative source pins, decision history, dependencies or runtime.

[OTLP 1.11.0](https://github.com/open-telemetry/opentelemetry-proto/releases/tag/v1.11.0)
adds request and response size guidance, a profiles ProcessContext message,
and an HTTP-date Retry-After clarification.
[OTLP 1.11.1](https://github.com/open-telemetry/opentelemetry-proto/releases/tag/v1.11.1)
clarifies UTF-8 and JSON timestamps, updates attribute examples and OpenAPI
enum generation, and annotates protobuf fields with introduction versions.
The owned profile remains OTLP 1.10.0 trace/metric protobuf, not profiles,
HTTP JSON or generated OpenAPI. Retaining that profile is not a claim of
OTLP 1.11 conformance: adopting the later size, retry and UTF-8 requirements
requires affected exporter behavior and interoperability evidence first.

The reviewed [Trace Context Level 1 history](https://www.w3.org/standards/history/trace-context-1/)
still lists the 2021 Recommendation as its latest publication; the monitored
older history URL redirects to that page. The reviewed
[Baggage history](https://www.w3.org/standards/history/baggage/)
still lists the 2024 Candidate Recommendation Snapshot. Neither history page
establishes a new Recommendation replacing the pinned owned profile. This
monitoring acknowledgement does not certify security qualification or broaden
the existing conformance claims; subsequent byte drift still fails closed.
