# Changelog

All notable changes to talam are documented here. This project follows [semantic versioning](https://semver.org/).

## [Unreleased]

### Changed — Breaking

- **talam-agent no longer applies remediation.** Per [ADR-0007](decisions/0007-agent-never-applies-remediation.md), `Applier` is removed and the agent's `ClusterRole` is now read-only against every mesh resource — no write verb on any Istio CRD, in any cluster. `MeshResolution` is now the artifact an **external system** (a GitOps controller, an existing config pipeline, or a human via `kubectl`) subscribes to and acts on; talam only relays whatever outcome that system reports back onto the object.
  - `MeshResolutionSpec.triggered` renamed to `spec.approved` — same population (mirrors a human's dashboard decision), now purely advisory: talam-agent never reads it.
  - `MeshResolutionStatus.performed` removed; replaced by `status.outcome` (`"Applied"` | `"Failed"`, written externally) and `status.appliedBy`.
  - `MeshResolutionStatus.dryRunDiff` removed — talam's dry-run validation required the same RBAC write verb this change removes, so it no longer exists; a consuming system does its own pre-apply validation.
  - `MeshIncidentStatus.complete` now derives from `status.outcome` being non-empty across referenced resolutions, instead of `status.performed`.
- If no external system is subscribed to `MeshResolution` in a cluster, the escape hatch is a direct `kubectl patch` against the resolution's `status` subresource after applying the change by hand — see [Getting Started, step 5](getting-started.md#5-approve-a-remediation).

## [v0.1.0] — 2026-09-17

**Initial release.** talam ships with core functionality for Istio mesh diagnostics.

### Added

**Components:**
- **talam-operator** — Kubernetes controller for lifecycle management
- **talam-server** — REST API, LLM coordination, MCP registry, evidence enrichment
- **talam-agent** — Cluster-local CRD reconciler, proposal applier
- **talam-mesh-mcp** — Istio-introspection MCP server (DestinationRule, ServiceEntry, mTLS status)

**CRDs:**
- `MeshDiagnostics` — Cluster-scoped config (server endpoint, scan intervals, enabled analyzers)
- `MeshIncident` — Agent-owned, synced from server (findings + proposals)
- `MeshResolution` — Agent-owned, synced from server (remediation state + outcomes)
- `MCPServer` — Register MCP endpoints for deterministic evidence enrichment
- `ModelBinding` — Configure LLM provider/model per namespace (Kubernetes-native env var alternative)

**Analyzers (Istio):**
- `istio-mismatch-routes` — VirtualService route missing in DestinationRule
- `destination-rule-unused` — DestinationRule defined but no VirtualService routes to it
- `mtls-policy-mismatch` — PeerAuthentication and DestinationRule mTLS modes conflict

**Workflow:**
- Deterministic analyzer-first detection (no LLM guessing)
- LLM explains findings and proposes fixes (two API calls per incident)
- Human-in-the-loop approval before any apply
- Atomic, resourceVersion-gated patches (no clobber races)
- MCP-backed evidence enrichment (deterministic, server-side, pre-fetch)
- Full audit trail (incidents, resolutions, outcomes in CRDs)

**Documentation:**
- Architecture & design docs (markdown + ADRs)
- Object model design (all CRDs, relationships, interaction flows)
- Component READMEs (operator, agent, server, mesh-mcp)
- Analyzer interface specification
- Security model & trust boundaries
- Getting Started guide, Design System, Contributing guidelines

### Known Limitations

- Analyzers cover Istio only (mesh-agnostic interface defined, other meshes TBD)
- Manual approval required (no auto-apply policies yet)
- MCP evidence enrichment is Istio-specific
- No cluster federation (single cluster per agent)
- LLM provider must be configured via env var (ModelBinding CRD planned for v0.2)

### Breaking Changes

None (initial release).

### Security

- Agent never holds server credentials (one-way sync via bearer token)
- Server reads mesh state via read-only MCP
- All findings & resolutions stored in cluster CRDs (audit trail)
- Bearer token never logged (wrapped in HTTP roundtripper)
- RBAC scoped to minimal permissions per component

### Contributors

- nirvanagit (architecture, implementation)
- Anthropic Claude (design, documentation, code review)

---

## Roadmap (v0.2+)

- [ ] Analyzer support for Linkerd, Amazon Mesh
- [ ] Auto-*approval* policies (`spec.approved` set automatically for allowlisted low-risk classes — still never applied by talam itself, see [ADR-0007](decisions/0007-agent-never-applies-remediation.md)); GitOps-native mode (proposals rendered as PRs)
- [ ] WebUI for incident triage and resolution approval
- [ ] Cluster federation (multi-cluster incident correlation)
- [ ] Policy engine (which checks to run, which clusters, time-based)
- [ ] Slack/email notifications
- [ ] Prometheus metrics (scan latency, proposal quality, apply success rate)

See [GitHub Issues](https://github.com/nirvanagit/talam/issues) for detailed tracking.

---

## How to Report Security Issues

Do **not** open a public issue for security vulnerabilities. Email security@example.com with:

1. Description of the vulnerability
2. Steps to reproduce (if applicable)
3. Potential impact
4. Suggested fix (optional)

We will acknowledge within 48 hours and aim to release a patch within 7 days.

---

## Feedback

Have thoughts on this release? [Join the discussion](https://github.com/nirvanagit/talam/discussions) or [open an issue](https://github.com/nirvanagit/talam/issues).
