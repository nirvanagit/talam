# ADR-0001: Split into server, agent, and operator instead of one binary

**Status:** Accepted — REST transport superseded by [`ADR-0008`](0008-kubernetes-native-fleet-transport.md); the three-component split and its trust-boundary principle (server holds no credentials into a spoke cluster it doesn't own) are unchanged.
**Related:** affects [`../components/operator/README.md`](../components/operator/README.md), [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md); see [`../architecture/overview.md#system-topology`](../architecture/overview.md#system-topology)

## Context

k8sgpt ships as a single CLI binary that runs ad hoc against one cluster's kubeconfig. Mesh diagnostics don't fit that model as cleanly: a service mesh often spans multiple clusters as one logical boundary, findings are more valuable when correlated across clusters and over time, and the component that talks to an LLM provider and holds fleet history has different security and availability requirements than the component that watches Kubernetes CRDs.

## Decision

talam is three separable components:

- **Operator** — a standard Kubernetes controller-runtime reconciliation loop, one per cluster. Installs and keeps the agent healthy via the `MeshDiagnostics` CR. Nothing else.
- **Agent** — one per cluster. Collects mesh state (Istio CRDs, istiod xDS, Envoy admin API), runs the deterministic analyzer engine locally, ships structured findings to the server.
- **Server** — central, one per fleet. Aggregates findings across clusters, correlates them into incidents, is the only component that calls an LLM, holds history, and brokers remediation approval.

## Consequences

Each component has a small, auditable blast radius — a compromised or buggy operator can't leak mesh data or call an LLM, because it has no code path to either. The cost is more moving parts to deploy and version than a single binary, and agents must tolerate the server being unreachable (buffer-and-retry) rather than failing scans outright. See [`../concepts/security-model.md`](../concepts/security-model.md) for how the trust boundaries are enforced.
