# ADR-0008: Fleet coordination is Kubernetes-native — agent and server exchange objects, not REST calls

**Status:** Accepted
**Supersedes:** the REST transport in [`ADR-0001`](0001-server-agent-operator-split.md) and the "agent creates/owns MeshIncident/MeshResolution in its own cluster" mechanism in [`ADR-0005`](0005-crd-native-incidents-and-resolutions.md). The trust-boundary *principles* both ADRs establish — server never holds credentials into a spoke cluster it doesn't own, agent is the sole writer of remediation-adjacent state — are unchanged; only the wire format and where objects physically live changes.
**Related:** reads [`ADR-0001`](0001-server-agent-operator-split.md), [`ADR-0005`](0005-crd-native-incidents-and-resolutions.md), [`ADR-0006`](0006-mcp-evidence-enrichment.md), [`ADR-0007`](0007-agent-never-applies-remediation.md), [`../concepts/security-model.md`](../concepts/security-model.md), [`../concepts/object-model.md`](../concepts/object-model.md); affects [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`../components/operator/README.md`](../components/operator/README.md), [`../api/crds.md`](../api/crds.md)

## Context

v0.1 shipped a hand-rolled REST API: the agent posts findings to `POST /v1/findings`, polls `GET /v1/incidents`/`GET /v1/proposals`, and posts outcomes to `POST /v1/proposals/{id}/outcome`. talam-server holds all of this in `Store` — an in-memory map guarded by one mutex, persisted by marshaling the *entire* store to a JSON file and rename-over-write on every single mutation.

Auditing this transport surfaced two real problems, not hypothetical ones:

1. **No authentication.** `handleListIncidents` and `handleReport` accept any request with no identity check at all. Any agent's HTTP client can query `?cluster=` for a cluster it doesn't own. Nothing in the code prevents it.
2. **No transport security.** `docs/concepts/security-model.md` claims mTLS between every agent and talam-server. The actual code calls `http.ListenAndServe` (server) and constructs a plain `http.Client{}` (agent) — no TLS anywhere. The doc was describing an intended future state, not what v0.1 does.

Both gaps have the same root cause: talam built its own auth/transport layer instead of using one that already exists and is well-audited. Kubernetes' own API server already solves exactly this problem — client certs, bearer tokens, per-namespace RBAC, encrypted transport — for every other piece of state in this system. `Store`'s custom persistence has the same character: a single mutex and a full-file rewrite per mutation is a scalability ceiling this project would eventually have had to solve itself, when etcd already solves it.

Separately, `Store.Ingest` does more than fingerprint-dedup: it diffs each report's full finding batch against every currently-`Open` incident for that cluster and auto-resolves anything not present in the latest batch (`internal/server/store.go:124-131`). This "did the problem go away?" logic is real, load-bearing product behavior that any redesign has to preserve, not an implementation detail to drop.

## Decision

Findings, incidents, and remediation state move entirely onto Kubernetes objects, exchanged via cross-cluster watches instead of a bespoke REST API. talam-server's own cluster becomes the meeting point.

### Namespace-per-cluster

The cluster talam-server runs in gets one namespace per registered spoke cluster — `fleet-<clusterName>`, pre-created out of band when a cluster joins the fleet (see **Credential bootstrap** below). Every object described in this ADR that originates from a given spoke cluster lives in that spoke's dedicated namespace. This is the RBAC mechanism, not a convention layered on top of one: a spoke cluster's agent gets a `Role`/`RoleBinding` scoped to exactly its own `fleet-<clusterName>` namespace and nothing else, so one compromised or misconfigured agent structurally cannot see or write another cluster's data — Kubernetes enforces the isolation the current `?cluster=` query parameter only *asks nicely* for today.

### The object model

Four object types, four distinct owners, each living in exactly one place:

**`MeshFinding`** — new, agent-owned, lives in the server cluster's `fleet-<clusterName>` namespace. One object per `Finding.Fingerprint()`. Every scan tick, the agent reconciles the *exact set* that should exist right now: create on first detection, update on re-detection (refreshed evidence, timestamp), **delete** when the analyzer that raised it no longer does. This replaces `Store.Ingest`'s full-batch-diff-against-open-incidents with ordinary Kubernetes object lifecycle — the same "did this go away?" signal, computed locally per cluster by the agent instead of centrally by the server comparing batches. This is the concrete mechanism behind the whole redesign being more scalable: that diff work is now sharded across N agents instead of serialized behind one server's mutex.

