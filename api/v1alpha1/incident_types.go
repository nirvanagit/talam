// MeshIncident and MeshResolution are the CRD realization of the server-side
// Incident / RemediationProposal concepts (pkg/api) — see ADR-0005 for why
// they live here, agent-owned, rather than being created directly by
// talam-server.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// AddIncidentTypesToScheme registers MeshIncident and MeshResolution.
// Callers wanting everything in this package should use AddAllToScheme instead.
func AddIncidentTypesToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&MeshIncident{}, &MeshIncidentList{},
		&MeshResolution{}, &MeshResolutionList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// AddAllToScheme registers every type in this package.
func AddAllToScheme(scheme *runtime.Scheme) error {
	if err := AddToScheme(scheme); err != nil {
		return err
	}
	return AddIncidentTypesToScheme(scheme)
}

// --- MeshIncident ------------------------------------------------------------

// MeshIncidentSpec mirrors what talam-server currently believes about this
// incident. It's agent-managed (populated by the Sync loop in
// internal/agent), never hand-authored — see ADR-0005.
type MeshIncidentSpec struct {
	// Fingerprint identifies "the same problem" the way mesh.Finding.Fingerprint does.
	Fingerprint string `json:"fingerprint"`
	// ServerIncidentID is talam-server's own ID for this incident, needed to
	// correlate future syncs and to look it up via the Fleet API.
	ServerIncidentID string `json:"serverIncidentId"`
	// Findings is the current finding set backing this incident.
	Findings  []mesh.Finding `json:"findings"`
	FirstSeen metav1.Time    `json:"firstSeen"`
}

// MeshIncidentStatus reflects the LLM gateway's output and this incident's
// remediation completeness, both driven by agent-local reconcilers.
type MeshIncidentStatus struct {
	// State mirrors api.IncidentState ("Open" | "Resolved").
	State    string      `json:"state,omitempty"`
	LastSeen metav1.Time `json:"lastSeen,omitempty"`

	Explanation  string `json:"explanation,omitempty"`
	ExplainError string `json:"explainError,omitempty"`

	// ResolutionRefs lists the MeshResolution objects created for this
	// incident (same namespace), populated by the Sync loop.
	ResolutionRefs []LocalObjectReference `json:"resolutionRefs,omitempty"`

	// Complete is set by the incident reconciler once every resolution in
	// ResolutionRefs has a non-empty status.outcome (Succeeded or Failed,
	// reported by whatever external system applied it — a Rejected
	// resolution does NOT count; see ADR-0005, amended by ADR-0007). False,
	// including when ResolutionRefs is empty, until at least one resolution
	// has an outcome.
	Complete bool `json:"complete"`
}

// LocalObjectReference names another object in the same namespace — avoids a
// dependency on corev1 for this one small shape.
type LocalObjectReference struct {
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MeshIncident struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MeshIncidentSpec   `json:"spec,omitempty"`
	Status            MeshIncidentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MeshIncidentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshIncident `json:"items"`
}

func (in *MeshIncident) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshIncident)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec.Fingerprint = in.Spec.Fingerprint
	out.Spec.ServerIncidentID = in.Spec.ServerIncidentID
	out.Spec.FirstSeen = in.Spec.FirstSeen
	out.Spec.Findings = append([]mesh.Finding(nil), in.Spec.Findings...)
	out.Status = in.Status
	out.Status.ResolutionRefs = append([]LocalObjectReference(nil), in.Status.ResolutionRefs...)
	return out
}

func (in *MeshIncidentList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshIncidentList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = *in.ListMeta.DeepCopy()
	out.Items = make([]MeshIncident, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*MeshIncident)
	}
	return out
}

// --- MeshResolution ------------------------------------------------------------

