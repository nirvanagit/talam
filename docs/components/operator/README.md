# Component: operator

**Related:** reads [`ADR-0001`](../../decisions/0001-server-agent-operator-split.md); links to [`../agent/README.md`](../agent/README.md), [`../../api/crds.md`](../../api/crds.md)

The operator is deliberately dumb. It's a standard Kubernetes controller-runtime reconciliation loop whose only responsibility is making the cluster match a [`MeshDiagnostics`](../../api/crds.md#meshdiagnostics) custom resource ([ADR-0001](../../decisions/0001-server-agent-operator-split.md)).

## Reconciles

| Reconciles | Responsibility |
|---|---|
| [`MeshDiagnostics`](../../api/crds.md#meshdiagnostics) | Top-level CR: which server to report to, scan interval, which analyzers are enabled, resource limits for the agent. |
| Agent Deployment | Creates/updates the [talam-agent](../agent/README.md) Deployment, ServiceAccount, and RBAC (ClusterRole scoped to read-only on core + Istio API groups — see [`../../concepts/security-model.md`](../../concepts/security-model.md)). |
| Agent health | Watches agent readiness; flips `MeshDiagnostics.status.agentHealthy`; restarts on crash-loop beyond a backoff threshold. |
| Upgrade rollout | Bumps the agent image on a new talam release per the CR's `upgradePolicy` (`manual` / `auto-patch` / `auto-minor`). |

## What it deliberately does not do

The operator never reads mesh diagnostic data and never calls an LLM. That's not an oversight — see [ADR-0001](../../decisions/0001-server-agent-operator-split.md) for why the boundary is drawn here: a bug in the operator can't leak mesh topology or trigger an unwanted remediation, because it has no code path to either.

## Owns

- `MeshDiagnostics` CRD definition — see [`../../api/crds.md#meshdiagnostics`](../../api/crds.md#meshdiagnostics) for the schema and an example manifest.
