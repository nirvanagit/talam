# Architecture overview

**Related:** reads [`0001`](../decisions/0001-server-agent-operator-split.md), [`0002`](../decisions/0002-deterministic-analyzers-then-llm.md), [`0003`](../decisions/0003-human-in-the-loop-remediation.md), [`0004`](../decisions/0004-mesh-agnostic-analyzer-interface.md), [`0007`](../decisions/0007-agent-never-applies-remediation.md), [`0008`](../decisions/0008-kubernetes-native-fleet-transport.md), [`0009`](../decisions/0009-agent-side-evidence-gathering-and-fleet-correlation.md); links out to every node under [`../components/`](../components/) and [`../concepts/`](../concepts/)

**Note:** the fleet-wide diagram and REST-call descriptions further below (`GET /v1/incidents`, `POST /v1/findings`, etc.) still describe the v0.1 transport, which [ADR-0008](../decisions/0008-kubernetes-native-fleet-transport.md) supersedes with cross-cluster Kubernetes watches. That diagram is pending an update; the single-cluster diagram immediately below is current.

talam reads Kubernetes and Istio API objects, runs deterministic analyzers against them, and hands the results to an LLM to explain in plain English — the same loop k8sgpt runs one layer down the stack. talam's failures live one layer up: not "pod won't schedule" but "pod is healthy, mesh routing is not."

## Design principles

1. **Deterministic first.** Every finding starts as a rule-based check against real API/xDS state ([ADR-0002](../decisions/0002-deterministic-analyzers-then-llm.md)). The LLM explains and proposes — it never originates a diagnosis.
2. **No blind writes.** talam never mutates cluster state at all — every proposed fix requires explicit human approval, then is applied by an external system talam only exposes it to, never by talam itself ([ADR-0003](../decisions/0003-human-in-the-loop-remediation.md), [ADR-0007](../decisions/0007-agent-never-applies-remediation.md)).
3. **Fleet-native.** One server, many clusters, many agents ([ADR-0001](../decisions/0001-server-agent-operator-split.md)). A mesh boundary spanning clusters is a first-class case.
4. **Mesh-agnostic core.** The analyzer interface doesn't know Istio exists ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)).

## System topology

### Single-cluster view

