---
title: "CRDs"
weight: 10
---
**Related:** reads [`ADR-0005`](/docs/decisions/0005-crd-native-incidents-and-resolutions/), [`ADR-0006`](/docs/decisions/0006-mcp-evidence-enrichment/), [`../components/operator/README.md`](/docs/components/operator/), [`../components/server/README.md`](/docs/components/server/), [`../concepts/remediation-flow.md`](/docs/concepts/remediation-flow/), [`../concepts/finding-and-incident.md`](/docs/concepts/finding-and-incident/); read by [`../components/operator/README.md`](/docs/components/operator/), [`../components/agent/README.md`](/docs/components/agent/), [`../components/server/README.md`](/docs/components/server/), [`../components/mesh-mcp/README.md`](/docs/components/mesh-mcp/), [`../concepts/object-model.md`](/docs/concepts/object-model/)

| Resource | Owner | Purpose |
|---|---|---|
| `MeshDiagnostics` | [Operator](/docs/components/operator/) | Cluster-scoped config: server endpoint, enabled analyzers, scan intervals, upgrade policy. |
| `ModelBinding` | [Server](/docs/components/server/) | Namespaced config: which LLM provider/model backs the [LLM gateway](/docs/components/server/#llm-provider-interface) — the Kubernetes-native alternative to setting the provider via environment variables. |
| `MCPServer` | [Server](/docs/components/server/) | Namespaced config: registers an MCP endpoint the deterministic [evidence-enrichment step](/docs/components/mesh-mcp/) can call — see [ADR-0006](/docs/decisions/0006-mcp-evidence-enrichment/). |
| `MeshFinding` | [Agent](/docs/components/agent/) (status-only mirror) | Local read-only CR mirroring the latest findings for that cluster, so `kubectl get meshfindings` works without hitting talam-server. |
| `MeshIncident` | [Agent](/docs/components/agent/) (synced from server) | The CRD realization of an [`Incident`](/docs/concepts/finding-and-incident/#incident) — see [ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/). |
| `MeshResolution` | [Agent](/docs/components/agent/) (synced from server; reconciled locally) | The CRD realization of a `RemediationProposal` — see [ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/) and [`remediation-flow.md`](/docs/concepts/remediation-flow/). |

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

`analyzers.enabled` / `analyzers.disabled` reference analyzer IDs from [`analyzer-catalog.md`](/docs/api/analyzer-catalog/).

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

If no `ModelBinding` is found (e.g. no Kubernetes access, or running talam-server outside a cluster per [`../components/server/README.md`](/docs/components/server/)), the server falls back to `ANTHROPIC_API_KEY`/`TALAM_LLM_MODEL` environment variables, then to a local `claude` CLI if one is on `PATH`.

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

talam-server polls every `MCPServer` in its own namespace every 30s, connects, and makes it available — keyed by `spec.toolset` — to the deterministic enrichment table in `internal/server/enrich` that runs before every Explain/Propose call. See [ADR-0006](/docs/decisions/0006-mcp-evidence-enrichment/) for why that table decides which tools to call, never the LLM, and [`../components/mesh-mcp/README.md`](/docs/components/mesh-mcp/) for the one MCP server this repo builds and owns (`toolset: mesh`) versus off-the-shelf ones (`metrics`, `kubernetes`) expected to be registered by URL.

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

Both are agent-owned, not created by talam-server directly — see [ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/) for why. `talam-agent`'s Sync loop mirrors talam-server's `Incident`/`RemediationProposal` objects into these every `--sync-interval` (default 15s); `spec.triggered` flips to `true` once a proposal is approved (dashboard or `talamctl`) — never set by talam-server or by any reconciler directly. `ResolutionReconciler` watches for `spec.triggered && !status.performed`, applies the patch the same resourceVersion-gated way as before, and writes `status.phase`/`status.performed`/`status.dryRunDiff`. `IncidentReconciler` watches `MeshIncident` objects and sets `status.complete = true` once every `MeshResolution` referencing it (`spec.incidentRef`) has `status.performed == true` — a `Rejected` resolution does not count, so a rejected-only incident stays visibly incomplete.

`kubectl get meshincidents,meshresolutions -n talam-system` works without hitting talam-server at all.
