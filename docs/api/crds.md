# CRDs

**Related:** reads [`../components/operator/README.md`](../components/operator/README.md), [`../components/server/README.md`](../components/server/README.md), [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md); read by [`../components/operator/README.md`](../components/operator/README.md), [`../components/server/README.md`](../components/server/README.md)

| Resource | Owner | Purpose |
|---|---|---|
| `MeshDiagnostics` | [Operator](../components/operator/README.md) | Cluster-scoped config: server endpoint, enabled analyzers, scan intervals, upgrade policy. |
| `ModelBinding` | [Server](../components/server/README.md) | Namespaced config: which LLM provider/model backs the [LLM gateway](../components/server/README.md#llm-provider-interface) — the Kubernetes-native alternative to setting the provider via environment variables. |
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

## ModelBinding

```yaml
apiVersion: talam.dev/v1alpha1
kind: ModelBinding
metadata:
  name: default
  namespace: talam-system
spec:
  provider: anthropic       # anthropic | openai-compatible | claude-cli
  model: claude-sonnet-5
  apiKeySecretRef:
    name: talam-llm-credentials
    key: apiKey
```

talam-server polls the `ModelBinding` named in its `--model-binding-name` flag (default `default`) in its own namespace every 30s and rebuilds its provider on change — editing `spec.model` or `spec.provider` live-switches the model the fleet uses, no server restart required. `provider: openai-compatible` additionally requires `spec.baseURL`, for pointing talam at a self-hosted model on air-gapped clusters. `provider: claude-cli` shells out to a local `claude` binary in talam-server's own environment and is a local-dev convenience, not a cluster deployment target (implemented in `internal/server/llm`).

If no `ModelBinding` is found (e.g. no Kubernetes access, or running talam-server outside a cluster per [`../components/server/README.md`](../components/server/README.md)), the server falls back to `ANTHROPIC_API_KEY`/`TALAM_LLM_MODEL` environment variables, then to a local `claude` CLI if one is on `PATH`.
