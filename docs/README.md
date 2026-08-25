# Documentation map

This folder is organized as a graph: every doc is a **node** with a single clear purpose, and links between docs are **edges** — explicit, relative, and load-bearing. There's no separate index database; the graph *is* the folder structure plus the links inside each file. GitHub renders it natively, so the graph is walkable with nothing but a browser.

## Node types

| Folder | Node type | Answers |
|---|---|---|
| [`architecture/`](architecture/) | System-level design | "How do the pieces fit together?" |
| [`components/`](components/) | One node per deployable | "What does *this* piece do, own, and depend on?" |
| [`concepts/`](concepts/) | Cross-cutting ideas | "What is a Finding? What is mesh-agnostic mean here?" |
| [`decisions/`](decisions/) | ADRs — why, not what | "Why server+agent+operator instead of a single binary?" |
| [`api/`](api/) | Contracts | "What's the exact shape of a CRD / RPC / schema?" |
| [`glossary/`](glossary/) | Shared vocabulary | "What does talam mean by 'Incident' vs 'Finding'?" |

## Linking conventions

1. **Every doc opens with a "Related" block** listing the nodes it depends on (reads) and the nodes that depend on it (is read by). This is what makes the graph traversable in both directions — you're never stuck at a dead end.
2. **Link to the concept, not just the mention.** The first time a doc uses a graph term (`Finding`, `Incident`, `MeshSnapshot`, `RemediationProposal`) it links to that term's definition in [`concepts/`](concepts/) or [`glossary/`](glossary/).
3. **ADRs are immutable once accepted.** A decision that changes gets a new ADR that supersedes the old one (see [`decisions/README.md`](decisions/README.md)) — the graph keeps history instead of overwriting it.
4. **Components link to the ADRs that constrain them**, not the other way around — ADRs stay reusable across components; components stay honest about which decisions they're bound by.
5. **No orphans.** Every doc added here must be linked from at least one other node (typically this file, or the component/concept it belongs under) before merge.

## Full node list

- [`architecture/overview.md`](architecture/overview.md) — system topology, principles, roadmap
- [`components/operator/README.md`](components/operator/README.md)
- [`components/agent/README.md`](components/agent/README.md)
- [`components/server/README.md`](components/server/README.md)
- [`components/mesh-mcp/README.md`](components/mesh-mcp/README.md)
- [`concepts/analyzer-interface.md`](concepts/analyzer-interface.md)
- [`concepts/finding-and-incident.md`](concepts/finding-and-incident.md)
- [`concepts/remediation-flow.md`](concepts/remediation-flow.md)
- [`concepts/security-model.md`](concepts/security-model.md)
- [`concepts/object-model.md`](concepts/object-model.md) — every CRD, what owns it, how they reference each other
- [`decisions/README.md`](decisions/README.md) — ADR index
- [`api/crds.md`](api/crds.md)
- [`api/analyzer-catalog.md`](api/analyzer-catalog.md)
- [`glossary/README.md`](glossary/README.md)
