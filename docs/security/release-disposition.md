# Published security disposition

Reviewed on 2026-10-07 by telemetry maintainers. This document distinguishes
historical source behavior, corrected library controls, upstream dependency
selection and deployment impact. It is not a claim that every application
using an earlier version was exploitable, or that scanner success establishes
the absence of vulnerabilities.

## Published sources inspected

| Version | Source commit |
| --- | --- |
| v1.0.0 | `f3afa80b28a654fea8a73c7ca2dfa512616f2d02` |
| v1.1.0 | `1dcf8de07e8a4c7e667b3fbe50afe09a29ade7ef` |
| v1.1.1 | `c6f5be7b846e9f81405daf3a48a0ae6ba7e2cb6e` |
| v1.2.0 | `355cc9fe36b493527aba3e44eb6bc120ecfda57c` |
| v2.0.0 | `5b98c6215bb0b43dc8731a3871905aa8ffe6713d` |
| v2.0.1 | `a64c883a7c44526bae5a9db59e8e7f55b0bed3ba` |

The four inspected v1 sources share the historical controls described below.
This is an explicit source list, not an assertion about uninspected versions
or downstream dependency overrides. The v2 corrections were integrated at
`6134f9eb3c8cdceb9469558416c82573a57b6a0d` and first published in v2.0.0.

## Corrected library boundaries

The inspected v1 sources had confirmed security-relevant configuration
admission and diagnostic gaps:

- Direct `BuildResource` constructed and sorted attributes before applying
  resource admission. Direct exporter configuration lacked header budgets;
  queue, cardinality, view and baggage configuration lacked the present
  maximum or aggregate admission controls.
- Rejected configuration values and TLS file diagnostics could appear in
  returned errors. An application that included sensitive values in those
  fields and exposed those diagnostics could disclose them.
- Configured TLS material paths invoked filesystem reads directly, without
  the present caller-owned reader, byte budget and cancellation boundary.
  Custom CA configuration also augmented platform trust rather than defining
  the complete custom pool.

Version 2 validates direct construction boundaries before owned allocation,
bounds configuration, returns categorical diagnostics and requires an
explicit TLS reader. Existing regression coverage is maintained in
`security_admission_internal_test.go`,
`otlp/security_admission_internal_test.go`,
`metric/security_admission_internal_test.go` and
`propagation/security_admission_internal_test.go`.

The source and tests establish these defects and corrections. Application
impact depends on who supplies configuration, controls TLS material and can
observe returned diagnostics. They do not establish remote attacker reach,
an observed confidentiality incident or an application-independent severity.
Do not expose configuration or raw historical diagnostics to untrusted peers.

## Contract hardening and lifecycle corrections

Version 2 makes signal activation, plaintext transport and process-global
registration explicit choices instead of active defaults. It also prevents
a pre-canceled shutdown call from consuming the sole shutdown attempt.
These are default-policy and lifecycle corrections, not evidence of a
historical authentication bypass or attacker-triggered shutdown failure.

Request-specific `TrustInbound` strengthens the application-owned baggage
trust decision. The inspected v1 propagation documentation already required
a proven authenticated boundary before selecting `TrustedInbound`. Neither
flag is a replacement for application authentication; review middleware
ordering and the proof callback when migrating.

## Upstream dependency selection

The [changelog](../../CHANGELOG.md) records upstream remediation separately
from owned library controls:

- All four inspected v1 sources select OpenTelemetry SDK 1.44.0. Version
  2.0.0 selects 1.45.0 and records the upstream correction for conditional
  endpoint disclosure in verbose internal diagnostics; v2.0.1 selects SDK
  1.46.0. This does not establish that ordinary telemetry exports disclosed
  endpoints in every earlier deployment.
- Versions 1.0.0 and 1.1.0 select gRPC 1.82.1; v1.1.1 and v1.2.0 select
  1.83.1. Version 1.1.1 first publishes this repository's dependency upgrade
  for fragmented DATA-frame receive-buffer exhaustion.
- Version 2.0.0 first selects gRPC 1.83.2 and records the upstream xDS-server
  missing-authority correction. Including that dependency does not establish
  that the telemetry library or a consumer runs an affected xDS server.

Applications must inspect their resolved dependency graph and the upstream
advisory conditions. A source dependency selection is not a deployment scan
or a blanket affected-version assessment of every consumer.

## Upgrade and disclosure disposition

The supported line is v2, as stated in the [security policy](../../SECURITY.md).
Upgrade to the latest supported release using
`github.com/faustbrian/go-telemetry/v2` and Go 1.27. Review explicit signal and
transport choices, finite configuration limits, caller-owned TLS readers,
request-specific baggage proof and shutdown ownership before deployment.
Version 1 is unsupported; no original-module patch is promised.

This document records the source-specific corrections, not a published
own-library vulnerability advisory. Own-library advisory applicability is
not established merely by these configuration tests: it requires assessment
of the affected trust boundary, practical impact and precise affected
versions under the private reporting policy. Confirmed vulnerabilities
require an advisory and coordinated disclosure; absence of an advisory must
not be interpreted as a security clearance for historical versions.

Telemetry maintainers own that assessment. Reassess on a private report,
evidence of attacker-controlled configuration or diagnostic exposure, a
changed upstream advisory, or the next residual-risk review on 2026-11-07.
The [threat model](../threat-model.md) retains current accepted risks and their
application owners. Scanner results, public consumer compatibility and live
deployment verification remain distinct evidence boundaries.
