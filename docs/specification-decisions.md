# Telemetry specification decisions

This register records every material standards choice owned by the telemetry
runtime. Source bytes and change authorities are pinned in the
[manifest](../specification/manifest.tsv) and
[monitoring policy](../specification/monitoring.json). Statuses are `resolved`,
`unresolved`, or `superseded`; observable changes require compatibility,
changelog, executable evidence, conformance, and decision-history review.

## TELEMETRY-DEC-001: OTLP signal and transport profile

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** optional behavior
- **Decision scope:** transport-specific
- **Specification:** OpenTelemetry Protocol 1.10.0
- **Version:** OTLP 1.10.0
- **Source authority:** otlp-source
- **Section:** Protocol Details, OTLP/gRPC, and OTLP/HTTP
- **Requirement strength:** OPTIONAL
- **Issue:** OTLP defines multiple signals, encodings, and transports while this runtime must expose a stable application profile.
- **Credible interpretation:** Expose every OTLP signal and encoding
- **Credible interpretation:** Support trace and metric protobuf export over gRPC and HTTP
- **Credible interpretation:** Delegate all transport selection to environment variables
- **Peer behavior:** OpenTelemetry Go v1.44.0 provides separate trace and metric exporters for OTLP/gRPC and OTLP/HTTP protobuf.
- **Selected behavior:** The stable runtime exports traces and metrics using explicitly selected OTLP/gRPC or OTLP/HTTP protobuf; logs, profiles, and OTLP/HTTP JSON are outside the profile.
- **Rationale:** The selected stable OpenTelemetry Go exporters provide one explicit vendor-neutral profile without promising unstable signals.
- **Security consequences:** Explicit endpoints and headers avoid implicit vendor configuration.
- **Resource consequences:** Exporter queues, batches, timeouts, and metric cardinality remain separately bounded.
- **Compatibility consequences:** Adding another signal or encoding requires compatibility review.
- **Wire consequences:** Trace and metric requests use the pinned OTLP protobuf service messages over the selected transport.
- **Executable evidence:** TestHTTPCollectorInteroperability
- **Executable evidence:** TestGRPCCollectorInteroperabilityAndRetry
- **Executable evidence:** TestExporterConstructionSupportsEverySignalAndProtocol
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** Protocol
- **Public API:** ExporterConfig
- **Public API:** otlp.Config
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/collector.md
- **Documentation:** docs/logs.md
- **Upstream status:** OTLP 1.10.0 is pinned and later protocol releases are monitored without automatic behavior changes.
- **Reconsider when:** A supported OpenTelemetry Go line changes its stable OTLP signal or transport contract.
- **Authoritative URL:** https://raw.githubusercontent.com/open-telemetry/opentelemetry-proto/v1.10.0/docs/specification.md

## TELEMETRY-DEC-002: Bounded OTLP delivery and transport security

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** omission
- **Decision scope:** defensive
- **Specification:** OpenTelemetry Protocol 1.10.0
- **Version:** OTLP 1.10.0
- **Source authority:** otlp-source
- **Section:** Failures, Throttling, and Implementation Recommendations
- **Requirement strength:** SHOULD
- **Issue:** OTLP classifies delivery failures but does not choose application timeout, retry horizon, TLS identity, or local plaintext policy.
- **Credible interpretation:** Use exporter defaults without validation
- **Credible interpretation:** Require TLS for every endpoint
- **Credible interpretation:** Require explicit bounded timeouts and retries while allowing deliberate local plaintext
- **Peer behavior:** OpenTelemetry Go exporters implement OTLP retry classification, gzip, TLS, and per-attempt timeout options but leave policy values to callers.
- **Selected behavior:** Exporter configuration requires a positive timeout, finite retry intervals when enabled, coherent TLS material, and rejects combining plaintext with TLS settings; plaintext remains an explicit local or same-cluster option.
- **Rationale:** Fail-closed validation prevents contradictory security identity and unbounded retry policy while retaining practical Collector deployment.
- **Security consequences:** TLS identity and client credentials cannot be silently ignored by plaintext mode.
- **Resource consequences:** Retries and attempts have finite configured horizons and compression is limited to none or gzip.
- **Compatibility consequences:** Previously tolerated contradictory or unbounded configuration is rejected.
- **Wire consequences:** Retryable OTLP failures may be retried within the configured horizon; non-retryable and timed-out attempts surface exporter errors.
- **Executable evidence:** TestConfigValidationRejectsInvalidTransportSettings
- **Executable evidence:** TestHTTPExporterFailureModes
- **Executable evidence:** TestSecureExporterConstruction
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** ExporterConfig.Validate
- **Public API:** otlp.Config.Validate
- **Public API:** TLSConfig
- **Public API:** RetryConfig
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/collector.md
- **Documentation:** docs/security.md
- **Upstream status:** OTLP retry and throttling rules are monitored separately from package deployment policy.
- **Reconsider when:** OTLP or OpenTelemetry Go defines incompatible mandatory client delivery or security behavior.
- **Authoritative URL:** https://raw.githubusercontent.com/open-telemetry/opentelemetry-proto/v1.10.0/docs/specification.md

