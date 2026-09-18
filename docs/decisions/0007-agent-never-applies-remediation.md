# ADR-0007: talam-agent never applies remediation; external systems subscribe and act

**Status:** Accepted
**Supersedes:** the apply mechanism in [`ADR-0003`](0003-human-in-the-loop-remediation.md) — the human-approval-gate principle it established still holds, just enforced one layer up (below).
**Related:** reads [`ADR-0001`](0001-server-agent-operator-split.md), [`ADR-0005`](0005-crd-native-incidents-and-resolutions.md), [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md), [`../concepts/security-model.md`](../concepts/security-model.md); affects [`../components/agent/README.md`](../components/agent/README.md), [`../api/crds.md`](../api/crds.md), [`../concepts/finding-and-incident.md`](../concepts/finding-and-incident.md), [`../concepts/object-model.md`](../concepts/object-model.md)

## Context

Through ADR-0005, `talam-agent` was the sole writer of remediation to a cluster: once a human approved a proposal, the agent's `ResolutionReconciler` dry-ran and applied the patch directly against Istio CRDs, using a `patch` verb the operator granted it.

That assumption doesn't hold for every environment talam runs in. Many clusters already have an established system of record for mesh configuration — a GitOps controller (Argo CD, Flux) reconciling from a Git repo, an internal platform tool, a change-management pipeline with its own approval gates. In those environments, talam directly patching a `DestinationRule` is not a convenience, it's a conflict: it creates a second, uncoordinated writer to the same resources, defeats the existing system's drift detection, and routes around whatever approval process that system already enforces.

talam's actual value in this position is upstream of applying anything: detecting the misconfiguration, explaining it, and proposing a structured, reviewable fix. Handing that fix to whatever system already owns config changes in a given cluster — rather than talam competing with it — is a better fit for how mesh configuration is actually managed in most organizations.

## Decision

`talam-agent` stops applying patches. `Applier` (the type that dry-ran and executed a patch against Istio CRDs) is removed entirely, along with the `patch` RBAC verb the operator granted the agent on Istio CRDs — the agent's `ClusterRole` is now read-only against every mesh resource, full stop.

`MeshResolution` keeps its role as the CRD realization of a `RemediationProposal` (ADR-0005), but its job changes from "trigger for the agent to act on" to **the artifact an external system subscribes to**:

- `spec` still carries the full proposed patch (target, JSON Patch ops, the `resourceVersion` it was computed against) — unchanged, this is still the thing a consuming system needs to act.
- `spec.triggered` is renamed `spec.approved` and becomes purely advisory: it still mirrors a human's approval decision from the dashboard, but talam-agent never reads it to decide anything, because talam-agent never decides to act at all. A subscribing system may treat it as a gate on its own automation or ignore it — that choice belongs to whoever owns the apply, not to talam.
- `status.outcome` (`"Applied"` | `"Failed"`), `status.appliedBy`, and `status.appliedAt` are new fields **talam never writes**. Whatever external system actually applies the patch — a GitOps controller reconciling the same intent from its own source, a human running `kubectl patch` as an escape hatch, a custom operator — writes them via the CRD's status subresource once it acts.
- `ResolutionReconciler` still runs in the agent, but its job shrinks to exactly one thing: watch for `status.outcome` becoming non-empty, and relay it to talam-server (`POST /v1/proposals/{id}/outcome`) so the fleet-wide history stays accurate. It never touches a mesh resource. Retries apply only to the relay, same as before (ADR-0005's `outcomeReported` bookkeeping, unchanged in spirit).
- `IncidentReconciler` computes `MeshIncident.status.complete` from `status.outcome` being non-empty across referenced resolutions, instead of the old `status.performed` boolean — same semantics (a `Rejected` resolution never gets an outcome, so it never counts), different source of truth.

The human-approval principle ADR-0003 established doesn't go away — it moves. talam still never originates a diagnosis without deterministic evidence (ADR-0002), still never applies a proposal without a human decision recorded against it (the dashboard's approve/reject flow is unchanged), and now additionally never *executes* that decision itself. The gate that used to be "has a human approved this" followed immediately by talam's own apply is now "has a human approved this" followed by *whatever system the organization already trusts* to make the change — which may itself require its own approval (a GitOps PR review, a change ticket) on top.

## Consequences

The agent's trust boundary shrinks materially: it needs no write access to any mesh resource in any cluster, only read access for analysis plus write access to talam's own CRDs (`MeshIncident`/`MeshResolution`, unchanged). A compromised or buggy agent can no longer mutate Istio configuration, full stop — the worst it can do is misreport findings or drop an outcome relay, both already-recoverable failure modes. This is a strictly smaller blast radius than ADR-0005 shipped with.

The cost is that talam alone can no longer close the loop end-to-end: getting from "proposal approved" to "cluster actually fixed" now depends on an external system existing and being configured to watch `MeshResolution` objects. For a cluster with no such system, the escape hatch is the same as ADR-0005's: `kubectl patch` the resolution's status directly. Demo and getting-started flows need to show this explicitly, since "approve and watch it happen" no longer works out of the box — see [`../getting-started.md`](../getting-started.md).

This also means talam's dry-run validation goes away: it was implemented as a server-side dry-run patch call, which requires the same RBAC verb as the real write — keeping it would have meant keeping exactly the write access this ADR removes. A consuming system is expected to do its own pre-apply validation with whatever tooling it already uses; talam's contribution stops at a well-formed, resourceVersion-pinned patch.

No change to talam-server: it already only ever talks to the agent's synced CRD state, never touches a cluster directly (ADR-0001), and `RecordOutcome` already treated the outcome as an opaque report rather than something it originated.
