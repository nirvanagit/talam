# Architecture overview

**Related:** reads [`0001`](../decisions/0001-server-agent-operator-split.md), [`0002`](../decisions/0002-deterministic-analyzers-then-llm.md), [`0003`](../decisions/0003-human-in-the-loop-remediation.md), [`0004`](../decisions/0004-mesh-agnostic-analyzer-interface.md); links out to every node under [`../components/`](../components/) and [`../concepts/`](../concepts/)

talam reads Kubernetes and Istio API objects, runs deterministic analyzers against them, and hands the results to an LLM to explain in plain English — the same loop k8sgpt runs one layer down the stack. talam's failures live one layer up: not "pod won't schedule" but "pod is healthy, mesh routing is not."

## Design principles

1. **Deterministic first.** Every finding starts as a rule-based check against real API/xDS state ([ADR-0002](../decisions/0002-deterministic-analyzers-then-llm.md)). The LLM explains and proposes — it never originates a diagnosis.
2. **No blind writes.** talam never mutates cluster state without explicit human approval ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md)).
3. **Fleet-native.** One server, many clusters, many agents ([ADR-0001](../decisions/0001-server-agent-operator-split.md)). A mesh boundary spanning clusters is a first-class case.
4. **Mesh-agnostic core.** The analyzer interface doesn't know Istio exists ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)).

## System topology

### Single-cluster view

Each mesh-bearing cluster runs one [operator](../components/operator/README.md) and one [agent](../components/agent/README.md).

```mermaid
graph TB
    subgraph talam["talam-system namespace"]
        Op["<b>talam-operator</b><br/>Reconciles MeshDiagnostics<br/>Manages RBAC & lifecycle"]
        Ag["<b>talam-agent</b><br/>Syncs from server<br/>Reconciles CRDs<br/>Applies patches"]
    end
    
    subgraph k8s["Kubernetes cluster"]
        Api["<b>Kubernetes API</b><br/>(kube-apiserver)"]
        Istio["<b>Istio Control Plane</b><br/>(istiod)"]
        Workloads["<b>User Namespaces</b><br/>Istio-injected pods"]
    end
    
    subgraph crds["Custom Resources<br/>(stored in etcd)"]
        MD["MeshDiagnostics<br/>(config)"]
        MI["MeshIncident<br/>(findings)"]
        MR["MeshResolution<br/>(remediation)"]
        MS["MCPServer<br/>(evidence sources)"]
        MB["ModelBinding<br/>(LLM config)"]
    end
    
    Op -->|watches| MD
    Op -->|creates RBAC| Api
    
    Ag -->|watches| MI
    Ag -->|watches| MR
    Ag -->|watches| MB
    Ag -->|reads| Api
    Ag -->|reads xDS| Istio
    Ag -->|patches| Api
    Ag -->|patches| Istio
    
    Api -->|stores| crds
    Istio -->|configures| Workloads
    
    style talam fill:#0f766e,color:#fff,stroke:#0f766e
    style Op fill:#0891b2,color:#fff
    style Ag fill:#0891b2,color:#fff
    style k8s fill:#f3f4f6,stroke:#0b1220,stroke-width:2px
    style crds fill:#fff8dc,stroke:#f59e0b,stroke-width:2px
```

**Key flows:**
- **Operator** reconciles `MeshDiagnostics` (cluster-scoped config) and sets up RBAC
- **Agent** watches `MeshIncident` and `MeshResolution` CRDs (synced from server)
- **Agent** reads live cluster state (API + xDS) to understand what's running
- **Agent** applies patches to fix misconfigurations (dry-run first, then live)
- **All state lives in CRDs** — everything is auditable, queryable with kubectl

### Fleet-wide view

Agents from multiple clusters report findings to a central talam-server, which coordinates with an LLM provider:

```mermaid
graph TB
    User["👤 SRE / CLI<br/>REST API"]
    Server["<b>talam-server</b><br/>Findings aggregation<br/>LLM coordination<br/>Proposal broker<br/>Fleet history"]
    LLM["<b>LLM Provider</b><br/>Claude, GPT, etc<br/>(pluggable)"]
    
    User -->|approval| Server
    Server <-->|explain & propose| LLM
    
    subgraph ClusterA["Cluster A (EKS/GKE/on-prem)"]
        OpA["talam-operator"]
        AgentA["talam-agent"]
        ApiA["K8s API + Istio"]
    end
    
    subgraph ClusterB["Cluster B"]
        OpB["talam-operator"]
        AgentB["talam-agent"]
        ApiB["K8s API + Istio"]
    end
    
    Server -->|proposals| ClusterA
    Server -->|proposals| ClusterB
    
    AgentA -->|findings| Server
    AgentB -->|findings| Server
    
    OpA -.->|RBAC| ApiA
    AgentA -->|patch| ApiA
    AgentA -->|read state| ApiA
    
    OpB -.->|RBAC| ApiB
    AgentB -->|patch| ApiB
    AgentB -->|read state| ApiB
    
    style Server fill:#0f766e,color:#fff
    style LLM fill:#0891b2,color:#fff
    style User fill:#f3f4f6,color:#0b1220
    style ClusterA fill:#f3f4f6,stroke:#0f766e,stroke-width:2px
    style ClusterB fill:#f3f4f6,stroke:#0f766e,stroke-width:2px
```

**Key insight:** The server is the sole authority on what needs fixing (proposals come from server to agents). Agents are the sole writers to their clusters (deterministic reconciliation, no conflicts).

Component-level detail: [operator](../components/operator/README.md) · [agent](../components/agent/README.md) · [server](../components/server/README.md).

## LLM integration

The [server](../components/server/README.md)'s LLM gateway makes two calls per finding, never one, and both are schema-validated before storage — see [`../concepts/finding-and-incident.md`](../concepts/finding-and-incident.md) for the shapes involved:

1. **Explain** — evidence in, plain-language root cause out.
2. **Propose** — explanation in, a structured `RemediationProposal` out (patch, risk tier, dry-run diff). See [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md) for what happens to a proposal next.

A malformed or hallucinated patch is rejected at the schema boundary and surfaced as "explanation only" rather than shown to a user.

## Roadmap

| Phase | Scope |
|---|---|
| **v0.1** | Single-cluster Istio, core analyzer catalog ([`../api/analyzer-catalog.md`](../api/analyzer-catalog.md)), manual-approval-only remediation ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md)) |
| **v0.2** | Multi-cluster correlation into incidents, web dashboard, richer xDS-drift analyzers |
| **v0.3** | Policy-gated auto-apply for allowlisted low-risk classes; GitOps mode (proposals as PRs) |
| **v0.4** | Linkerd analyzer set behind the existing interface ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)); ambient/sidecar-less (Cilium) support with new eBPF-aware collectors |

## See also

- Full visual version of this document: [talam Architecture artifact](https://claude.ai/code/artifact/7038ccdf-cd29-4692-bba9-e3cd1add674f)
