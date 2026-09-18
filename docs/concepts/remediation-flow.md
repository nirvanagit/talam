# Remediation flow

**Related:** reads [`ADR-0003`](../decisions/0003-human-in-the-loop-remediation.md), [`ADR-0005`](../decisions/0005-crd-native-incidents-and-resolutions.md), [`ADR-0007`](../decisions/0007-agent-never-applies-remediation.md), [`finding-and-incident.md`](finding-and-incident.md), [`../api/crds.md`](../api/crds.md#meshincident--meshresolution); read by [`../components/server/README.md`](../components/server/README.md), [`../components/agent/README.md`](../components/agent/README.md)

talam never writes to a mesh resource, on its own initiative or anyone else's ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)). Every proposed fix passes through explicit human approval, same as before ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md)) — but the party that actually executes the fix is an **external system**: a GitOps controller, an existing config-management pipeline, or a human running `kubectl` directly. talam's job ends at producing a well-formed, reviewable, resourceVersion-pinned patch and exposing it; someone else's system decides whether and how to apply it.

The hand-off point is a Kubernetes object, not a REST call: a `RemediationProposal` a human approves (dashboard or `talamctl`) is mirrored down into a [`MeshResolution`](../api/crds.md#meshincident--meshresolution) CR in the agent's own cluster. From there, whatever is watching `MeshResolution` objects in that cluster picks it up. See [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md) for why this is agent-owned rather than server-driven, and [ADR-0007](../decisions/0007-agent-never-applies-remediation.md) for why the agent stops at exposing it.

## RemediationProposal

```go
type RemediationProposal struct {
    FindingID   string
    Summary     string          // one-line, for CLI/UI list views
    Explanation string          // full root-cause narrative
    RiskTier    RiskTier        // Low | Medium | High
    Patch       []JSONPatchOp   // structured, validated against the live schema
}
```

## State flow

```
Finding ──► Proposal ──► Pending review ──► Human decides (dashboard/talamctl)
  (agent)   (LLM: explain     (server)         │
             + propose)                        ├─ approve ─┐
                                                └─ reject ──┤  Recorded, not exposed further
                                                             │
                        (agent's Sync loop, every ~15s)      ▼
                     MeshResolution.spec.approved = true (advisory — ADR-0007)
                                                             │
                              ┌──────────────────────────────┴───────────────────────┐
                              │           subscribed by an external system            │
                              │   (GitOps controller · config pipeline · kubectl)     │
                              └──────────────────────────────┬───────────────────────┘
                                                             ▼
                                        external system applies (or doesn't) and writes
                                          MeshResolution.status.outcome = Applied|Failed
                                                             │
                        (ResolutionReconciler, every ~5s)    ▼
                              relays outcome to talam-server, never applies anything
```

Rejected proposals are recorded with their reason, never silently discarded — the [server](../components/server/README.md)'s history store is what makes "has talam seen this before" and remediation-quality tracking possible. The `MeshResolution` CR (see [`../api/crds.md`](../api/crds.md#meshincident--meshresolution)) is the artifact a subscribing system watches; the server's REST decision endpoint is still what a human interacts with to approve or reject, but nothing in talam ever reconciles `spec.approved` into an apply — that reconciliation, if it happens at all, belongs to whatever system owns config changes in that cluster.

If no such system exists in a given cluster, the escape hatch is the same one ADR-0005 already established for `spec.approved`: `kubectl patch` the resolution's `status` directly (`outcome`, `appliedBy`, `appliedAt`) once you've applied the change by hand — talam picks it up on the next reconcile tick same as it would from any other system.

## Approval modes

Two knobs govern friction, both owned by the server:

- **`approvalMode`** — `manual` (every proposal needs an explicit action) or `policy-gated` (an allowlist of low-risk patch types can flip `spec.approved` automatically; everything else waits for a human). v0.1 ships `manual` only — see [ADR-0003](../decisions/0003-human-in-the-loop-remediation.md) and the [roadmap](../roadmap.md). Either way, `spec.approved` is advisory metadata a subscribing system may or may not honor ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)) — talam itself never acts on it.
- **`riskTier` thresholds** — decide which proposals are even eligible for `policy-gated` auto-approval once that mode exists.
