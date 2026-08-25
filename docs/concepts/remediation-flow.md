# Remediation flow

**Related:** reads [`ADR-0003`](../decisions/0003-human-in-the-loop-remediation.md), [`ADR-0005`](../decisions/0005-crd-native-incidents-and-resolutions.md), [`finding-and-incident.md`](finding-and-incident.md), [`../api/crds.md`](../api/crds.md#meshincident--meshresolution); read by [`../components/server/README.md`](../components/server/README.md), [`../components/agent/README.md`](../components/agent/README.md)

talam never writes to a cluster on its own initiative ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md)). Every proposed fix passes through explicit approval before an [agent](../components/agent/README.md) applies anything, and every apply is dry-run first.

The trigger for applying a fix is a Kubernetes object, not a REST call: a `RemediationProposal` a human approves (dashboard or `talamctl`) is mirrored down into a [`MeshResolution`](../api/crds.md#meshincident--meshresolution) CR in the agent's own cluster, and it's `spec.triggered` flipping `true` on that object — reconciled by the agent, not pushed by the server — that actually causes the apply. See [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md) for why this is agent-owned rather than server-driven.

## RemediationProposal

```go
type RemediationProposal struct {
    FindingID   string
    Summary     string          // one-line, for CLI/UI list views
    Explanation string          // full root-cause narrative
    RiskTier    RiskTier        // Low | Medium | High
    Patch       []JSONPatchOp   // structured, validated against the live schema
    DryRunDiff  string          // rendered YAML diff for human review
}
```

## State flow

```
Finding ──► Proposal ──► Pending review ──► Human decides (dashboard/talamctl)
  (agent)   (LLM: explain     (server)         │
             + propose)                        ├─ approve ─┐
                                                └─ reject ──┤  Recorded, not applied
                                                             │
                        (agent's Sync loop, every ~15s)      ▼
                     MeshResolution.spec.triggered = true (ADR-0005)
                                                             │
                        (ResolutionReconciler, every ~5s)    ▼
                              Dry run ─► Applied ─► outcome reported to server,
                                                     MeshResolution.status updated
```

Rejected proposals and failed dry-runs are recorded with their reason, never silently discarded — the [server](../components/server/README.md)'s history store is what makes "has talam seen this before" and remediation-quality tracking possible. The `MeshResolution` CR (see [`../api/crds.md`](../api/crds.md#meshincident--meshresolution)) is what actually drives the apply; the server's REST decision endpoint is still what a human interacts with, but it's the CR's `spec.triggered` a controller reconciles, not a REST poll.

## Approval modes

Two knobs govern friction, both owned by the server:

- **`approvalMode`** — `manual` (every proposal needs an explicit action) or `policy-gated` (an allowlist of low-risk patch types can auto-apply; everything else waits). v0.1 ships `manual` only — see [ADR-0003](../decisions/0003-human-in-the-loop-remediation.md) and the [roadmap](../architecture/overview.md#roadmap).
- **`riskTier` thresholds** — decide which proposals are even eligible to reach auto-apply once `policy-gated` mode exists.
