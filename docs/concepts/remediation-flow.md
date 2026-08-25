# Remediation flow

**Related:** reads [`ADR-0003`](../decisions/0003-human-in-the-loop-remediation.md), [`finding-and-incident.md`](finding-and-incident.md); read by [`../components/server/README.md`](../components/server/README.md), [`../components/agent/README.md`](../components/agent/README.md)

talam never writes to a cluster on its own initiative ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md)). Every proposed fix passes through explicit approval before an [agent](../components/agent/README.md) applies anything, and every apply is dry-run first.

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
Finding ──► Proposal ──► Pending review ──► Human decides
  (agent)   (LLM: explain     (CLI / UI)      │
             + propose)                       ├─ approve ─► Dry run ─► Applied (outcome recorded)
                                               ├─ edit ────► Dry run ─► Applied (outcome recorded)
                                               └─ reject ──► Recorded, not applied
```

Rejected proposals and failed dry-runs are recorded with their reason, never silently discarded — the [server](../components/server/README.md)'s history store is what makes "has talam seen this before" and remediation-quality tracking possible.

## Approval modes

Two knobs govern friction, both owned by the server:

- **`approvalMode`** — `manual` (every proposal needs an explicit action) or `policy-gated` (an allowlist of low-risk patch types can auto-apply; everything else waits). v0.1 ships `manual` only — see [ADR-0003](../decisions/0003-human-in-the-loop-remediation.md) and the [roadmap](../architecture/overview.md#roadmap).
- **`riskTier` thresholds** — decide which proposals are even eligible to reach auto-apply once `policy-gated` mode exists.