## TELEMETRY-DEC-003: Bounded W3C Trace Context extraction

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** omission
- **Decision scope:** defensive
- **Specification:** W3C Trace Context Level 1 Recommendation 2021-11-23
- **Version:** W3C Recommendation 2021-11-23
- **Source authority:** trace-context-source
- **Section:** 3 traceparent Header Field and 3.2 tracestate Header Field
- **Requirement strength:** not specified
- **Issue:** Trace Context defines parsing and invalid-field behavior but does not set one combined application allocation budget for traceparent and tracestate.
- **Credible interpretation:** Parse headers without a package bound
- **Credible interpretation:** Reject the containing request when propagation is malformed
- **Credible interpretation:** Ignore propagation when the combined configured byte bound is exceeded
- **Peer behavior:** OpenTelemetry Go v1.44.0 TraceContext parses the W3C fields and ignores invalid context without returning an application error.
- **Selected behavior:** Extraction delegates W3C syntax and version handling to OpenTelemetry Go only when traceparent plus tracestate fit the configured positive byte bound; malformed or oversized context is ignored without failing the carrier operation.
- **Rationale:** Distributed context remains interoperable while hostile metadata cannot force unbounded parsing or application failure.
- **Security consequences:** Invalid or oversized context grants no remote parent authority.
- **Resource consequences:** Combined trace-context header work is bounded before peer parsing.
- **Compatibility consequences:** Peers sending context beyond the configured budget lose trace linkage.
- **Wire consequences:** Valid in-budget traceparent and tracestate retain OpenTelemetry Go W3C semantics.
- **Executable evidence:** TestPolicySeparatesTrustedAndUntrustedInboundBaggage
- **Executable evidence:** TestPolicyIgnoresOversizedTraceContext
- **Executable evidence:** TestWithinLimitCountsAllFields
- **Fuzz evidence:** FuzzPropagationHeaders
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** propagation.Config
- **Public API:** propagation.Policy.Extract
- **Public API:** propagation.Policy.ExtractTrusted
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/propagation.md
- **Upstream status:** The 2021 Recommendation and W3C publication history are monitored.
- **Reconsider when:** A superseding Trace Context Recommendation changes field processing or mandatory limits.
- **Authoritative URL:** https://www.w3.org/TR/2021/REC-trace-context-1-20211123/

## TELEMETRY-DEC-004: Outbound W3C propagation replaces stale fields

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** interoperability policy
- **Decision scope:** defensive
- **Specification:** W3C Trace Context Level 1 Recommendation 2021-11-23
- **Version:** W3C Recommendation 2021-11-23
- **Source authority:** trace-context-source
- **Section:** 3.2.5 Mutating the tracestate Field
- **Requirement strength:** MAY
- **Issue:** Carrier APIs may already contain traceparent, tracestate, or baggage values and append-versus-replace behavior can create ambiguous downstream context.
- **Credible interpretation:** Append to existing fields
- **Credible interpretation:** Leave stale fields when the current context has no value
- **Credible interpretation:** Clear owned fields before injecting one current context
- **Peer behavior:** OpenTelemetry Go v1.44.0 serializes one current TraceContext value through TextMapCarrier.Set.
- **Selected behavior:** Injection clears traceparent, tracestate, and baggage before writing the filtered current context, so stale carrier values never survive or concatenate.
- **Rationale:** One outbound request must carry at most one unambiguous package-owned propagation profile.
- **Security consequences:** Attacker-supplied stale context cannot remain alongside current authority.
- **Resource consequences:** Replacement keeps carrier growth constant across retries and clones.
- **Compatibility consequences:** Callers relying on prepopulated propagation values must provide them through the current context.
- **Wire consequences:** Outbound propagation contains only the current serialized W3C fields and filtered baggage.
- **Executable evidence:** TestPolicyReplacesOutboundHeadersAndFiltersBaggage
- **Executable evidence:** TestDisabledPolicyClearsExistingBaggage
- **Executable evidence:** TestTransportInjectsContextWithoutRecordingTargetData
- **Fuzz evidence:** FuzzPropagationHeaders
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** propagation.Policy.Inject
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/propagation.md
- **Upstream status:** No W3C erratum selecting append semantics is known; publication history is monitored.
- **Reconsider when:** TextMapCarrier or Trace Context defines conflicting multi-value injection semantics.
- **Authoritative URL:** https://www.w3.org/TR/2021/REC-trace-context-1-20211123/

