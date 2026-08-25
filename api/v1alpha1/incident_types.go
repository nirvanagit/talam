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
	// ResolutionRefs has been Performed (Applied or Failed — a Rejected
	// resolution does NOT count; see ADR-0005). False, including when
	// ResolutionRefs is empty, until at least one resolution has been
	// performed.
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

// MeshResolutionSpec is the CRD realization of api.RemediationProposal.
// Populated by the Sync loop; the one field a human (via the dashboard, or
// directly via kubectl as an escape hatch) is expected to flip is Triggered.
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

	// Triggered authorizes the resolution reconciler to apply this patch.
	// Manual only (ADR-0003): set true only as a result of approval in the
	// dashboard (mirrored here by the Sync loop) or a direct kubectl patch.
	// Never set true by talam-server or by any reconciler in this repo.
	Triggered bool `json:"triggered"`
}

// MeshResolutionStatus reports what the resolution reconciler did.
//
// Field ownership (see ADR-0005): Phase is synced from talam-server on every
// tick *until* Performed becomes true — this is how a Rejected decision (or
// any other server-side transition that never triggers a local apply) still
// shows up here. Once Performed is true, ResolutionReconciler is the sole
// owner of everything below it; the sync loop never touches this object's
// status again, including Phase — Applied/Failed are terminal local facts,
// not something to keep re-mirroring from a server whose own view of "did
// this apply" only updates once OutcomeReported succeeds.
type MeshResolutionStatus struct {
	// Phase mirrors api.ProposalState ("Pending" | "Approved" | "Rejected" | "Applied" | "Failed").
	Phase string `json:"phase,omitempty"`
	// Performed is true once Phase is Applied or Failed — an apply was
	// actually attempted, as opposed to Rejected (a decision not to act) or
	// still Pending. This is what the incident reconciler waits on; it never
	// changes back to false and never gates a re-apply.
	Performed bool `json:"performed"`
	// OutcomeReported is true once POST /v1/proposals/{id}/outcome has
	// succeeded. Performed can be true while this is still false (the apply
	// happened but reporting it back to talam-server failed) — the
	// resolution reconciler retries the report, never the apply, until this
	// flips true, so the server's durable history doesn't get stuck out of
	// sync with what the cluster actually did.
	OutcomeReported bool         `json:"outcomeReported"`
	DryRunDiff      string       `json:"dryRunDiff,omitempty"`
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
