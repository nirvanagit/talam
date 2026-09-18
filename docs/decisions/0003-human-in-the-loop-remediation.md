# ADR-0003: Remediation requires human approval before apply

**Status:** Accepted — apply mechanism superseded by [`ADR-0007`](0007-agent-never-applies-remediation.md); the approval-gate principle below still holds.
**Related:** reads [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md); affects [`../components/server/README.md`](../components/server/README.md), [`../components/agent/README.md`](../components/agent/README.md)

## Context

A mesh control plane is exactly the kind of system where an automated bad write (a wrong `PeerAuthentication` change, a `DestinationRule` patch that routes traffic somewhere unintended) has outsized blast radius — mTLS and routing misconfigurations can silently break or expose traffic across many services at once. talam also generates its proposed fixes via an LLM, which makes an unreviewed auto-apply path materially riskier than a deterministic tool's would be.

## Decision

v0.1 ships with exactly one approval mode: **manual**. Every `RemediationProposal` sits in a pending state until a human explicitly approves it via CLI or UI. See [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md) for the full state diagram.

**Amended by [ADR-0007](0007-agent-never-applies-remediation.md):** the sentence originally here described the agent dry-running and applying the patch itself once approved. talam-agent no longer does either — approval still gates whether a subscribing external system is expected to act, but talam itself stops at approval, never executes. The principle this ADR establishes (no unreviewed apply) is unchanged; only who performs the apply changed.

A `policy-gated` mode (an allowlist of low-risk patch types eligible for auto-apply) is scoped for a later release, not this one — see [`../architecture/overview.md#roadmap`](../architecture/overview.md#roadmap).

## Consequences

Every applied change is attributable to a person and auditable end to end, and a bad LLM-proposed patch is caught by a human before it touches the cluster, not after. The cost is throughput: talam can't self-heal without someone in the loop yet, which is the intended trade for v0.1 while proposal quality builds a track record.
