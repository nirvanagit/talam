// Package api defines the wire types shared by talam-server, talam-agent,
// and talamctl. v0.1 transport is JSON over HTTP; mTLS gRPC is roadmap
// (see docs/components/server/README.md).
package api

import (
	"time"

	"github.com/nirvanagit/talam/pkg/mesh"
)

// ReportRequest is one batch of findings pushed by an agent.
type ReportRequest struct {
	Cluster  string         `json:"cluster"`
	Findings []mesh.Finding `json:"findings"`
}

// ReportResponse acknowledges an ingested batch.
type ReportResponse struct {
	Accepted int `json:"accepted"`
}

// IncidentState tracks an incident through its lifecycle.
type IncidentState string

const (
	IncidentOpen     IncidentState = "Open"
	IncidentResolved IncidentState = "Resolved"
)

// Incident is the server-side correlation of one or more Findings that
// describe the same underlying problem. Agents never produce Incidents.
type Incident struct {
	ID          string         `json:"id"`
	Fingerprint string         `json:"fingerprint"`
	State       IncidentState  `json:"state"`
	Findings    []mesh.Finding `json:"findings"`
	FirstSeen   time.Time      `json:"firstSeen"`
	LastSeen    time.Time      `json:"lastSeen"`

	// Explanation is the LLM gateway's plain-language root cause ("" until
	// the explain call completes).
	Explanation string `json:"explanation,omitempty"`
	// ExplainError records a failed or rejected LLM interaction; per the
	// architecture doc such incidents surface as "explanation only" or raw.
	ExplainError string `json:"explainError,omitempty"`

	// Complete is computed (never stored) from this incident's proposals: true
	// once every RemediationProposal for it has an outcome — reached Applied
	// or Failed, as reported by whatever external system acted on it (see
	// ADR-0007). A Rejected proposal does not count, and an incident with
	// zero proposals is never complete. See ADR-0005.
	Complete bool `json:"complete"`
}

// RiskTier classifies how dangerous a proposed patch is to apply.
type RiskTier string

const (
	RiskLow    RiskTier = "Low"
	RiskMedium RiskTier = "Medium"
	RiskHigh   RiskTier = "High"
)

// ProposalState tracks a RemediationProposal through the human-approval flow
// (docs/concepts/remediation-flow.md). Every transition is recorded, never
// silently discarded.
type ProposalState string

const (
	ProposalPending  ProposalState = "Pending"
	ProposalApproved ProposalState = "Approved"
	ProposalRejected ProposalState = "Rejected"
	ProposalApplied  ProposalState = "Applied"
	ProposalFailed   ProposalState = "Failed"
)

// JSONPatchOp is one RFC 6902 operation. The LLM proposes these; the server
// schema-validates them before storage, and the agent server-side dry-runs
// before applying.
type JSONPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// RemediationProposal is a structured fix proposed by the LLM gateway against
// an Incident. talam-agent never applies it (ADR-0007) — it's synced down as
// a MeshResolution for whatever external system (GitOps controller, existing
// config pipeline, human via kubectl) is subscribed to act on it.
type RemediationProposal struct {
	ID         string           `json:"id"`
	IncidentID string           `json:"incidentId"`
	Cluster    string           `json:"cluster"`
	Target     mesh.ResourceRef `json:"target"`
	// TargetResourceVersion is the target's resourceVersion at the time the
	// evidence it was proposed from was collected. Index-based patches (e.g.
	// "/spec/subsets/1") are only safe against the exact object shape they
	// were computed from, so a consuming system should treat a live
	// resourceVersion mismatch as a reason to refuse the patch rather than
	// apply it blind — talam-agent no longer performs this check itself
	// since it no longer applies anything (ADR-0007).
	TargetResourceVersion string `json:"targetResourceVersion,omitempty"`

	Summary     string        `json:"summary"`
	Explanation string        `json:"explanation"`
	RiskTier    RiskTier      `json:"riskTier"`
	Patch       []JSONPatchOp `json:"patch"`

	State     ProposalState `json:"state"`
	CreatedAt time.Time     `json:"createdAt"`
	DecidedAt *time.Time    `json:"decidedAt,omitempty"`
	DecidedBy string        `json:"decidedBy,omitempty"`
	// AppliedBy identifies the external system or person that reported an
	// outcome for this proposal — distinct from DecidedBy, which is whoever
	// approved it. Empty until that system reports (ADR-0007).
	AppliedBy string `json:"appliedBy,omitempty"`
	// Outcome records what the external system that applied this proposal
	// reported back, regardless of result. Empty until that system reports.
	Outcome string `json:"outcome,omitempty"`
}

// DecisionRequest is a CLI/UI approval or rejection of a proposal.
type DecisionRequest struct {
	Approve   bool   `json:"approve"`
	DecidedBy string `json:"decidedBy"`
	Reason    string `json:"reason,omitempty"`
}

// OutcomeRequest is talam-agent forwarding what an external system reported
// after it applied (or failed to apply) an approved proposal — talam-agent
// is a relay here, never the actor (ADR-0007).
type OutcomeRequest struct {
	Success bool `json:"success"`
	// AppliedBy identifies the external system or person that reported this
	// outcome — mirrors MeshResolutionStatus.AppliedBy. Optional.
	AppliedBy string `json:"appliedBy,omitempty"`
	Detail    string `json:"detail,omitempty"`
}
