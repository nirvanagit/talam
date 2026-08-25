# CRDs

**Related:** reads [`../components/operator/README.md`](../components/operator/README.md), [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md); read by [`../components/operator/README.md`](../components/operator/README.md)

| Resource | Owner | Purpose |
|---|---|---|
| `MeshDiagnostics` | [Operator](../components/operator/README.md) | Cluster-scoped config: server endpoint, enabled analyzers, scan intervals, upgrade policy. |
| `MeshFinding` | [Agent](../components/agent/README.md) (status-only mirror) | Local read-only CR mirroring the latest findings for that cluster, so `kubectl get meshfindings` works without hitting talam-server. |
| `RemediationProposal` | [Server](../components/server/README.md) (via API), applied by [Agent](../components/agent/README.md) | Not a CR by default — lives in talam-server's store; exposed via API/CLI. Promoted to a CR only if an org wants GitOps-style approval via PR instead of CLI/UI — see [`remediation-flow.md`](../concepts/remediation-flow.md). |

## MeshDiagnostics

```yaml
apiVersion: talam.dev/v1alpha1
kind: MeshDiagnostics
metadata:
  name: default
spec:
  serverEndpoint: talam-server.platform.svc:8443
  meshBackend: istio
  scanInterval: 5m
  analyzers:
    enabled: ["*"]
    disabled: ["serviceentry.unused"]
  upgradePolicy: auto-patch
```

`analyzers.enabled` / `analyzers.disabled` reference analyzer IDs from [`analyzer-catalog.md`](analyzer-catalog.md).