// MeshResolutionSpec is the CRD realization of api.RemediationProposal — the
// proposed fix, in full: target, patch, and the resourceVersion it was
// computed against. talam never applies it (see ADR-0007); this object is
// the artifact an external system (GitOps controller, existing config
// pipeline, human via kubectl) subscribes to and acts on. Populated by the
// Sync loop; the one field a human or process is expected to flip is
// Approved.
type MeshResolutionSpec struct {
	// IncidentRef names the MeshIncident (same namespace) this resolves.
	IncidentRef string `json:"incidentRef"`
	// ServerProposalID is talam-server's ID for this proposal, needed to
	// report the outcome back (POST /v1/proposals/{id}/outcome).
	ServerProposalID string `json:"serverProposalId"`

	Target                mesh.ResourceRef  `json:"target"`
	TargetResourceVersion string            `json:"targetResourceVersion"`
	Summary               string            `json:"summary"`
	Explanation           string            `json:"explanation,omitempty"`
	RiskTier              api.RiskTier      `json:"riskTier"`
	Patch                 []api.JSONPatchOp `json:"patch"`

	// Approved records that a human or process has reviewed and endorsed
	// this proposal (mirrored here by the Sync loop from a server-side
	// approval, or set directly via kubectl as an escape hatch). It is
	// advisory only — talam-agent never reads it to decide whether to act,
	// because talam-agent never acts (ADR-0007). A subscribing system is
	// free to honor it as a gate on its own automation, or to ignore it
	// entirely and apply on its own criteria.
	Approved bool `json:"approved"`
}

// MeshResolutionStatus reports the proposal's outcome once some external
// system has acted on it. talam-agent never populates Outcome, AppliedBy, or
// AppliedAt itself — those are written by whatever applied the patch
// (kubectl, a GitOps controller, a custom operator), typically via its own
// status subresource patch. See ADR-0007.
//
// Field ownership (see ADR-0005, amended by ADR-0007): Phase is synced from
// talam-server on every tick *until* Outcome becomes non-empty — this is how
// a Rejected decision still shows up here even though nothing local ever
// acts on it. Once Outcome is set, ResolutionReconciler is the sole owner of
// OutcomeReported; the sync loop never touches this object's status again.
type MeshResolutionStatus struct {
	// Phase mirrors api.ProposalState ("Pending" | "Approved" | "Rejected" | "Applied" | "Failed").
	Phase string `json:"phase,omitempty"`
	// Outcome is set by the external system that applied (or attempted to
	// apply) this proposal — "Applied" or "Failed" (api.ProposalApplied /
	// api.ProposalFailed). Empty means no external system has reported back
	// yet. This is what the incident reconciler waits on; once non-empty it
	// never resets. ResolutionReconciler copies it into Phase once relayed.
	Outcome string `json:"outcome,omitempty"`
	// AppliedBy identifies whatever set Outcome — a GitOps controller name,
	// a person's identity, a pipeline run ID. Free text, optional.
	AppliedBy string `json:"appliedBy,omitempty"`
	// OutcomeReported is true once POST /v1/proposals/{id}/outcome has
	// succeeded for the current Outcome. The resolution reconciler retries
	// only this report — never anything that touches the mesh — until it
	// flips true, so talam-server's durable history doesn't get stuck out of
	// sync with what the external system reported.
	OutcomeReported bool         `json:"outcomeReported"`
	Detail          string       `json:"detail,omitempty"`
	AppliedAt       *metav1.Time `json:"appliedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MeshResolution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MeshResolutionSpec   `json:"spec,omitempty"`
	Status            MeshResolutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MeshResolutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshResolution `json:"items"`
}

func (in *MeshResolution) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshResolution)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Spec.Patch = append([]api.JSONPatchOp(nil), in.Spec.Patch...)
	out.Status = in.Status
	if in.Status.AppliedAt != nil {
		t := *in.Status.AppliedAt
		out.Status.AppliedAt = &t
	}
	return out
}

func (in *MeshResolutionList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshResolutionList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = *in.ListMeta.DeepCopy()
	out.Items = make([]MeshResolution, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*MeshResolution)
	}
	return out
}
