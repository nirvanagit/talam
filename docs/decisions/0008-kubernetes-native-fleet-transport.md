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

### The object model — zero new CRD types

`MeshIncident` and `MeshResolution` are the only object types involved. Both already exist; this ADR changes who owns them and where they live, not their kind count. Two earlier drafts of this ADR introduced `MeshFinding` and `MeshResolutionCompleted` as new types — both folded back into the existing two once a same-object mechanism was found that preserves the exact behavior without adding a type. That history is worth keeping here since the reasoning generalizes: prefer a spec/status split or a keyed list on an existing object over a new object type, and only reach for a new type when neither can express the ownership boundary you need.

**`MeshIncident`** — lives in the server cluster's `fleet-<clusterName>` namespace, named after the finding's fingerprint. Spec and status are owned by different writers, same convention every other object in this system already follows:

```yaml
spec:                          # agent-owned
  resource: {...}              # from Finding.Resource
  analyzerId: string
  rawEvidence: {...}
  detected: bool                # true while the analyzer still flags this fingerprint
  firstSeen: timestamp
  lastDetectedAt: timestamp     # refreshed by the agent on every re-detection
status:                        # server-owned
  state: Open | Resolved
  explanation: string
  explainError: string
  resolutionRef: string         # name of the MeshResolution, once proposed
```

Every scan tick, the agent reconciles this object directly instead of posting a batch for the server to diff: create it on first detection, refresh `spec.lastDetectedAt` on re-detection, and **flip `spec.detected` to `false`** on its last write once the analyzer stops flagging it — never delete it itself (deletion is a purge-time decision, see below, and happens only after the whole remediation cycle closes). This is the direct replacement for `Store.Ingest`'s full-batch-diff-against-open-incidents (`internal/server/store.go:124-131`): the same "did this go away?" signal, computed locally per cluster by the agent instead of centrally by the server comparing batches — the concrete mechanism behind this redesign being more scalable, since that diffing work is now sharded across N agents instead of serialized behind one server's mutex.

The server watches for spec changes on `MeshIncident` fleet-wide: on create, or on `spec.detected` going `true` again after `status.state` was `Resolved` (a flaky/intermittent problem reopening — a real, distinct case `Store.Ingest` already handles today and this preserves), it runs `Explain` (then `Propose` if warranted). When it observes `spec.detected == false` while `status.state == Open`, it sets `status.state = Resolved` — the direct replacement for `Store.Ingest`'s auto-resolve branch.

**`MeshResolution`** — still has **two homes**, which remains the one deliberate exception to single-source-of-truth in this design (see Consequences). The canonical copy is server-owned, created in the server cluster's `fleet-<clusterName>` namespace from the LLM's `Propose` call ([ADR-0007](0007-agent-never-applies-remediation.md) applies in full: `spec.approved` is still advisory, talam still never applies it). The agent mirrors a **read-only copy** down into the spoke cluster — the same mechanism `CRDSync` already implements today, just pointed at a Kubernetes watch instead of a REST `GET`. The external system in the spoke cluster subscribes to this mirror, never the canonical copy directly (it has no cross-cluster credential — see **Credential bootstrap**).

Completion uses a Kubernetes-native pattern instead of a new object: [Pod readiness gates](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate).

```yaml
spec:
  ...                            # target, patch, riskTier, approved — unchanged from ADR-0007
  readinessGates: ["Applied"]    # documents intent; server sets this when it creates the resolution
status:
  phase: string                  # mirrored from server-side ProposalState until a condition lands
  outcomeReported: bool          # talam-only bookkeeping — agent-owned, external system never touches this
  conditions:                    # external system upserts ITS OWN entry, keyed by type
    - type: Applied
      status: "True" | "False"
      reason: string              # identity of the applying system, e.g. "argocd"
      message: string             # free-form detail
      lastTransitionTime: timestamp
```

The external system, after acting, patches `status.conditions` on the **local mirror** to upsert one entry with `type: Applied` — the same upsert-by-type pattern `controller-runtime`'s `meta.SetStatusCondition` already implements, so there's real tooling behind this, not a bespoke convention. The agent watches the local mirror for that condition appearing and relays it onto the canonical copy in the server cluster (same relay role `ResolutionReconciler` already plays under ADR-0007, now via CRD mirroring instead of an HTTP POST), then sets `status.outcomeReported = true` once the relay lands.

**Honest trade-off, not a hedge:** Kubernetes RBAC can't scope a grant to "only the `Applied` entry in `conditions`" — a `patch` grant on `meshresolutions/status` is all-or-nothing at the subresource level, so a careless external system could in principle also touch `outcomeReported`. This is the identical trust model core Kubernetes accepts for Pod conditions (kubelet and external readiness-gate controllers share one status-write grant), so it's a well-worn trade-off, not a novel gap — but it is weaker than the create-only-on-a-separate-object approach an earlier draft of this ADR used, which had a structural (not conventional) guarantee. Revisit if an external integration turns out not to respect the convention.

### Purge

Once the server observes the `Applied` condition on a `MeshResolution` it owns, it deletes that `MeshResolution` and the corresponding `MeshIncident` — the fleet's history store shrinks itself back down instead of accumulating terminal state forever. The agent, watching the same condition (both on its local mirror and transitively via its own successful relay), knows the fingerprint's remediation cycle has fully closed and is free to let `spec.detected` flip back to `true` and open a clean new cycle on the next re-detection, rather than colliding with a resolution about to be purged.

### Credential bootstrap

