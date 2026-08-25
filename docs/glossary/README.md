# Glossary

**Related:** read by every node in this repo — link here on first use of a term rather than redefining it inline.

**Agent** — the per-cluster component that collects mesh state and runs analyzers locally. See [`../components/agent/README.md`](../components/agent/README.md).

**Analyzer** — the unit of deterministic detection; reads a `MeshSnapshot`, returns `Finding`s. See [`../concepts/analyzer-interface.md`](../concepts/analyzer-interface.md).

**Finding** — the output of one analyzer run against one resource. See [`../concepts/finding-and-incident.md#finding`](../concepts/finding-and-incident.md#finding).

**Incident** — a server-side correlation of one or more Findings describing the same underlying problem. See [`../concepts/finding-and-incident.md#incident`](../concepts/finding-and-incident.md#incident).

**MeshSnapshot** — the point-in-time collected state an analyzer reads; built once per scan tick from that backend's collectors. See [`../concepts/analyzer-interface.md`](../concepts/analyzer-interface.md).

**Operator** — the per-cluster Kubernetes controller that installs and keeps the agent healthy; does not read mesh data or call an LLM. See [`../components/operator/README.md`](../components/operator/README.md).

**RemediationProposal** — an LLM-generated, schema-validated candidate fix for a Finding/Incident, held pending human approval. See [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md).

**Server (talam-server)** — the fleet-wide aggregation, correlation, LLM gateway, and remediation broker component. See [`../components/server/README.md`](../components/server/README.md).
