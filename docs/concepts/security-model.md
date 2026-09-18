# Security model

**Related:** reads [`ADR-0001`](../decisions/0001-server-agent-operator-split.md), [`ADR-0007`](../decisions/0007-agent-never-applies-remediation.md); read by [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`finding-and-incident.md`](finding-and-incident.md)

The server/agent/operator split ([ADR-0001](../decisions/0001-server-agent-operator-split.md)) exists partly to keep trust boundaries small and legible. Four mechanisms enforce it:

## Agent RBAC

The agent's `ClusterRole` is **read-only** (`get`/`list`/`watch`) on core resources and Istio API groups — full stop. Since [ADR-0007](../decisions/0007-agent-never-applies-remediation.md), the agent never applies remediation itself, so it holds no write verb on any mesh resource, not even a scoped one — never `Secret`, never RBAC objects, and now never `DestinationRule`/`VirtualService`/`Gateway`/`ServiceEntry` either. Its only write access is to talam's own CRDs (`MeshIncident`/`MeshResolution`, both status and spec, namespaced to `talam-system`). A compromised or buggy agent cannot mutate mesh configuration; the worst it can do is misreport findings or drop an outcome relay. Defined alongside the agent's Deployment; see [`../components/agent/README.md`](../components/agent/README.md).

## Agent ↔ server transport

mTLS between every agent and talam-server, with a per-cluster identity/certificate so a compromised agent can be revoked individually without affecting the rest of the fleet.

## Evidence redaction

Collectors strip `Secret` contents and header/payload bodies before evidence ever reaches the [`Finding.RawEvidence`](finding-and-incident.md#finding) field — only config shape and metadata are eligible to leave the cluster, and only config shape and metadata are what the LLM gateway ever sees.

## Approval audit trail

Every [`RemediationProposal`](remediation-flow.md), its approver identity, and the outcome reported back by whatever external system applied it are retained in the server's history store — remediation is always attributable to a person and a system, not just to "talam." Since talam never executes the apply itself ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)), a second boundary applies here too: talam's audit trail covers everything up to and including approval and the reported outcome, not the mechanics of the apply itself — that's the subscribing system's own audit responsibility.
