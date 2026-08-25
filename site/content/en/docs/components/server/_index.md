---
title: "Component: server"
weight: 30
---
**Related:** reads [`ADR-0001`](/docs/decisions/0001-server-agent-operator-split/), [`ADR-0002`](/docs/decisions/0002-deterministic-analyzers-then-llm/), [`ADR-0003`](/docs/decisions/0003-human-in-the-loop-remediation/), [`ADR-0005`](/docs/decisions/0005-crd-native-incidents-and-resolutions/), [`ADR-0006`](/docs/decisions/0006-mcp-evidence-enrichment/), [`../../concepts/finding-and-incident.md`](/docs/concepts/finding-and-incident/), [`../../concepts/remediation-flow.md`](/docs/concepts/remediation-flow/), [`../../api/crds.md`](/docs/api/crds/); links to [`../agent/README.md`](/docs/components/agent/), [`../mesh-mcp/README.md`](/docs/components/mesh-mcp/)

talam-server is the only component that talks to an LLM provider and the only place that holds cross-cluster, historical state ([ADR-0001](/docs/decisions/0001-server-agent-operator-split/)). It can run in-cluster or entirely outside Kubernetes — agents only need network reachability to it.

## Subsystems

| Subsystem | Job |
|---|---|
| Ingest API | Receives finding batches from every registered [agent](/docs/components/agent/); validates against the [`Finding`](/docs/concepts/finding-and-incident/#finding) schema; deduplicates against recent history. |
| Correlation engine | Groups related findings across agents into one [`Incident`](/docs/concepts/finding-and-incident/#incident). |
| Evidence enrichment | Deterministic table (`internal/server/enrich`) mapping a finding's analyzer to a fixed set of MCP tool calls, run before every LLM call — richer evidence without the LLM ever choosing what to fetch ([ADR-0006](/docs/decisions/0006-mcp-evidence-enrichment/), [`../mesh-mcp/README.md`](/docs/components/mesh-mcp/)). |
| LLM gateway | Turns a Finding/Incident + its (enriched) evidence into a prompt, calls the configured model, and validates the structured response before storing it. Two calls per finding — explain, then propose — never one ([ADR-0002](/docs/decisions/0002-deterministic-analyzers-then-llm/)). |
| Remediation broker | Holds proposed fixes in a pending state, exposes them for CLI/UI approval and, filtered by `?cluster=`, for the originating agent's [CRD Sync](/docs/components/agent/) loop to pull ([ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/)) — talam-server never pushes into a cluster or holds credentials for one. Full state flow: [`../../concepts/remediation-flow.md`](/docs/concepts/remediation-flow/). Never auto-applies in v0.1 ([ADR-0003](/docs/decisions/0003-human-in-the-loop-remediation/)). |
| History store | Durable store (Postgres) for findings, incidents, remediation decisions, and outcomes. |
| Fleet API + UI | REST API (v0.1; gRPC is roadmap) backing `talamctl`, the agent's CRD Sync loop, and a built-in web dashboard — incidents, their LLM explanations, and pending/decided remediation proposals, with one-click approve/reject. Served by talam-server itself as a static bundle (`internal/server/ui`), no separate deploy. v0.1 has no auth in front of it; OIDC is roadmap — put it behind a trusted network or a reverse-proxying ingress until then. |

## LLM provider interface

Provider-agnostic by design: the gateway targets a small internal interface (`Explain(ctx, evidence) → string`, `Propose(ctx, explanation) → RemediationProposal`), backed by Claude by default with an adapter for any OpenAI-compatible endpoint — so an org can point talam at a self-hosted model for air-gapped clusters. Every output is schema-validated before storage; a malformed or hallucinated patch is rejected and surfaced as "explanation only."

Which provider/model backs the gateway is itself Kubernetes-native: talam-server polls a [`ModelBinding`](/docs/api/crds/#modelbinding) custom resource in its own namespace and rebuilds its provider whenever that object changes — no restart needed to switch models or rotate to a different provider. This sits on top of, not instead of, the plain environment-variable path (`ANTHROPIC_API_KEY`, `TALAM_LLM_MODEL`), which remains the fallback when no `ModelBinding` is found or talam-server runs outside Kubernetes entirely (which it can — see the intro above).
