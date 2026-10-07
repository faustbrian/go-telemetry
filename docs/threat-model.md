# Threat model

- **Version:** 2026-09-13
- **Owner:** telemetry maintainers
- **Next residual-risk review:** 2026-11-07, or sooner on a listed review
  condition. This schedule does not extend a changed or overdue risk.
- **Review trigger:** public API, exporter transport, propagation, resource,
  lifecycle, OpenTelemetry dependency, or trust-boundary changes

Current model applicability check: 2026-10-07, published v2.0.1 source
`a64c883a7c44526bae5a9db59e8e7f55b0bed3ba`. The model version above retains
its original identity. Current security qualification and deployment behavior
remain separate evidence boundaries; documentation is not a scanner result.

## Assets and boundaries

Assets are exporter credentials and TLS keys, application availability,
bounded process memory and goroutines, trustworthy trace parentage, and the
absence of secrets or high-cardinality identifiers from telemetry. Untrusted
boundaries include inbound HTTP and messaging metadata, caller-provided
configuration, Collector network paths, exporter responses, and shutdown
contexts. Process-local callers and authenticated workloads are not trusted to
supply unlimited data.

## Threats and controls

| Threat | Control |
| --- | --- |
| Implicit network, background work, or process-global mutation | Defaults disable traces, metrics, and global registration; each is enabled explicitly. |
| Collector impersonation or credential disclosure | TLS is the default; plaintext, custom CAs, client keys, and skip-verification are explicit. TLS material is validated before use. |
| Malformed or oversized propagation | Combined header byte and baggage item limits are checked before parsing; hostile propagation fuzz targets run in the selected CI gate. |
| Unauthenticated baggage authority | Baggage is disabled by default, keys are allow-listed, and HTTP trusted extraction requires request-specific authentication proof. |
| Configuration memory exhaustion | Resource attributes, exporter headers, metric views, allowed attributes, and histogram boundaries have count or aggregate byte limits. |
| TLS material memory exhaustion or implicit filesystem access | Configured paths require a caller-owned cancellation-aware reader, with a finite constructor timeout and a revalidated 1 MiB budget per result. Custom CA trust is explicit and does not load ambient system roots. |
| Cardinality and queue exhaustion | Metric cardinality, trace queue and batch sizes, exporter retry horizons, and operation/attribute vocabularies are bounded. |
| Secret leakage | Default instrumentation excludes payloads, raw URLs, headers, arbitrary errors, and identifiers; documentation prohibits secret resource attributes. |
| Shutdown hangs or accidental abandonment | Caller and configuration deadlines reach providers and exporters; a pre-canceled call does not consume the one owned shutdown attempt. Custom exporters must cooperate with cancellation, and process supervisors enforce hard termination. |

## Accepted residual risks

| Risk | Owner and rationale | Mitigation and review condition |
| --- | --- | --- |
| Explicit plaintext may expose telemetry on a compromised network. | Service owner; local and same-trust-zone Collectors sometimes cannot terminate TLS. | Require network policy and workload authentication. Review when the deployment boundary or Collector topology changes. |
| `InsecureSkipVerify` permits Collector impersonation. | Service owner; retained for exceptional migration and diagnostic use. | Time-bound outside local development and prefer a mounted CA. Review on every production use or credential change. |
| A caller can implement an incorrect trusted-inbound proof. | Service owner; authentication policy belongs to the application boundary. | Base proof on completed authentication, test false and true paths, and never use source address alone. Review when middleware order or authentication changes. |
| A trusted-inbound proof callback can block the request indefinitely. | Service owner; Go cannot preempt an arbitrary in-process callback without leaking work, and authentication policy remains application-owned. | Use a prompt-return, non-blocking callback over already-computed authentication state, or isolate the process boundary. Review every callback implementation and whenever authentication middleware changes. |
| Shutdown that times out after provider work starts is not retried. | Runtime owner; OpenTelemetry providers may be partially closed and do not promise retry-safe shutdown. | Allocate shutdown budget after application drain, require custom exporters to honor context cancellation, and enforce a process-level termination deadline. Review if upstream defines retry-safe lifecycle semantics. |
| Trace context within the configured byte limit may be attacker-controlled. | Telemetry maintainers; cross-service trace continuation requires accepting valid W3C context. | Treat it only as correlation, never authorization; invalid or oversized context is ignored. Review if trace IDs gain application authority. |

Production operators must combine these library controls with egress policy,
Collector authentication and redaction, secret management, backend access
control, and process resource limits. A trace queue retains at most 65,536
span-interface slots, with a batch no larger than the queue. Each metric stream
retains at most 100,000 series; histogram boundaries and allowed attributes
remain independently bounded. These limits do not bound arbitrary payloads
added through the standard OpenTelemetry API or the number of caller-created
instruments, so applications still own instrumentation scope and process limits.

An explicit TLS reader is a trusted collaborator. Its application owner must
ensure context cancellation reaches underlying I/O; Go cannot force an
arbitrary reader to return without leaking detached work. Use a maintained
bounded reader or already-loaded material and a supervised process. Reassess
each reader implementation, transport change, or deadline overrun. Standard TLS
without a custom CA delegates platform trust-root loading to Go; application
owners must control that platform trust configuration and reassess it after
changes to deployed trust roots or runtime platforms.
