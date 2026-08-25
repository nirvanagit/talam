# Component: server

**Related:** reads [`ADR-0001`](../../decisions/0001-server-agent-operator-split.md), [`ADR-0002`](../../decisions/0002-deterministic-analyzers-then-llm.md), [`ADR-0003`](../../decisions/0003-human-in-the-loop-remediation.md), [`../../concepts/finding-and-incident.md`](../../concepts/finding-and-incident.md), [`../../concepts/remediation-flow.md`](../../concepts/remediation-flow.md); links to [`../agent/README.md`](../agent/README.md)

talam-server is the only component that talks to an LLM provider and the only place that holds cross-cluster, historical state ([ADR-0001](../../decisions/0001-server-agent-operator-split.md)). It can run in-cluster or entirely outside Kubernetes — agents only need network reachability to it.

## Subsystems

| Subsystem | Job |
|---|---|
| Ingest API | Receives finding batches from every registered [agent](../agent/README.md); validates against the [`Finding`](../../concepts/finding-and-incident.md#finding) schema; deduplicates against recent history. |
| Correlation engine | Groups related findings across agents into one [`Incident`](../../concepts/finding-and-incident.md#incident). |
| LLM gateway | Turns a Finding/Incident + its evidence into a prompt, calls the configured model, and validates the structured response before storing it. Two calls per finding — explain, then propose — never one ([ADR-0002](../../decisions/0002-deterministic-analyzers-then-llm.md)). |
| Remediation broker | Holds proposed fixes in a pending state, exposes them for CLI/UI approval, dispatches approved patches back to the originating agent for application. Full state flow: [`../../concepts/remediation-flow.md`](../../concepts/remediation-flow.md). Never auto-applies in v0.1 ([ADR-0003](../../decisions/0003-human-in-the-loop-remediation.md)). |
| History store | Durable store (Postgres) for findings, incidents, remediation decisions, and outcomes. |
| Fleet API + UI | gRPC/REST API backing the CLI and an optional web dashboard; auth via OIDC. |

## LLM provider interface

Provider-agnostic by design: the gateway targets a small internal interface (`Explain(ctx, evidence) → string`, `Propose(ctx, explanation) → RemediationProposal`), backed by Claude by default with an adapter for any OpenAI-compatible endpoint — so an org can point talam at a self-hosted model for air-gapped clusters. Every output is schema-validated before storage; a malformed or hallucinated patch is rejected and surfaced as "explanation only."
