# Changelog

All notable changes to talam are documented here. This project follows [semantic versioning](https://semver.org/).

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
- [ ] Auto-apply policies (conditional, progressive rollout)
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
