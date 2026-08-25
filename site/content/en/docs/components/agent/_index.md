---
title: "Component: agent"
weight: 20
---
**Related:** reads [`ADR-0001`](/docs/decisions/0001-server-agent-operator-split/), [`ADR-0005`](/docs/decisions/0005-crd-native-incidents-and-resolutions/), [`../../concepts/analyzer-interface.md`](/docs/concepts/analyzer-interface/), [`../../concepts/finding-and-incident.md`](/docs/concepts/finding-and-incident/), [`../../concepts/remediation-flow.md`](/docs/concepts/remediation-flow/); links to [`../operator/README.md`](/docs/components/operator/), [`../server/README.md`](/docs/components/server/), [`../../api/analyzer-catalog.md`](/docs/api/analyzer-catalog/), [`../../api/crds.md`](/docs/api/crds/#meshincident--meshresolution), [`../../concepts/object-model.md`](/docs/concepts/object-model/)

One agent per cluster (not per namespace) keeps the footprint low while still giving each mesh boundary its own credential and blast radius ([`../../concepts/security-model.md`](/docs/concepts/security-model/)). The agent runs the analyzer engine locally and ships structured [Findings](/docs/concepts/finding-and-incident/), not raw dumps — that keeps the wire format small and keeps raw cluster secrets from ever leaving the cluster.

## Subsystems

| Subsystem | Job |
|---|---|
| Collectors | Kube API watcher (Istio CRDs: VirtualService, DestinationRule, Gateway, PeerAuthentication, AuthorizationPolicy, ServiceEntry, Sidecar), istiod debug/xDS endpoints, Envoy admin API sampling, Prometheus/telemetry query client. |
| Analyzer engine | Runs the registered [`Analyzer`](/docs/concepts/analyzer-interface/) set — see [`../../api/analyzer-catalog.md`](/docs/api/analyzer-catalog/) for the concrete v0.1 set — against collected state on each scan tick or watch-triggered event; produces `Finding` objects. |
| Local cache | Short-lived in-memory snapshot of mesh state for correlation (e.g. matching a `DestinationRule` subset to live pod labels) — no persistent DB in the agent. |
| Reporter | Batches findings and pushes to [talam-server](/docs/components/server/) over mTLS gRPC; buffers and retries on server unavailability; never blocks scanning. |
| CRD Sync | Pulls this cluster's `Incident`/`RemediationProposal` objects from talam-server and mirrors them into local [`MeshIncident`/`MeshResolution`](/docs/api/crds/#meshincident--meshresolution) CRs — the only place server state enters the cluster, and it's a pull, never a push ([ADR-0005](/docs/decisions/0005-crd-native-incidents-and-resolutions/)). |
| Resolution reconciler | Watches local `MeshResolution` objects for `spec.triggered && !status.performed` and is what actually applies — dry run, resourceVersion-gated patch, then status + outcome report — never the Reporter or CRD Sync. |
| Incident reconciler | Watches local `MeshIncident` objects and rolls up `status.complete` from the `MeshResolution` objects referencing each one, entirely locally, no server round-trip needed. |

## Scan cadence

Configurable per analyzer via `Trigger()` — cheap checks like "subset has zero endpoints" run on watch events (near-real-time); expensive checks like full xDS-config-vs-intent diffing run on a longer interval (default 5 min).

## Deployed by

The agent doesn't deploy itself — see [`../operator/README.md`](/docs/components/operator/) for how it's installed, configured, and kept healthy.
