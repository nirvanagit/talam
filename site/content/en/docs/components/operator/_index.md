---
title: "Component: operator"
weight: 10
---
**Related:** reads [`ADR-0001`](/docs/decisions/0001-server-agent-operator-split/); links to [`../agent/README.md`](/docs/components/agent/), [`../../api/crds.md`](/docs/api/crds/)

The operator is deliberately dumb. It's a standard Kubernetes controller-runtime reconciliation loop whose only responsibility is making the cluster match a [`MeshDiagnostics`](/docs/api/crds/#meshdiagnostics) custom resource ([ADR-0001](/docs/decisions/0001-server-agent-operator-split/)).

## Reconciles

| Reconciles | Responsibility |
|---|---|
| [`MeshDiagnostics`](/docs/api/crds/#meshdiagnostics) | Top-level CR: which server to report to, scan interval, which analyzers are enabled, resource limits for the agent. |
| Agent Deployment | Creates/updates the [talam-agent](/docs/components/agent/) Deployment, ServiceAccount, and RBAC (ClusterRole scoped to read-only on core + Istio API groups — see [`../../concepts/security-model.md`](/docs/concepts/security-model/)). |
| Agent health | Watches agent readiness; flips `MeshDiagnostics.status.agentHealthy`; restarts on crash-loop beyond a backoff threshold. |
| Upgrade rollout | Bumps the agent image on a new talam release per the CR's `upgradePolicy` (`manual` / `auto-patch` / `auto-minor`). |

## What it deliberately does not do

The operator never reads mesh diagnostic data and never calls an LLM. That's not an oversight — see [ADR-0001](/docs/decisions/0001-server-agent-operator-split/) for why the boundary is drawn here: a bug in the operator can't leak mesh topology or trigger an unwanted remediation, because it has no code path to either.

## Owns

- `MeshDiagnostics` CRD definition — see [`../../api/crds.md#meshdiagnostics`](/docs/api/crds/#meshdiagnostics) for the schema and an example manifest.