This was already sketched, unbuilt, in [`object-model.md`](../concepts/object-model.md) as a local-only visibility mirror. It graduates here to the actual agent→server transport, which subsumes the original motivation (`kubectl get meshfindings` works without hitting talam-server) for free — the object already lives somewhere the agent can watch and reason about locally, it just also happens to be the transport now.

**`MeshIncident`** — existing type, ownership reverses: **now server-owned**, created and updated entirely by talam-server's own controller watching `MeshFinding` objects fleet-wide. Fingerprint-based correlation collapses to near-1:1 with `MeshFinding` today (`findByFingerprintLocked` never did more than exact-match), but the two stay separate objects rather than folding `MeshIncident` into `MeshFinding` — same single-writer principle every other object in this system follows: `MeshFinding` is purely agent-written deterministic state, `MeshIncident` is purely server-written LLM-derived state, and keeping them apart leaves room for smarter multi-finding correlation later without touching the agent's contract. When a `MeshFinding` disappears, the server's controller marks the corresponding `MeshIncident.status.state = Resolved` — the direct replacement for `Store.Ingest`'s auto-resolve branch.

**`MeshResolution`** — existing type, now has **two homes**. The canonical copy is server-owned, created in the server cluster's `fleet-<clusterName>` namespace from the LLM's `Propose` call, same shape as before ([ADR-0007](0007-agent-never-applies-remediation.md) still applies in full: `spec.approved` is still advisory, talam still never applies it). The agent additionally maintains a **read-only mirror** of it in the spoke cluster itself — watching the canonical copy in the server cluster and reflecting it down, exactly the mechanism `CRDSync` already implements today, just pointed at a Kubernetes watch instead of a `GET` against a REST endpoint. This mirror is what the external system in the spoke cluster actually subscribes to.

**`MeshResolutionCompleted`** — new, and the answer to a deliberate RBAC-hygiene choice: the external system that applies a remediation never gets write access to `MeshResolution.status` (a talam-owned object) in either cluster. Instead, once it's applied the patch, it **creates** a `MeshResolutionCompleted` object *locally*, in the spoke cluster, referencing the resolution by name — a create-only permission, nothing to patch, nothing it can corrupt on an object it doesn't own. The agent watches for these locally and relays a corresponding `MeshResolutionCompleted` into the server cluster's `fleet-<clusterName>` namespace — the same relay pattern `ResolutionReconciler` already implements for outcome reporting in ADR-0007, just via CRD mirroring instead of an HTTP POST.

### Purge

