# Architecture Decision Records

**Related:** read by [`../architecture/overview.md`](../architecture/overview.md) and every doc under [`../components/`](../components/).

ADRs capture *why*, not *what* — the "what" lives in [`architecture/`](../architecture/) and [`components/`](../components/) and changes as the system evolves. An ADR is accepted once and stays as a historical record; a later decision that changes course adds a new ADR that supersedes it, it doesn't edit the old one.

| ID | Title | Status | Affects |
|---|---|---|---|
| [ADR-0001](0001-server-agent-operator-split.md) | Split into server, agent, and operator instead of one binary | Accepted | [operator](../components/operator/README.md), [agent](../components/agent/README.md), [server](../components/server/README.md) |
| [ADR-0002](0002-deterministic-analyzers-then-llm.md) | Analyzers stay deterministic; LLM only explains and proposes | Accepted | [analyzer-interface](../concepts/analyzer-interface.md), [server](../components/server/README.md) |
| [ADR-0003](0003-human-in-the-loop-remediation.md) | Remediation requires human approval before apply | Accepted | [remediation-flow](../concepts/remediation-flow.md), [server](../components/server/README.md), [agent](../components/agent/README.md) |
| [ADR-0004](0004-mesh-agnostic-analyzer-interface.md) | Analyzer interface is mesh-agnostic; Istio ships as the first backend | Accepted | [analyzer-interface](../concepts/analyzer-interface.md) |
| [ADR-0005](0005-crd-native-incidents-and-resolutions.md) | Incidents and remediation are Kubernetes CRDs, agent-owned, synced from server | Accepted | [agent](../components/agent/README.md), [server](../components/server/README.md), [finding-and-incident](../concepts/finding-and-incident.md), [remediation-flow](../concepts/remediation-flow.md) |
| [ADR-0006](0006-mcp-evidence-enrichment.md) | MCP-backed evidence enrichment stays deterministic, server-side, and pre-fetch | Accepted | [server](../components/server/README.md), [mesh-mcp](../components/mesh-mcp/README.md) |

## Template

New ADRs follow [`0000-template.md`](0000-template.md).