The agent needs a second credential now — one scoped to its `fleet-<clusterName>` namespace in the server's cluster, in addition to its existing local one. This is provisioned **out of band, once, per cluster joining the fleet**: whoever administers the server cluster pre-creates the namespace plus a `ServiceAccount`/`Role`/`RoleBinding` scoped to exactly that namespace (create/update/get/list/watch on `meshincidents` — no delete, only the server purges; get/list/watch on `meshresolutions`, plus `update`/`patch` on `meshresolutions/status` so the agent can relay the `Applied` condition onto the canonical copy), mints a token, and hands the resulting kubeconfig to whoever is standing up that spoke cluster — the same one-time, out-of-band step admins already do for `--server` today, just kubeconfig-shaped instead of URL-shaped.

**talam-operator's role stays deliberately thin here**: it does not mint or manage this credential itself. `MeshDiagnostics` gains `spec.fleetKubeconfigSecretRef`, naming a `Secret` (already sitting in the spoke cluster, placed there during that same one-time bootstrap step) that the operator mounts into the agent's Deployment alongside its existing local `ServiceAccount` token. The operator's job is exactly what it already does — plumb a credential into the agent's pod spec — not a new capability to reach into a remote cluster itself. Fleet self-registration (a spoke cluster requesting to join and getting a namespace auto-provisioned, CSR-style) is worth building eventually but is explicitly **not** part of this ADR — the manual, out-of-band bootstrap above is the v1 mechanism.

The external system in a spoke cluster needs **no cross-cluster credential at all** — it only ever patches `status.conditions` on the local `MeshResolution` mirror, ordinary same-cluster RBAC. The agent remains the *only* thing in a spoke cluster that ever holds a credential into the server's cluster, in either direction — the same "one component, one crossing point" shape ADR-0001 already established, just with two flows (`MeshIncident` up, `MeshResolution` down plus the relayed `Applied` condition back up) through that single point instead of one.

### talam-mesh-mcp stays exactly where it is

This ADR does not move evidence enrichment. It's still the server, right before an `Explain` call, that reaches into the relevant spoke cluster's `talam-mesh-mcp` endpoint ([ADR-0006](0006-mcp-evidence-enrichment.md), unchanged) — not the agent. The reasoning holds independently of this ADR's transport change: enrichment is lazy (fetched only for findings that actually escalate to an LLM call), and only the server knows, at explain-time, which findings those are. Moving the call to the agent would mean either over-fetching on every `MeshIncident` write (most re-detections are repeats that never trigger a fresh `Explain`) or reintroducing a push-style "please enrich this one" signal into the spoke cluster — exactly the kind of coordination this whole ADR is built to avoid. The one remaining place talam-server reaches into a spoke cluster directly stays scoped to exactly that, and no wider.

## Consequences

**Real wins, not just symmetry with the rest of the object model:**
- Authentication and transport security come from the Kubernetes API server's own auth, for free — closes both gaps named in Context without talam building or maintaining any of it itself.
- `Store`'s single-mutex, full-file-rewrite persistence is gone, replaced by etcd — a strict capacity upgrade, not a lateral move.
- Per-namespace RBAC is a structurally enforced tenant boundary, not a server-side authorization check that has to be remembered and gotten right on every handler (the current `?cluster=` gap only exists because nothing enforces it).
- The "did this problem go away" diff is sharded per-cluster instead of centralized, which is the concrete scalability improvement motivating this whole redesign.

- No new CRD types — `MeshFinding` and `MeshResolutionCompleted` were both considered and folded back into `MeshIncident`/`MeshResolution` via a spec/status split and a readinessGate-style condition, respectively (see **The object model**). Fewer schemas to version, fewer RBAC objects to provision, fewer watch loops to run.

**Real costs, worth naming plainly:**
- Every spoke cluster now needs a one-time, out-of-band bootstrap step (namespace + scoped ServiceAccount + kubeconfig secret) before it can join the fleet — more setup than "point `--server` at a URL." Fleet self-registration would remove this but is explicitly out of scope here.
- The agent needs network reachability to the server cluster's API server (typically port 6443), not just an HTTPS endpoint behind whatever ingress a REST API could sit behind. Depending on network topology (restrictive firewalls, air-gapped edges) this may be a harder path to open than the REST model needed.
- talam-server's own `ServiceAccount` needs a `ClusterRole` (watch `MeshIncident` fleet-wide, write `MeshIncident`/`MeshResolution` into any `fleet-*` namespace) — broader than anything it held before, though still scoped entirely to its own cluster, not a new cross-cluster grant.
- `MeshResolution` having two homes (canonical + mirror) reintroduces exactly the kind of dual-copy state ADR-0005 was written to avoid duplicating for `MeshIncident`/`MeshResolution` in the old model — accepted here because the mirror is genuinely necessary (the external system in a spoke cluster can't reasonably hold a cross-cluster credential of its own, per **Credential bootstrap** above), but worth remembering as the one asymmetry in an otherwise single-source-of-truth-per-object model.
- The conditions-based completion signal trades a structural RBAC guarantee (a separate, create-only object type literally cannot touch `MeshResolution.status`) for a conventional one (an all-or-nothing `status` patch grant, same trust model Kubernetes itself uses for Pod readiness gates). Named explicitly in **The object model** above — revisit if an external integration doesn't respect the convention.

**Not done in this ADR:** actual implementation. This replaces `internal/server/store.go` and `internal/server/http.go` (the REST handlers) with a controller-runtime-style watch loop in talam-server, extends `CRDSync`/`ResolutionReconciler` in the agent to talk to two clusters instead of one, updates the `MeshIncident`/`MeshResolution` CRD schemas and RBAC manifests, and adds `spec.fleetKubeconfigSecretRef` handling to the operator. That's a substantial, multi-package change — scoped as follow-on implementation work, not part of writing this decision down.
