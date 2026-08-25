# Component: server

**Related:** reads [`ADR-0001`](../../decisions/0001-server-agent-operator-split.md), [`ADR-0002`](../../decisions/0002-deterministic-analyzers-then-llm.md), [`ADR-0003`](../../decisions/0003-human-in-the-loop-remediation.md), [`ADR-0005`](../../decisions/0005-crd-native-incidents-and-resolutions.md), [`../../concepts/finding-and-incident.md`](../../concepts/finding-and-incident.md), [`../../concepts/remediation-flow.md`](../../concepts/remediation-flow.md), [`../../api/crds.md`](../../api/crds.md); links to [`../agent/README.md`](../agent/README.md)

talam-server is the only component that talks to an LLM provider and the only place that holds cross-cluster, historical state ([ADR-0001](../../decisions/0001-server-agent-operator-split.md)). It can run in-cluster or entirely outside Kubernetes — agents only need network reachability to it.

## Subsystems

| Subsystem | Job |
|---|---|
| Ingest API | Receives finding batches from every registered [agent](../agent/README.md); validates against the [`Finding`](../../concepts/finding-and-incident.md#finding) schema; deduplicates against recent history. |
| Correlation engine | Groups related findings across agents into one [`Incident`](../../concepts/finding-and-incident.md#incident). |
| LLM gateway | Turns a Finding/Incident + its evidence into a prompt, calls the configured model, and validates the structured response before storing it. Two calls per finding — explain, then propose — never one ([ADR-0002](../../decisions/0002-deterministic-analyzers-then-llm.md)). |
| Remediation broker | Holds proposed fixes in a pending state, exposes them for CLI/UI approval and, filtered by `?cluster=`, for the originating agent's [CRD Sync](../agent/README.md) loop to pull ([ADR-0005](../../decisions/0005-crd-native-incidents-and-resolutions.md)) — talam-server never pushes into a cluster or holds credentials for one. Full state flow: [`../../concepts/remediation-flow.md`](../../concepts/remediation-flow.md). Never auto-applies in v0.1 ([ADR-0003](../../decisions/0003-human-in-the-loop-remediation.md)). |
| History store | Durable store (Postgres) for findings, incidents, remediation decisions, and outcomes. |
| Fleet API + UI | REST API (v0.1; gRPC is roadmap) backing `talamctl`, the agent's CRD Sync loop, and a built-in web dashboard — incidents, their LLM explanations, and pending/decided remediation proposals, with one-click approve/reject. Served by talam-server itself as a static bundle (`internal/server/ui`), no separate deploy. v0.1 has no auth in front of it; OIDC is roadmap — put it behind a trusted network or a reverse-proxying ingress until then. |

## LLM provider interface

Provider-agnostic by design: the gateway targets a small internal interface (`Explain(ctx, evidence) → string`, `Propose(ctx, explanation) → RemediationProposal`), backed by Claude by default with an adapter for any OpenAI-compatible endpoint — so an org can point talam at a self-hosted model for air-gapped clusters. Every output is schema-validated before storage; a malformed or hallucinated patch is rejected and surfaced as "explanation only."

Which provider/model backs the gateway is itself Kubernetes-native: talam-server polls a [`ModelBinding`](../../api/crds.md#modelbinding) custom resource in its own namespace and rebuilds its provider whenever that object changes — no restart needed to switch models or rotate to a different provider. This sits on top of, not instead of, the plain environment-variable path (`ANTHROPIC_API_KEY`, `TALAM_LLM_MODEL`), which remains the fallback when no `ModelBinding` is found or talam-server runs outside Kubernetes entirely (which it can — see the intro above).
