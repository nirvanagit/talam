# Architecture overview

**Related:** reads [`0001`](../decisions/0001-server-agent-operator-split.md), [`0002`](../decisions/0002-deterministic-analyzers-then-llm.md), [`0003`](../decisions/0003-human-in-the-loop-remediation.md), [`0004`](../decisions/0004-mesh-agnostic-analyzer-interface.md), [`0007`](../decisions/0007-agent-never-applies-remediation.md); links out to every node under [`../components/`](../components/) and [`../concepts/`](../concepts/)

talam reads Kubernetes and Istio API objects, runs deterministic analyzers against them, and hands the results to an LLM to explain in plain English — the same loop k8sgpt runs one layer down the stack. talam's failures live one layer up: not "pod won't schedule" but "pod is healthy, mesh routing is not."

## Design principles

1. **Deterministic first.** Every finding starts as a rule-based check against real API/xDS state ([ADR-0002](../decisions/0002-deterministic-analyzers-then-llm.md)). The LLM explains and proposes — it never originates a diagnosis.
2. **No blind writes.** talam never mutates cluster state at all — every proposed fix requires explicit human approval, then is applied by an external system talam only exposes it to, never by talam itself ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md), [ADR-0007](../decisions/0007-agent-never-applies-remediation.md)).
3. **Fleet-native.** One server, many clusters, many agents ([ADR-0001](../decisions/0001-server-agent-operator-split.md)). A mesh boundary spanning clusters is a first-class case.
4. **Mesh-agnostic core.** The analyzer interface doesn't know Istio exists ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)).

## System topology

### Single-cluster view

Each mesh-bearing cluster runs one [operator](../components/operator/README.md) and one [agent](../components/agent/README.md).

```mermaid
flowchart TB
    subgraph OpLane[" "]
        direction TB
        Op(["⚙️ <b>talam-operator</b>"])
    end

    subgraph AgLane[" "]
        direction TB
        Ag(["🔄 <b>talam-agent</b>"])
    end

    subgraph ExtLane[" "]
        direction TB
        ExtSys(["🔌 <b>External system</b><br/><i>GitOps · pipeline · kubectl</i>"])
    end

    MD[("MeshDiagnostics<br/><i>cluster config</i>")]
    RBAC["ServiceAccount<br/>Role · RoleBinding<br/><i>read-only on mesh resources</i>"]

    subgraph WatchedCRDs["watched CRDs (synced from server)"]
        direction TB
        MI[("MeshIncident")]
        MR[("MeshResolution")]
        MB[("ModelBinding")]
    end

    Api{{"Kubernetes API"}}
    Istio{{"Istio Control Plane<br/><i>istiod / xDS</i>"}}
    Workloads["User workloads<br/><i>Istio-injected pods</i>"]

    Op == "1 reconciles" ==> MD
    Op == "2 provisions" ==> RBAC
    RBAC -. "grants read-only access to" .-> Ag

    Ag == "3 watches" ==> WatchedCRDs
    Ag == "4 reads state" ==> Api
    Ag == "4 reads xDS" ==> Istio

    ExtSys == "5 subscribes to" ==> MR
    ExtSys == "6 applies patch" ==> Api
    ExtSys == "6 applies patch" ==> Istio
    ExtSys == "7 reports outcome" ==> MR

    Api -.-> Workloads
    Istio -.->|configures| Workloads

    classDef opStyle fill:#0891b2,color:#fff,stroke:#075985,stroke-width:2px
    classDef agStyle fill:#0f766e,color:#fff,stroke:#075985,stroke-width:2px
    classDef extStyle fill:#7c3aed,color:#fff,stroke:#5b21b6,stroke-width:2px
    classDef crdStyle fill:#fff8dc,stroke:#f59e0b,stroke-width:2px
    classDef k8sStyle fill:#f3f4f6,stroke:#0b1220,stroke-width:2px
    classDef laneStyle fill:none,stroke:none

    class Op opStyle
    class Ag agStyle
    class ExtSys extStyle
    class MD,MI,MR,MB crdStyle
    class Api,Istio,Workloads,RBAC k8sStyle
    class OpLane,AgLane,ExtLane laneStyle

    linkStyle 0,1 stroke:#0891b2,stroke-width:2.5px
    linkStyle 2 stroke:#94a3b8,stroke-width:1.5px,stroke-dasharray:4 3
    linkStyle 3,4,5 stroke:#0f766e,stroke-width:2.5px
    linkStyle 6,7,8,9 stroke:#7c3aed,stroke-width:2.5px
```

**Key flows** — numbered to match the diagram:
1. **Operator reconciles** `MeshDiagnostics` (cluster-scoped config: which analyzers, scan interval, server endpoint)
2. **Operator provisions** the ServiceAccount/Role/RoleBinding the agent needs — read-only on every mesh resource, full stop ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md))
3. **Agent watches** `MeshIncident`, `MeshResolution`, `ModelBinding` — all synced down from talam-server
4. **Agent reads** live cluster state (API objects + Istio xDS) to see what's actually running — read-only, same as everything else it touches outside its own CRDs
5. **External system subscribes** to `MeshResolution` — a GitOps controller, an existing config pipeline, or a human via `kubectl`; talam never assumes which
6. **External system applies** the patch on its own initiative and authority — talam-agent has no write RBAC on any mesh resource to do this itself
7. **External system reports** the outcome back onto `MeshResolution.status`; the agent's `ResolutionReconciler` then relays it to talam-server (shown in the fleet-wide view below)