Once the server sees `MeshResolutionCompleted` in its own cluster, it deletes the `MeshFinding` (if the same fingerprint hasn't reappeared), `MeshIncident`, and `MeshResolution` it owns for that fingerprint — the fleet's history store shrinks itself back down instead of accumulating terminal state forever. The agent watches for the same `MeshResolutionCompleted` (both its local copy and, transitively, the fact its relay succeeded) so it knows the fingerprint's remediation cycle has fully closed and a fresh detection is free to open a new one cleanly, rather than colliding with a stale, about-to-be-purged object.

### Credential bootstrap

The agent needs a second credential now — one scoped to its `fleet-<clusterName>` namespace in the server's cluster, in addition to its existing local one. This is provisioned **out of band, once, per cluster joining the fleet**: whoever administers the server cluster pre-creates the namespace plus a `ServiceAccount`/`Role`/`RoleBinding` scoped to exactly that namespace (create/update/delete/get/list/watch on `meshfindings`; get/list/watch on `meshresolutions`; get/create on `meshresolutioncompleted`), mints a token, and hands the resulting kubeconfig to whoever is standing up that spoke cluster — the same one-time, out-of-band step admins already do for `--server` today, just kubeconfig-shaped instead of URL-shaped.

**talam-operator's role stays deliberately thin here**: it does not mint or manage this credential itself. `MeshDiagnostics` gains `spec.fleetKubeconfigSecretRef`, naming a `Secret` (already sitting in the spoke cluster, placed there during that same one-time bootstrap step) that the operator mounts into the agent's Deployment alongside its existing local `ServiceAccount` token. The operator's job is exactly what it already does — plumb a credential into the agent's pod spec — not a new capability to reach into a remote cluster itself. Fleet self-registration (a spoke cluster requesting to join and getting a namespace auto-provisioned, CSR-style) is worth building eventually but is explicitly **not** part of this ADR — the manual, out-of-band bootstrap above is the v1 mechanism.

The external system in a spoke cluster needs **no cross-cluster credential at all** — it only ever touches the local `MeshResolution` mirror and creates a local `MeshResolutionCompleted`, both ordinary same-cluster RBAC. The agent remains the *only* thing in a spoke cluster that ever holds a credential into the server's cluster, in either direction — the same "one component, one crossing point" shape ADR-0001 already established, just with two flows (`MeshFinding`/`MeshResolutionCompleted` up, `MeshResolution` down) through that single point instead of one.

### talam-mesh-mcp stays exactly where it is

This ADR does not move evidence enrichment. It's still the server, right before an `Explain` call, that reaches into the relevant spoke cluster's `talam-mesh-mcp` endpoint ([ADR-0006](0006-mcp-evidence-enrichment.md), unchanged) — not the agent. The reasoning holds independently of this ADR's transport change: enrichment is lazy (fetched only for findings that actually escalate to an LLM call), and only the server knows, at explain-time, which findings those are. Moving the call to the agent would mean either over-fetching on every `MeshFinding` write (most of which are repeats that never reach an LLM) or reintroducing a push-style "please enrich this one" signal into the spoke cluster — exactly the kind of coordination this whole ADR is built to avoid. The one remaining place talam-server reaches into a spoke cluster directly stays scoped to exactly that, and no wider.

## Consequences

**Real wins, not just symmetry with the rest of the object model:**
- Authentication and transport security come from the Kubernetes API server's own auth, for free — closes both gaps named in Context without talam building or maintaining any of it itself.
- `Store`'s single-mutex, full-file-rewrite persistence is gone, replaced by etcd — a strict capacity upgrade, not a lateral move.
- Per-namespace RBAC is a structurally enforced tenant boundary, not a server-side authorization check that has to be remembered and gotten right on every handler (the current `?cluster=` gap only exists because nothing enforces it).
- The "did this problem go away" diff is sharded per-cluster instead of centralized, which is the concrete scalability improvement motivating this whole redesign.

**Real costs, worth naming plainly:**
- Every spoke cluster now needs a one-time, out-of-band bootstrap step (namespace + scoped ServiceAccount + kubeconfig secret) before it can join the fleet — more setup than "point `--server` at a URL." Fleet self-registration would remove this but is explicitly out of scope here.
- The agent needs network reachability to the server cluster's API server (typically port 6443), not just an HTTPS endpoint behind whatever ingress a REST API could sit behind. Depending on network topology (restrictive firewalls, air-gapped edges) this may be a harder path to open than the REST model needed.
- talam-server's own `ServiceAccount` needs a `ClusterRole` (watch `MeshFinding`/`MeshResolutionCompleted` fleet-wide, write `MeshIncident`/`MeshResolution` into any `fleet-*` namespace) — broader than anything it held before, though still scoped entirely to its own cluster, not a new cross-cluster grant.
- `MeshResolution` having two homes (canonical + mirror) reintroduces exactly the kind of dual-copy state ADR-0005 was written to avoid duplicating for `MeshIncident`/`MeshResolution` in the old model — accepted here because the mirror is genuinely necessary (the external system in a spoke cluster can't reasonably hold a cross-cluster credential of its own, per **Credential bootstrap** above), but worth remembering as the one asymmetry in an otherwise single-source-of-truth-per-object model.

**Not done in this ADR:** actual implementation. This replaces `internal/server/store.go` and `internal/server/http.go` (the REST handlers) with a controller-runtime-style watch loop in talam-server, extends `CRDSync`/`ResolutionReconciler` in the agent to talk to two clusters instead of one, adds the `MeshFinding`/`MeshResolutionCompleted` CRDs and their RBAC manifests, and adds `spec.fleetKubeconfigSecretRef` handling to the operator. That's a substantial, multi-package change — scoped as follow-on implementation work, not part of writing this decision down.