## TELEMETRY-DEC-005: Trusted allow-listed baggage profile

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** interoperability policy
- **Decision scope:** defensive
- **Specification:** W3C Baggage Candidate Recommendation 2024-05-30
- **Version:** W3C Candidate Recommendation 2024-05-30
- **Source authority:** baggage-source
- **Section:** 3.3.2 Limits, 3.5 Mutating Baggage, and 4 Security Considerations
- **Requirement strength:** SHOULD
- **Issue:** The Candidate Recommendation encourages propagation and defines minimum compliant capacity, while arbitrary baggage can cross trust boundaries, leak data, and create cardinality or allocation pressure.
- **Credible interpretation:** Propagate every valid member up to W3C limits
- **Credible interpretation:** Disable all baggage permanently
- **Credible interpretation:** Disable baggage by default and require a trusted boundary, allow-list, item limit, and byte limit
- **Peer behavior:** OpenTelemetry Go v1.44.0 implements W3C baggage grammar; it does not choose this package's trust boundary or application allow-list.
- **Selected behavior:** Untrusted extraction always clears baggage. Trusted extraction and injection are opt-in, retain only allow-listed keys, enforce configured positive item and byte bounds, and drop oversized output rather than emitting partial members.
- **Rationale:** The profile deliberately prioritizes privacy and bounded authority over transparent propagation and does not claim full Candidate Recommendation propagation compliance.
- **Security consequences:** Unknown keys, user identifiers, secrets, and untrusted baggage do not cross the package boundary by default.
- **Resource consequences:** Parsed and emitted baggage is bounded by configured bytes and item count.
- **Compatibility consequences:** General W3C baggage peers can observe dropped members; trusted applications must declare their stable key contract.
- **Wire consequences:** The baggage header is absent unless enabled and contains only complete allow-listed members within bounds.
- **Executable evidence:** TestPolicySeparatesTrustedAndUntrustedInboundBaggage
- **Executable evidence:** TestTrustedPolicyBoundsItemCountAndOutboundSize
- **Executable evidence:** TestPolicyAcceptsExactOutboundHeaderLimit
- **Fuzz evidence:** FuzzUntrustedMetadata
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** propagation.Config
- **Public API:** propagation.Policy.Extract
- **Public API:** propagation.Policy.ExtractTrusted
- **Public API:** propagation.Policy.Inject
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/propagation.md
- **Documentation:** docs/privacy.md
- **Upstream status:** The 2024 document remains a Candidate Recommendation and its W3C history is monitored.
- **Reconsider when:** Baggage reaches Recommendation with incompatible security, limits, or mutation requirements.
- **Authoritative URL:** https://www.w3.org/TR/2024/CR-baggage-20240530/