Each mesh-bearing cluster runs one [operator](../components/operator/README.md) and one [agent](../components/agent/README.md). This reflects the current design — [ADR-0007](../decisions/0007-agent-never-applies-remediation.md) (agent never applies remediation), [ADR-0008](../decisions/0008-kubernetes-native-fleet-transport.md) (`MeshIncident`/canonical `MeshResolution` live in the server's cluster, not here), and [ADR-0009](../decisions/0009-agent-side-evidence-gathering-and-fleet-correlation.md) (evidence gathering via local MCP servers). Almost everything below happens inside this one cluster — the single exception is one clearly-marked edge where the agent reaches into the server's cluster, which is exactly the one cross-cluster credential this design allows.

```mermaid
flowchart TB
    subgraph Spoke["Spoke cluster"]
        direction TB

        Op(["⚙️ <b>talam-operator</b>"])
        Ag(["🔄 <b>talam-agent</b>"])
        ExtSys(["🔌 <b>External system</b><br/><i>GitOps · pipeline · kubectl</i>"])

        MD[("MeshDiagnostics<br/><i>cluster config</i>")]
        LocalRBAC["ServiceAccount · Role<br/><i>local, read-only on mesh resources</i>"]
        FleetSecret["Secret: fleet kubeconfig<br/><i>pre-provisioned out of band —<br/>operator only mounts it, never mints it</i>"]
        MCP["MCPServer registrations<br/><i>talam-mesh-mcp + others, local only</i>"]
        LocalMR[("MeshResolution<br/><i>read-only mirror</i>")]

        Api{{"Kubernetes API"}}
        Istio{{"Istio Control Plane<br/><i>istiod / xDS</i>"}}
        Workloads["User workloads<br/><i>Istio-injected pods</i>"]

        Op == "1 reconciles" ==> MD
        Op == "2 provisions" ==> LocalRBAC
        Op == "3 mounts" ==> FleetSecret
        LocalRBAC -. "grants local read-only" .-> Ag
        FleetSecret -. "grants cross-cluster access" .-> Ag

        Ag == "4 reads state" ==> Api
        Ag == "4 reads xDS" ==> Istio
        Ag == "5 gathers evidence" ==> MCP
        Ag == "7 mirrors down" ==> LocalMR

        ExtSys == "8 subscribes to" ==> LocalMR
        ExtSys == "9 applies patch" ==> Api
        ExtSys == "9 applies patch" ==> Istio
        ExtSys == "10 reports outcome" ==> LocalMR

        MCP -.-> Istio
        Api -.-> Workloads
        Istio -.->|configures| Workloads
    end

    ServerCluster[["Server cluster<br/><i>fleet-&lt;clusterName&gt; namespace</i><br/>MeshIncident · MeshResolution (canonical)"]]

    Ag == "6 creates/updates MeshIncident;<br/>watches MeshResolution; relays outcome" ==> ServerCluster

    classDef opStyle fill:#0891b2,color:#fff,stroke:#075985,stroke-width:2px
    classDef agStyle fill:#0f766e,color:#fff,stroke:#075985,stroke-width:2px
    classDef extStyle fill:#7c3aed,color:#fff,stroke:#5b21b6,stroke-width:2px
    classDef crdStyle fill:#fff8dc,stroke:#f59e0b,stroke-width:2px
    classDef k8sStyle fill:#f3f4f6,stroke:#0b1220,stroke-width:2px
    classDef mcpStyle fill:#fef3c7,stroke:#d97706,stroke-width:2px
    classDef remoteStyle fill:#f8fafc,stroke:#0f766e,stroke-width:2px,stroke-dasharray:6 3

    class Op opStyle
    class Ag agStyle
    class ExtSys extStyle
    class MD,LocalMR crdStyle
    class Api,Istio,Workloads,LocalRBAC,FleetSecret k8sStyle
    class MCP mcpStyle
    class ServerCluster remoteStyle

    linkStyle 0,1,2 stroke:#0891b2,stroke-width:2.5px
    linkStyle 3,4 stroke:#94a3b8,stroke-width:1.5px,stroke-dasharray:4 3
    linkStyle 5,6,7,8 stroke:#0f766e,stroke-width:2.5px
    linkStyle 9,10,11,12 stroke:#7c3aed,stroke-width:2.5px
    linkStyle 16 stroke:#0f766e,stroke-width:3px,stroke-dasharray:3 2
```

**Key flows** — numbered to match the diagram:
1. **Operator reconciles** `MeshDiagnostics` (cluster-scoped config: which analyzers, scan interval, server endpoint, `fleetKubeconfigSecretRef`)
2. **Operator provisions** the agent's *local* ServiceAccount/Role — read-only on every mesh resource, full stop ([ADR-0007](../decisions/0007-agent-never-applies-remediation.md))
3. **Operator mounts** the pre-provisioned fleet-credential `Secret` into the agent's Deployment — it does **not** mint this credential itself, only plumbs it through ([ADR-0008](../decisions/0008-kubernetes-native-fleet-transport.md#credential-bootstrap))
4. **Agent reads** live local cluster state (API objects + Istio xDS) — read-only, same as everything else it touches outside its own writes
5. **Agent gathers evidence** from local `MCPServer` registrations (`talam-mesh-mcp` and others) while producing a finding — same-cluster only, no network exception needed ([ADR-0009](../decisions/0009-agent-side-evidence-gathering-and-fleet-correlation.md))
6. **Agent creates/updates `MeshIncident` in the server's cluster** — the one edge that leaves this cluster, using the credential from step 3. Also watches the canonical `MeshResolution` there and relays any reported outcome back to it ([ADR-0008](../decisions/0008-kubernetes-native-fleet-transport.md))
7. **Agent mirrors** the canonical `MeshResolution` down as a **read-only local copy** — this is what the external system actually subscribes to, so it never needs a cross-cluster credential of its own
8. **External system subscribes** to the local `MeshResolution` mirror — a GitOps controller, an existing config pipeline, or a human via `kubectl`; talam never assumes which
9. **External system applies** the patch on its own initiative and authority — talam-agent has no write RBAC on any mesh resource to do this itself
10. **External system reports** the outcome by patching `status.conditions` on the local mirror (the readinessGate pattern, [ADR-0008](../decisions/0008-kubernetes-native-fleet-transport.md#the-object-model--zero-new-crd-types)) — the agent relays it back up as part of step 6

Cyan edges belong to the operator, teal to the agent (including the one thick dashed edge crossing the cluster boundary — still agent-owned, just visually marked as leaving), violet to the external system that actually applies remediation. No two components ever write to the same target, so responsibility stays visually separable even where paths cross. `MCPServer` (amber) and the server cluster (dashed-border box) are the two additions since the last version of this diagram.

Local CRD state is queryable with `kubectl get meshdiagnostics`, `kubectl get meshresolutions`, etc. — but `MeshIncident` and the canonical `MeshResolution` now live in the server's cluster, not here; `kubectl get meshincidents` only works there.

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
