---
title: "Security model"
weight: 50
---
**Related:** reads [`ADR-0001`](/docs/decisions/0001-server-agent-operator-split/); read by [`../components/agent/README.md`](/docs/components/agent/), [`../components/server/README.md`](/docs/components/server/), [`finding-and-incident.md`](/docs/concepts/finding-and-incident/)

The server/agent/operator split ([ADR-0001](/docs/decisions/0001-server-agent-operator-split/)) exists partly to keep trust boundaries small and legible. Four mechanisms enforce it:

## Agent RBAC

The agent's `ClusterRole` is read-only (`get`/`list`/`watch`) on core resources and Istio API groups. Write verbs are granted only on the specific patch-application path, scoped to Istio CRDs — never `Secret`, never RBAC objects. Defined alongside the agent's Deployment; see [`../components/agent/README.md`](/docs/components/agent/).

## Agent ↔ server transport

mTLS between every agent and talam-server, with a per-cluster identity/certificate so a compromised agent can be revoked individually without affecting the rest of the fleet.

## Evidence redaction

Collectors strip `Secret` contents and header/payload bodies before evidence ever reaches the [`Finding.RawEvidence`](/docs/concepts/finding-and-incident/#finding) field — only config shape and metadata are eligible to leave the cluster, and only config shape and metadata are what the LLM gateway ever sees.

## Approval audit trail

Every [`RemediationProposal`](/docs/concepts/remediation-flow/), its approver identity, its dry-run output, and its applied diff are retained in the server's history store — remediation is always attributable to a person, not just to "talam."
