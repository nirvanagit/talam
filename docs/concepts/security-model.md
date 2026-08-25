# Security model

**Related:** reads [`ADR-0001`](../decisions/0001-server-agent-operator-split.md); read by [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`finding-and-incident.md`](finding-and-incident.md)

The server/agent/operator split ([ADR-0001](../decisions/0001-server-agent-operator-split.md)) exists partly to keep trust boundaries small and legible. Four mechanisms enforce it:

## Agent RBAC

The agent's `ClusterRole` is read-only (`get`/`list`/`watch`) on core resources and Istio API groups. Write verbs are granted only on the specific patch-application path, scoped to Istio CRDs — never `Secret`, never RBAC objects. Defined alongside the agent's Deployment; see [`../components/agent/README.md`](../components/agent/README.md).

## Agent ↔ server transport

mTLS between every agent and talam-server, with a per-cluster identity/certificate so a compromised agent can be revoked individually without affecting the rest of the fleet.

## Evidence redaction

Collectors strip `Secret` contents and header/payload bodies before evidence ever reaches the [`Finding.RawEvidence`](finding-and-incident.md#finding) field — only config shape and metadata are eligible to leave the cluster, and only config shape and metadata are what the LLM gateway ever sees.

## Approval audit trail

Every [`RemediationProposal`](remediation-flow.md), its approver identity, its dry-run output, and its applied diff are retained in the server's history store — remediation is always attributable to a person, not just to "talam."
