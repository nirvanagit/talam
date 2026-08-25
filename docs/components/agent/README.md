# Component: agent

**Related:** reads [`ADR-0001`](../../decisions/0001-server-agent-operator-split.md), [`ADR-0005`](../../decisions/0005-crd-native-incidents-and-resolutions.md), [`../../concepts/analyzer-interface.md`](../../concepts/analyzer-interface.md), [`../../concepts/finding-and-incident.md`](../../concepts/finding-and-incident.md), [`../../concepts/remediation-flow.md`](../../concepts/remediation-flow.md); links to [`../operator/README.md`](../operator/README.md), [`../server/README.md`](../server/README.md), [`../../api/analyzer-catalog.md`](../../api/analyzer-catalog.md), [`../../api/crds.md`](../../api/crds.md#meshincident--meshresolution)

One agent per cluster (not per namespace) keeps the footprint low while still giving each mesh boundary its own credential and blast radius ([`../../concepts/security-model.md`](../../concepts/security-model.md)). The agent runs the analyzer engine locally and ships structured [Findings](../../concepts/finding-and-incident.md), not raw dumps — that keeps the wire format small and keeps raw cluster secrets from ever leaving the cluster.

## Subsystems

| Subsystem | Job |
|---|---|
| Collectors | Kube API watcher (Istio CRDs: VirtualService, DestinationRule, Gateway, PeerAuthentication, AuthorizationPolicy, ServiceEntry, Sidecar), istiod debug/xDS endpoints, Envoy admin API sampling, Prometheus/telemetry query client. |
| Analyzer engine | Runs the registered [`Analyzer`](../../concepts/analyzer-interface.md) set — see [`../../api/analyzer-catalog.md`](../../api/analyzer-catalog.md) for the concrete v0.1 set — against collected state on each scan tick or watch-triggered event; produces `Finding` objects. |
| Local cache | Short-lived in-memory snapshot of mesh state for correlation (e.g. matching a `DestinationRule` subset to live pod labels) — no persistent DB in the agent. |
| Reporter | Batches findings and pushes to [talam-server](../server/README.md) over mTLS gRPC; buffers and retries on server unavailability; never blocks scanning. |
| CRD Sync | Pulls this cluster's `Incident`/`RemediationProposal` objects from talam-server and mirrors them into local [`MeshIncident`/`MeshResolution`](../../api/crds.md#meshincident--meshresolution) CRs — the only place server state enters the cluster, and it's a pull, never a push ([ADR-0005](../../decisions/0005-crd-native-incidents-and-resolutions.md)). |
| Resolution reconciler | Watches local `MeshResolution` objects for `spec.triggered && !status.performed` and is what actually applies — dry run, resourceVersion-gated patch, then status + outcome report — never the Reporter or CRD Sync. |
| Incident reconciler | Watches local `MeshIncident` objects and rolls up `status.complete` from the `MeshResolution` objects referencing each one, entirely locally, no server round-trip needed. |

## Scan cadence

Configurable per analyzer via `Trigger()` — cheap checks like "subset has zero endpoints" run on watch events (near-real-time); expensive checks like full xDS-config-vs-intent diffing run on a longer interval (default 5 min).

## Deployed by

The agent doesn't deploy itself — see [`../operator/README.md`](../operator/README.md) for how it's installed, configured, and kept healthy.
