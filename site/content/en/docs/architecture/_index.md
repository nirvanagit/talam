---
title: "Architecture overview"
weight: 10
---
**Related:** reads [`0001`](/docs/decisions/0001-server-agent-operator-split/), [`0002`](/docs/decisions/0002-deterministic-analyzers-then-llm/), [`0003`](/docs/decisions/0003-human-in-the-loop-remediation/), [`0004`](/docs/decisions/0004-mesh-agnostic-analyzer-interface/); links out to every node under [`../components/`](../components/) and [`../concepts/`](../concepts/)

talam reads Kubernetes and Istio API objects, runs deterministic analyzers against them, and hands the results to an LLM to explain in plain English — the same loop k8sgpt runs one layer down the stack. talam's failures live one layer up: not "pod won't schedule" but "pod is healthy, mesh routing is not."

## Design principles

1. **Deterministic first.** Every finding starts as a rule-based check against real API/xDS state ([ADR-0002](/docs/decisions/0002-deterministic-analyzers-then-llm/)). The LLM explains and proposes — it never originates a diagnosis.
2. **No blind writes.** talam never mutates cluster state without explicit human approval ([ADR-0003](/docs/decisions/0003-human-in-the-loop-remediation/)).
3. **Fleet-native.** One server, many clusters, many agents ([ADR-0001](/docs/decisions/0001-server-agent-operator-split/)). A mesh boundary spanning clusters is a first-class case.
4. **Mesh-agnostic core.** The analyzer interface doesn't know Istio exists ([ADR-0004](/docs/decisions/0004-mesh-agnostic-analyzer-interface/)).

## System topology

Each mesh-bearing cluster runs one [operator](/docs/components/operator/) and one [agent](/docs/components/agent/). Agents report structured [findings](/docs/concepts/finding-and-incident/) to a central [talam-server](/docs/components/server/), the only component that talks to an LLM provider and the only place fleet-wide history lives.

```
                     ┌────────────────────┐        ┌──────────────┐
                     │    talam-server    │◄──────►│ LLM provider │
   ┌───────────┐     │  (aggregation,     │        │  (pluggable) │
   │ SRE / CLI │────►│   correlation,     │        └──────────────┘
   └───────────┘     │   remediation      │
                      │   broker, history) │
                      └─────────▲──────────┘
                                │  findings (mTLS gRPC)
              ┌─────────────────┴─────────────────┐
              │                                    │
   ┌──────────┴──────────┐              ┌──────────┴──────────┐
   │      cluster A       │              │      cluster B       │
   │ ┌──────────┐┌───────┐│              │ ┌──────────┐┌───────┐│
   │ │ operator ││ agent ││              │ │ operator ││ agent ││
   │ └──────────┘└───┬───┘│              │ └──────────┘└───┬───┘│
   │        ┌─────────┴───┴────┐         │        ┌─────────┴───┴────┐
   │        │ apiserver/istiod │         │        │ apiserver/istiod │
   │        └──────────────────┘         │        └──────────────────┘
   └──────────────────────────┘          └──────────────────────────┘
```

Component-level detail: [operator](/docs/components/operator/) · [agent](/docs/components/agent/) · [server](/docs/components/server/).

## LLM integration

The [server](/docs/components/server/)'s LLM gateway makes two calls per finding, never one, and both are schema-validated before storage — see [`../concepts/finding-and-incident.md`](/docs/concepts/finding-and-incident/) for the shapes involved:

1. **Explain** — evidence in, plain-language root cause out.
2. **Propose** — explanation in, a structured `RemediationProposal` out (patch, risk tier, dry-run diff). See [`../concepts/remediation-flow.md`](/docs/concepts/remediation-flow/) for what happens to a proposal next.

A malformed or hallucinated patch is rejected at the schema boundary and surfaced as "explanation only" rather than shown to a user.

## Roadmap

| Phase | Scope |
|---|---|
| **v0.1** | Single-cluster Istio, core analyzer catalog ([`../api/analyzer-catalog.md`](/docs/api/analyzer-catalog/)), manual-approval-only remediation ([ADR-0003](/docs/decisions/0003-human-in-the-loop-remediation/)) |
| **v0.2** | Multi-cluster correlation into incidents, web dashboard, richer xDS-drift analyzers |
| **v0.3** | Policy-gated auto-apply for allowlisted low-risk classes; GitOps mode (proposals as PRs) |
| **v0.4** | Linkerd analyzer set behind the existing interface ([ADR-0004](/docs/decisions/0004-mesh-agnostic-analyzer-interface/)); ambient/sidecar-less (Cilium) support with new eBPF-aware collectors |

## See also

- Full visual version of this document: [talam Architecture artifact](https://claude.ai/code/artifact/7038ccdf-cd29-4692-bba9-e3cd1add674f)
