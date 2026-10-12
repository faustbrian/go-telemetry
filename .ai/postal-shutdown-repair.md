# Postal shutdown repair

Assurance: Tier C lifecycle fix; Tier D compatible v2 patch release.
Baseline: main 948935d2149306a9a0a1d4984936d7051d51ed60; released v2.0.2.

Contract: metric flushing must not prevent trace-provider cleanup. Preserve
the shorter caller/configured total deadline, pre-canceled rejection, terminal
error aggregation and one-attempt concurrent/repeated shutdown semantics.
The same provider-cleanup ownership applies to partial initialization.

Consumers: Postal's short-lived migration/operator telemetry, runtime owners,
and the service lifecycle adapter. No application-local exporter workaround.
Risks: exhausting the total deadline before later cleanup, duplicate shutdown,
lost error results, or exceeding the caller's deadline.

Verification: failing blocked-metric/exporter-closure regression, caller-deadline
variant, affected root and lifecycle-adapter tests, targeted lifecycle race
checks, unchanged public API check, required release CI and clean v2 consumer.
Independent complete final-diff review is required before release.

State: owner fix implemented. The original both-signal timeout regression fails
on the baseline (trace exporter never closes) and passes with the repair for
configured and shorter caller deadlines. A partial-initialization counterfactual
also fails on the baseline. Root, service adapter and successor-adapter tests,
targeted lifecycle race checks (three repetitions), go vet and inventory pass.
Bounded make check passes, including lint, static analysis, every package test,
local documentation and public API compatibility. Native release contract and
inventory checks pass. Independent reviewer 01a12dcf-3623-7333-a5c5-3541aef275a4
approved the complete final diff, including v2.0.3 metadata. Hosted exact-source
CI, release rehearsal, public consumption and Postal adoption remain pending.
