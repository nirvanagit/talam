# ADR-0009: Evidence gathering moves to the agent; the server's job becomes fleet-wide correlation

**Status:** Accepted
**Supersedes:** the server-side placement of MCP evidence enrichment in [`ADR-0006`](0006-mcp-evidence-enrichment.md) and the "talam-mesh-mcp stays exactly where it is" section of [`ADR-0008`](0008-kubernetes-native-fleet-transport.md). ADR-0006's *deterministic, never agentic* principle is unchanged — only which process does the fetching moves.
**Related:** reads [`ADR-0002`](0002-deterministic-analyzers-then-llm.md), [`ADR-0006`](0006-mcp-evidence-enrichment.md), [`ADR-0008`](0008-kubernetes-native-fleet-transport.md), [`../concepts/analyzer-interface.md`](../concepts/analyzer-interface.md); affects [`../components/agent/README.md`](../components/agent/README.md), [`../components/server/README.md`](../components/server/README.md), [`../components/mesh-mcp/README.md`](../components/mesh-mcp/README.md), [`../api/crds.md`](../api/crds.md)

## Context

ADR-0006 put MCP evidence enrichment on talam-server: right before an `Explain` call, the server reaches into the relevant spoke cluster's `talam-mesh-mcp` endpoint to fetch extra context. ADR-0008 reaffirmed this placement, arguing the timing (enrich only findings that reach an `Explain` call) required knowledge only the server had.

That argument held under the *old* incident model, where the server learned about a finding for the first time when a REST batch landed. It doesn't hold anymore: under [ADR-0008](0008-kubernetes-native-fleet-transport.md)'s merged `MeshIncident` object, the agent already tracks `spec.detected`/`firstSeen` locally — the exact "is this new or reopening?" signal that used to be server-only knowledge. The agent can apply the same enrich-only-when-it-matters heuristic itself, with no new coordination needed.

Once evidence-gathering is local, the server's remaining job past `Explain`/`Propose` gets more interesting than single-cluster fingerprint matching: with `MeshIncident` objects for every spoke cluster already visible fleet-wide in one place (ADR-0008's namespace-per-cluster model), the server is naturally positioned to correlate an incident against *other* incidents across the fleet, and against external context — recent GitHub commits/deploys, ArgoCD sync state — that no single agent could see on its own.

## Decision

### Evidence gathering moves to the agent

`MCPServer` relocates: it becomes a namespaced object **in the spoke cluster**, read by the agent, not the server. The agent connects directly to whatever local MCP endpoints it registers (`talam-mesh-mcp` foremost among them) — same-cluster traffic, no cross-cluster reachability required at all. This removes the last cross-cluster network exception ADR-0008 left in place; after this ADR, talam-server never reaches into a spoke cluster directly, for anything.

Evidence enrichment stops being a separate table matching `AnalyzerID` to a fixed set of tool calls (ADR-0006's central mechanism, only necessary because the decision-maker and the analyzer used to live in different processes). Since both now run in the agent, an analyzer can simply call the local MCP client itself, as part of producing a `Finding` — the same way it already queries its local collector for K8s/Istio state. No new laziness mechanism is needed either: `Trigger()` (`docs/concepts/analyzer-interface.md`) already gates which analyzers run how often; MCP calls just become part of what a triggered analyzer may do while investigating, at whatever cost that analyzer's own trigger policy already accepts.

**Still deterministic, still never agentic** — this is the one principle ADR-0006 established that survives unchanged. An analyzer calling a fixed MCP tool by name by fixed Go code path is exactly as deterministic as it calling `client.Get()` against the Kubernetes API. Nothing about this ADR gives an analyzer, or the MCP server it calls, any LLM-driven decision-making of its own (see the rejected alternative in Consequences).

### The server's job broadens to fleet-wide correlation

Past `Explain`/`Propose`, the server gains an extensibility point: pluggable correlation sources that enrich the evidence going into an `Explain` call with fleet-wide and external context, alongside whatever the agent already gathered locally. Two motivating sources, named here to establish the shape of the extensibility point, **not fully designed in this ADR**:

- **Cross-cluster correlation** — free from ADR-0008's namespace-per-cluster model: the server already watches every `fleet-*` namespace, so "does this same fingerprint (or a related one) appear in other clusters right now?" is a query against state it already has, no new integration required.
- **External sources (GitHub, ArgoCD, etc.)** — genuinely new integrations, each needing its own API client, credential, and correlation logic (e.g., "was there a merged PR or a new deploy shortly before this incident opened?", "does ArgoCD show this namespace's last sync failed?"). Each such source is scoped as its **own follow-on ADR and implementation effort**, matching how `talam-mesh-mcp` itself got its own ADR-0006 rather than being designed inline in ADR-0001. This ADR establishes that the server is the right place for these — it's the one component with a fleet-wide view and the natural place to hold SaaS credentials that no single spoke cluster's agent should need — not what each one does.

Architecturally, correlation hints from these sources are just more evidence feeding the same `Explain` call ADR-0002 already defined — no new LLM call shape, no change to the two-call (`Explain`, then `Propose`) contract. `ModelBinding` stays exactly what it is: one server-side singleton, used for every incident fleet-wide, unaffected by any of this.

## Consequences

**Wins:**
- Zero remaining cross-cluster network exceptions — talam-server touches no spoke cluster directly, for any reason, after this ADR. The trust-boundary story ADR-0001 started is now fully closed.
- Evidence-gathering code lives next to the analyzer that needs it, versioned and released together — no `AnalyzerID`-string-matching indirection between two processes that used to need independent updates.
- The server's correlation role now matches what only it can uniquely see (the whole fleet, external SaaS state) instead of doing per-cluster work (MCP fetching) it had to reach across a trust boundary for.

**Costs:**
- Analyzers gain a new capability (calling a local MCP client) and, with it, a new failure mode to handle gracefully — an MCP call timing out or erroring shouldn't fail the whole `Analyze()` call, the same non-fatal-on-enrichment-failure posture ADR-0006 already established, just enforced by analyzer authors now instead of one central call site.
- The server accumulates new external credentials (a GitHub token, an ArgoCD token) as correlation sources are added — each is a new, real secret to protect, though held in exactly one place (the server) rather than replicated per-cluster.
- `docs/components/mesh-mcp/README.md` and `docs/api/crds.md`'s `MCPServer` section need updating to reflect agent-side ownership; not done as part of writing this decision down.

**Rejected alternative — MCP server becomes agentic:** considered and explicitly rejected. Giving `talam-mesh-mcp` (or any registered MCP server) its own LLM-driven reasoning loop, rather than exposing fixed deterministic tools, would reopen exactly the risk ADR-0002 and ADR-0006 were written to close — an unreviewed, non-deterministic component sitting between raw cluster state and what a human eventually reviews. "MCP server" in this system means "a deterministic tool provider," full stop, regardless of which process calls it.

**Not done in this ADR:** implementation (relocating `internal/server/mcp` to the agent, adding MCP-client calls to individual analyzers, and the cross-cluster/GitHub/ArgoCD correlation sources themselves — each of the latter is its own follow-on ADR, not scoped here).
