# Roadmap

talam is in active development. Here's what's planned.

## v0.1 (Released 2026-09-17)

**Core functionality for Istio mesh diagnostics.**

- ✅ Server + agent + operator architecture
- ✅ CRD-native incident and remediation tracking
- ✅ Deterministic analyzers (Istio-specific)
- ✅ LLM-powered explanations and proposals
- ✅ Human-in-the-loop approval workflow
- ✅ MCP-backed evidence enrichment
- ✅ Full audit trail (all state in CRDs)
- ✅ Kubernetes-native configuration (MeshDiagnostics, MCPServer, ModelBinding)

**Known limitations:**
- Istio only (other meshes supported in v0.2+)
- Manual approval required (no auto-apply)
- Single cluster per agent (federation in v0.2)
- LLM provider via env var (ModelBinding CRD in v0.2)

---

## v0.2 (Q4 2026)

**Multi-mesh support and auto-apply policies.**

### Multi-Mesh
- [ ] Analyzer backend for Linkerd
- [ ] Analyzer backend for AWS App Mesh / Amazon Mesh
- [ ] Mesh-agnostic remediation proposals
- [ ] Test suite across all three meshes

### Auto-Apply Policies
- [ ] `RemediationPolicy` CRD: define which proposals auto-apply
- [ ] Dry-run validation before apply (already implemented, now opt-in)
- [ ] Progressive rollout: canary apply to 1 cluster, then all
- [ ] Automatic rollback on incident regression

### Fleet Management
- [ ] Multi-cluster federation: correlate incidents across clusters
- [ ] Central server sees all incidents from all agents
- [ ] Proposal consistency: same incident type gets same fix everywhere
- [ ] Cross-cluster dependency detection (e.g., "service A calls service B")

### Developer Experience
- [ ] WebUI for incident triage and approval
- [ ] Real-time incident dashboard
- [ ] Search and filter across all incidents
- [ ] Export incidents to CSV/JSON

---

## v0.3 (Q1 2027)

**Observability and notifications.**

- [ ] Prometheus metrics
  - Scan latency (time to run all analyzers)
  - Proposal quality (user approval rate)
  - Apply success rate (how many applied cleanly)
  - Incident MTTD (mean time to detection)

- [ ] Notifications
  - Slack: "new incident in namespace X"
  - Email digests (daily summary)
  - PagerDuty integration (critical incidents)
  - Custom webhooks

- [ ] Observability
  - OpenTelemetry tracing for analyzer execution
  - Logs: structured JSON from all components
  - Live tail: watch analyzer output as it runs

---

## v0.4+ (Backlog)

### Analysis Enhancements
- [ ] Behavioral anomaly detection (machine learning)
- [ ] Custom analyzer framework (user-defined rules in CRD)
- [ ] Remediation templates (parameterized fixes)

### Governance
- [ ] Policy engine: enforce which checks run where, when
- [ ] RBAC integration: only approved users can see certain clusters
- [ ] Audit log archival (export to S3/GCS)
- [ ] Compliance report generation (CIS, CISA guidelines)

### Ecosystem
- [ ] Terraform provider (define talam config as IaC)
- [ ] Helm chart for easier deployment
- [ ] kubectl plugin (talam as a native kubectl command)
- [ ] IDE extension (show incident status in VS Code)

---

## How to Contribute

Want to help build one of these features?

1. **Pick a feature** from the roadmap above
2. **Open a [discussion](https://github.com/nirvanagit/talam/discussions)** to discuss the approach
3. **Create an ADR** if it affects architecture
4. **Submit a PR** and we'll review together

See [Contribute](contribute.md) for the full workflow.

---

## Priorities

We prioritize based on:
1. **User requests** — if multiple people ask for it, it moves up
2. **Architecture alignment** — does it fit the v0.1 vision?
3. **Unblocking other work** — does it enable the next phase?

Have a request? [Open an issue](https://github.com/nirvanagit/talam/issues) or [start a discussion](https://github.com/nirvanagit/talam/discussions).