## TELEMETRY-DEC-006: Owned resource identity and schema version

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** interoperability policy
- **Decision scope:** application-policy
- **Specification:** OpenTelemetry Semantic Conventions 1.40.0
- **Version:** OpenTelemetry Semantic Conventions 1.40.0
- **Source authority:** semconv-source
- **Section:** Resource service, deployment environment, and telemetry SDK attributes
- **Requirement strength:** not specified
- **Issue:** Semantic conventions define resource keys but do not decide whether generic caller attributes may replace runtime-owned service and SDK identity.
- **Credible interpretation:** Let generic attributes override every key
- **Credible interpretation:** Silently prefer whichever detector runs last
- **Credible interpretation:** Reserve owned identity keys and attach the v1.40.0 schema URL
- **Peer behavior:** OpenTelemetry Go v1.44.0 exposes generated v1.40.0 resource keys and SchemaURL while leaving merge precedence to the application.
- **Selected behavior:** BuildResource owns service, deployment environment, telemetry SDK identity, and the v1.40.0 schema URL; generic Resource entries cannot override reserved keys and other bounded custom attributes remain available.
- **Rationale:** Stable service identity cannot depend on map order or attacker-controlled generic attributes.
- **Security consequences:** Caller-supplied generic attributes cannot impersonate the configured service or SDK.
- **Resource consequences:** Custom attribute count and value sizes remain bounded by configuration validation.
- **Compatibility consequences:** Changing the semantic-convention schema or reserved identity set requires resource compatibility review.
- **Wire consequences:** Exported resources carry the pinned schema URL and deterministic service and SDK attributes.
- **Executable evidence:** TestBuildResourceOwnsServiceIdentity
- **Executable evidence:** TestConfigValidationRejectsReservedResourceAttributes
- **Executable evidence:** TestBuildResourceIgnoresReservedCustomAttributes
- **Fuzz evidence:** FuzzResourceAttributes
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** BuildResource
- **Public API:** Config.Resource
- **Public API:** ServiceConfig
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/architecture.md
- **Documentation:** docs/compatibility.md
- **Upstream status:** Semantic Conventions 1.40.0 and later releases are monitored without automatic schema migration.
- **Reconsider when:** A supported OpenTelemetry Go release requires a different stable resource schema or merge contract.
- **Authoritative URL:** https://raw.githubusercontent.com/open-telemetry/semantic-conventions/v1.40.0/README.md

## TELEMETRY-DEC-007: Privacy-minimized semantic instrumentation profile

- **Status:** resolved
- **Owner:** telemetry maintainers
- **Classification:** optional behavior
- **Decision scope:** defensive
- **Specification:** OpenTelemetry Semantic Conventions 1.40.0
- **Version:** OpenTelemetry Semantic Conventions 1.40.0
- **Source authority:** semconv-source
- **Section:** HTTP, database, messaging, and general error semantic conventions
- **Requirement strength:** OPTIONAL
- **Issue:** Semantic conventions define broad signal attributes while stable library instrumentation must choose a bounded subset and avoid payload, target, credential, and high-cardinality data.
- **Credible interpretation:** Emit every available semantic attribute
- **Credible interpretation:** Use custom names only
- **Credible interpretation:** Emit stable low-cardinality semantic keys plus bounded package-specific metrics and omit sensitive values
- **Peer behavior:** OpenTelemetry maintained instrumentations commonly emit richer protocol and network attributes; this package uses the same stable key vocabulary for its smaller owned profile.
- **Selected behavior:** HTTP, PostgreSQL, queue, and cache instrumentation emits fixed operation, route-template, status-class, system, and error-class attributes plus bounded package metrics; it excludes raw paths, queries, hosts, headers, addresses, SQL, arguments, keys, messages, errors, and panic values.
- **Rationale:** Semantic interoperability does not require collecting sensitive or unbounded data that the package cannot safely govern.
- **Security consequences:** Default telemetry omits payloads, credentials, raw identifiers, and error text.
- **Resource consequences:** Attribute domains and metric cardinality remain bounded by fixed enums, configured allow-lists, and cardinality limits.
- **Compatibility consequences:** Consumers receive a documented subset rather than every attribute emitted by broader maintained instrumentations.
- **Wire consequences:** OTLP attributes use stable semantic keys where selected and package-specific names for explicitly owned metrics.
- **Executable evidence:** TestHandlerRecordsOnlyBoundedServerAttributes
- **Executable evidence:** TestTracerNeverRecordsSQLOrArguments
- **Executable evidence:** TestWrapHandlerDoesNotRecordMessagesOrErrors
- **Executable evidence:** TestInstrumenterRecordsOnlyFixedCacheSemantics
- **Differential evidence:** specification/maintained-peers.json
- **Public API:** nethttp.NewHandler
- **Public API:** nethttp.NewTransport
- **Public API:** gopostgres.New
- **Public API:** goqueue.New
- **Public API:** gocache.New
- **Documentation:** docs/specification-decisions.md
- **Documentation:** docs/instrumentation.md
- **Documentation:** docs/privacy.md
- **Documentation:** docs/cardinality.md
- **Upstream status:** Semantic Convention stability and release changes are monitored independently from package privacy policy.
- **Reconsider when:** A stable required semantic attribute conflicts with the package privacy or cardinality contract.
- **Authoritative URL:** https://raw.githubusercontent.com/open-telemetry/semantic-conventions/v1.40.0/README.md