Cyan edges belong to the operator, teal to the agent, violet to the external system that actually applies remediation — three components, three colors, and no two of them ever write to the same target, so responsibility stays visually separable even where paths cross.

All CRD state is queryable with `kubectl get meshincidents`, `kubectl get meshresolutions`, etc. — no separate database.

### Fleet-wide view

Agents from multiple clusters report findings to a central talam-server, which coordinates with an LLM provider:

```mermaid
flowchart TB
    User(["👤 SRE / CLI"])
    Server["<b>talam-server</b><br/><i>findings aggregation · LLM coordination<br/>proposal broker · fleet history</i>"]
    LLM["<b>LLM Provider</b><br/><i>Claude, GPT, etc — pluggable</i>"]

    User == "approves" ==> Server
    Server == "explain + propose" ==> LLM
    LLM == "response" ==> Server

    subgraph ClusterA["Cluster A — EKS / GKE / on-prem"]
        direction TB
        OpA(["⚙️ operator"])
        AgentA(["🔄 agent"])
        ExtA(["🔌 external system"])
        ApiA{{"K8s API + Istio"}}
        OpA -. "provisions read-only RBAC for" .-> AgentA
        AgentA == "read" ==> ApiA
        ExtA == "applies" ==> ApiA
    end

    subgraph ClusterB["Cluster B"]
        direction TB
        OpB(["⚙️ operator"])
        AgentB(["🔄 agent"])
        ExtB(["🔌 external system"])
        ApiB{{"K8s API + Istio"}}
        OpB -. "provisions read-only RBAC for" .-> AgentB
        AgentB == "read" ==> ApiB
        ExtB == "applies" ==> ApiB
    end

    Server == "proposals" ==> ClusterA
    Server == "proposals" ==> ClusterB
    AgentA == "findings + relayed outcomes" ==> Server
    AgentB == "findings + relayed outcomes" ==> Server

    classDef serverStyle fill:#0f766e,color:#fff,stroke:#075985,stroke-width:2px
    classDef llmStyle fill:#0891b2,color:#fff,stroke:#075985,stroke-width:2px
    classDef userStyle fill:#f3f4f6,color:#0b1220,stroke:#94a3b8
    classDef clusterStyle fill:#f8fafc,stroke:#0f766e,stroke-width:2px
    classDef opStyle fill:#0891b2,color:#fff,stroke:#075985
    classDef agStyle fill:#0f766e,color:#fff,stroke:#075985
    classDef extStyle fill:#7c3aed,color:#fff,stroke:#5b21b6,stroke-width:2px
    classDef apiStyle fill:#fff8dc,stroke:#f59e0b,stroke-width:2px

    class Server serverStyle
    class LLM llmStyle
    class User userStyle
    class ClusterA,ClusterB clusterStyle
    class OpA,OpB opStyle
    class AgentA,AgentB agStyle
    class ExtA,ExtB extStyle
    class ApiA,ApiB apiStyle
```

**Key insight:** the server is the sole authority on *what* needs fixing (proposals flow server → agent, one direction only); each agent is the sole writer to *its own* cluster's talam CRDs, but reads (never writes) mesh resources directly ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)) — the external system in each cluster is the sole writer of mesh resources, and it's the agent that relays its outcome back to the server (findings + outcomes flow agent → server, the mirror of proposals). The two clusters never talk to each other, and an external system in cluster A never talks to cluster B — all coordination is brokered through the server, so there's exactly one path between any two components and no line has to guess which box it started from.

Component-level detail: [operator](../components/operator/README.md) · [agent](../components/agent/README.md) · [server](../components/server/README.md).

## LLM integration

The [server](../components/server/README.md)'s LLM gateway makes two calls per finding, never one, and both are schema-validated before storage — see [`../concepts/finding-and-incident.md`](../concepts/finding-and-incident.md) for the shapes involved:

1. **Explain** — evidence in, plain-language root cause out.
2. **Propose** — explanation in, a structured `RemediationProposal` out (patch, risk tier, resourceVersion pin). See [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md) for what happens to a proposal next — including why talam itself never applies it ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md)).

A malformed or hallucinated patch is rejected at the schema boundary and surfaced as "explanation only" rather than shown to a user.

## Roadmap

| Phase | Scope |
|---|---|
| **v0.1** | Single-cluster Istio, core analyzer catalog ([`../api/analyzer-catalog.md`](../api/analyzer-catalog.md)), manual-approval remediation talam only proposes and exposes, never applies ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md), [ADR-0007](../decisions/0007-agent-never-applies-remediation.md)) |
| **v0.2** | Multi-cluster correlation into incidents, web dashboard, richer xDS-drift analyzers |
| **v0.3** | Policy-gated auto-approval for allowlisted low-risk classes (still handed off, never applied by talam); GitOps-native mode (proposals rendered as PRs against the repo an external controller already reconciles from) |
| **v0.4** | Linkerd analyzer set behind the existing interface ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)); ambient/sidecar-less (Cilium) support with new eBPF-aware collectors |

See [`../roadmap.md`](../roadmap.md) for the fuller breakdown.

## See also

- Full visual version of this document: [talam Architecture artifact](https://claude.ai/code/artifact/7038ccdf-cd29-4692-bba9-e3cd1add674f)
