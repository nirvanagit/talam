# Documentation map

talam's documentation lives entirely in markdown, organized in this folder. GitHub renders it natively — no build step, no external dependencies.

## Quick Start

New to talam? Start here:

1. **[Getting Started](getting-started.md)** — Install, run your first scan, approve a remediation (5 min)
2. **[Architecture Overview](architecture/overview.md)** — System design: operator, server, agent, incident/remediation flow
3. **[Roadmap](roadmap.md)** — What's planned (v0.2: multi-mesh, auto-apply; v0.3: observability)
4. **[Design System](design.md)** — Brand, colors, UI principles

## For Everyone Else

- **[Guides](guides.md)** — kubectl recipes, common tasks, troubleshooting
- **[Design System](design.md)** — Brand identity, colors, typography, visual principles
- **[Roadmap](roadmap.md)** — What's coming: multi-mesh, auto-apply, observability, federation
- **[Contribute](contribute.md)** — How to submit code, report bugs, add analyzers
- **[Changelog](changelog.md)** — Release notes, breaking changes, security contact

## Technical Deep Dives

The documentation graph below organizes all technical details:

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

### Getting started
- [`getting-started.md`](getting-started.md) — install, first scan, first remediation
- [`guides.md`](guides.md) — kubectl recipes and quick reference
- [`roadmap.md`](roadmap.md) — v0.1 current state, v0.2-v0.4+ planned features

### Design and contribution
- [`design.md`](design.md) — brand, colors, typography, UI principles
- [`contribute.md`](contribute.md) — how to contribute code, analyzers, docs
- [`changelog.md`](changelog.md) — release notes, roadmap, upgrade guide

### Technical architecture
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
