# CRDs

**Related:** reads [`ADR-0005`](../decisions/0005-crd-native-incidents-and-resolutions.md), [`ADR-0006`](../decisions/0006-mcp-evidence-enrichment.md), [`../components/operator/README.md`](../components/operator/README.md), [`../components/server/README.md`](../components/server/README.md), [`../concepts/remediation-flow.md`](../concepts/remediation-flow.md), [`../concepts/finding-and-incident.md`](../concepts/finding-and-incident.md); read by [`../components/operator/README.md`](../components/operator/README.md), [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`../components/mesh-mcp/README.md`](../components/mesh-mcp/README.md), [`../concepts/object-model.md`](../concepts/object-model.md)

| Resource | Owner | Purpose |
|---|---|---|
| `MeshDiagnostics` | [Operator](../components/operator/README.md) | Cluster-scoped config: server endpoint, enabled analyzers, scan intervals, upgrade policy. |
| `ModelBinding` | [Server](../components/server/README.md) | Namespaced config: which LLM provider/model backs the [LLM gateway](../components/server/README.md#llm-provider-interface) — the Kubernetes-native alternative to setting the provider via environment variables. |
| `MCPServer` | [Server](../components/server/README.md) | Namespaced config: registers an MCP endpoint the deterministic [evidence-enrichment step](../components/mesh-mcp/README.md) can call — see [ADR-0006](../decisions/0006-mcp-evidence-enrichment.md). |
| `MeshFinding` | [Agent](../components/agent/README.md) (status-only mirror) | Local read-only CR mirroring the latest findings for that cluster, so `kubectl get meshfindings` works without hitting talam-server. |
| `MeshIncident` | [Agent](../components/agent/README.md) (synced from server) | The CRD realization of an [`Incident`](../concepts/finding-and-incident.md#incident) — see [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md). |
| `MeshResolution` | [Agent](../components/agent/README.md) (synced from server; reconciled locally) | The CRD realization of a `RemediationProposal` — see [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md) and [`remediation-flow.md`](../concepts/remediation-flow.md). |

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

## MCPServer

```yaml
apiVersion: talam.dev/v1alpha1
kind: MCPServer
metadata:
  name: mesh
  namespace: talam-system
spec:
  toolset: mesh   # metrics | kubernetes | mesh | custom
  endpoint: http://talam-mesh-mcp.talam-system.svc:9090
  authSecretRef:      # optional
    name: mesh-mcp-credentials
    key: token
```

talam-server polls every `MCPServer` in its own namespace every 30s, connects, and makes it available — keyed by `spec.toolset` — to the deterministic enrichment table in `internal/server/enrich` that runs before every Explain/Propose call. See [ADR-0006](../decisions/0006-mcp-evidence-enrichment.md) for why that table decides which tools to call, never the LLM, and [`../components/mesh-mcp/README.md`](../components/mesh-mcp/README.md) for the one MCP server this repo builds and owns (`toolset: mesh`) versus off-the-shelf ones (`metrics`, `kubernetes`) expected to be registered by URL.

## MeshIncident / MeshResolution

```yaml
apiVersion: talam.dev/v1alpha1
kind: MeshResolution
metadata:
  name: prop-1787644387-3
  namespace: talam-system
spec:
  incidentRef: inc-1787644360-2
  serverProposalId: prop-1787644387-3
  target: {kind: DestinationRule, namespace: demo, name: httpbin}
  targetResourceVersion: "3627"
  riskTier: Medium
  patch: [{op: remove, path: /spec/subsets/1}]
  triggered: false   # the one field a human sets — see below
status:
  phase: Pending
  performed: false
```

Both are agent-owned, not created by talam-server directly — see [ADR-0005](../decisions/0005-crd-native-incidents-and-resolutions.md) for why. `talam-agent`'s Sync loop mirrors talam-server's `Incident`/`RemediationProposal` objects into these every `--sync-interval` (default 15s); `spec.triggered` flips to `true` once a proposal is approved (dashboard or `talamctl`) — never set by talam-server or by any reconciler directly. `ResolutionReconciler` watches for `spec.triggered && !status.performed`, applies the patch the same resourceVersion-gated way as before, and writes `status.phase`/`status.performed`/`status.dryRunDiff`. `IncidentReconciler` watches `MeshIncident` objects and sets `status.complete = true` once every `MeshResolution` referencing it (`spec.incidentRef`) has `status.performed == true` — a `Rejected` resolution does not count, so a rejected-only incident stays visibly incomplete.

`kubectl get meshincidents,meshresolutions -n talam-system` works without hitting talam-server at all.
