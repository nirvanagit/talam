---
title: "Architecture Decision Records"
weight: 10
---
**Related:** read by [`../architecture/overview.md`](/docs/architecture/) and every doc under [`../components/`](../components/).

ADRs capture *why*, not *what* — the "what" lives in [`architecture/`](../architecture/) and [`components/`](../components/) and changes as the system evolves. An ADR is accepted once and stays as a historical record; a later decision that changes course adds a new ADR that supersedes it, it doesn't edit the old one.

| ID | Title | Status | Affects |
|---|---|---|---|
| [ADR-0001](/docs/decisions/0001-server-agent-operator-split/) | Split into server, agent, and operator instead of one binary | Accepted | [operator](/docs/components/operator/), [agent](/docs/components/agent/), [server](/docs/components/server/) |
| [ADR-0002](/docs/decisions/0002-deterministic-analyzers-then-llm/) | Analyzers stay deterministic; LLM only explains and proposes | Accepted | [analyzer-interface](/docs/concepts/analyzer-interface/), [server](/docs/components/server/) |
| [ADR-0003](/docs/decisions/0003-human-in-the-loop-remediation/) | Remediation requires human approval before apply | Accepted | [remediation-flow](/docs/concepts/remediation-flow/), [server](/docs/components/server/), [agent](/docs/components/agent/) |
| [ADR-0004](/docs/decisions/0004-mesh-agnostic-analyzer-interface/) | Analyzer interface is mesh-agnostic; Istio ships as the first backend | Accepted | [analyzer-interface](/docs/concepts/analyzer-interface/) |
| [ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/) | Incidents and remediation are Kubernetes CRDs, agent-owned, synced from server | Accepted | [agent](/docs/components/agent/), [server](/docs/components/server/), [finding-and-incident](/docs/concepts/finding-and-incident/), [remediation-flow](/docs/concepts/remediation-flow/) |
| [ADR-0006](/docs/decisions/0006-mcp-evidence-enrichment/) | MCP-backed evidence enrichment stays deterministic, server-side, and pre-fetch | Accepted | [server](/docs/components/server/), [mesh-mcp](/docs/components/mesh-mcp/) |

## Template

New ADRs follow [`0000-template.md`](https://github.com/nirvanagit/talam/blob/main/docs/decisions/0000-template.md) in the repo.
