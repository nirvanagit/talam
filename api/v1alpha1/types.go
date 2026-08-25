// Package v1alpha1 contains the talam.dev/v1alpha1 API: MeshDiagnostics
// (owned by the operator, docs/components/operator/README.md) and
// ModelBinding (watched by the server, docs/components/server/README.md) —
// the Kubernetes-native way to declare which LLM the fleet uses, instead of
// only an environment variable.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const GroupName = "talam.dev"

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// AddToScheme registers these types with a runtime.Scheme.
func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&MeshDiagnostics{}, &MeshDiagnosticsList{},
		&ModelBinding{}, &ModelBindingList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// --- MeshDiagnostics -------------------------------------------------------

// MeshDiagnosticsSpec configures the operator-managed agent for one cluster.
// See docs/api/crds.md#meshdiagnostics.
type MeshDiagnosticsSpec struct {
	ServerEndpoint string            `json:"serverEndpoint"`
	MeshBackend    string            `json:"meshBackend"`
	ScanInterval   metav1.Duration   `json:"scanInterval,omitempty"`
	Analyzers      AnalyzerSelection `json:"analyzers,omitempty"`
	UpgradePolicy  string            `json:"upgradePolicy,omitempty"` // manual | auto-patch | auto-minor
	AgentImage     string            `json:"agentImage,omitempty"`
}

type AnalyzerSelection struct {
	Enabled  []string `json:"enabled,omitempty"`
	Disabled []string `json:"disabled,omitempty"`
}

// MeshDiagnosticsStatus reflects what the operator has observed.
type MeshDiagnosticsStatus struct {
	AgentHealthy       bool   `json:"agentHealthy"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	Message            string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MeshDiagnostics struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MeshDiagnosticsSpec   `json:"spec,omitempty"`
	Status            MeshDiagnosticsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MeshDiagnosticsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshDiagnostics `json:"items"`
}

func (in *MeshDiagnostics) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshDiagnostics)
	*out = *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec.Analyzers.Enabled = append([]string(nil), in.Spec.Analyzers.Enabled...)
	out.Spec.Analyzers.Disabled = append([]string(nil), in.Spec.Analyzers.Disabled...)
	return out
}

func (in *MeshDiagnosticsList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MeshDiagnosticsList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = *in.ListMeta.DeepCopy()
	out.Items = make([]MeshDiagnostics, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*MeshDiagnostics)
	}
	return out
}

// --- ModelBinding ------------------------------------------------------------

// ModelBindingSpec declares which LLM backs talam-server's gateway. This is
// the Kubernetes-native alternative to setting TALAM_LLM_MODEL by hand: point
// talam-server at a ModelBinding, and changing this object live-switches the
// model the fleet uses.
type ModelBindingSpec struct {
	// Provider selects the wire protocol: "anthropic" (default, api.anthropic.com),
	// "openai-compatible" (any OpenAI-chat-completions-shaped endpoint, e.g. a
	// self-hosted model for air-gapped clusters — see docs/components/server/README.md),
	// or "claude-cli" (shells out to a local `claude` binary; local-dev only).
	Provider string `json:"provider"`

	// Model is the model identifier passed to the provider, e.g. "claude-sonnet-5".
	Model string `json:"model,omitempty"`

	// BaseURL overrides the provider's default endpoint. Required for
	// openai-compatible; ignored for claude-cli.
	BaseURL string `json:"baseURL,omitempty"`

	// APIKeySecretRef points at a Secret in the same namespace holding the
	// provider credential. Ignored for claude-cli.
	APIKeySecretRef *SecretKeyRef `json:"apiKeySecretRef,omitempty"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// ModelBindingStatus reflects the last time the server successfully resolved
// and used this binding.
type ModelBindingStatus struct {
	Ready          bool   `json:"ready"`
	Message        string `json:"message,omitempty"`
	LastVerifiedAt string `json:"lastVerifiedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ModelBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ModelBindingSpec   `json:"spec,omitempty"`
	Status            ModelBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ModelBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelBinding `json:"items"`
}

func (in *ModelBinding) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ModelBinding)
	*out = *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if in.Spec.APIKeySecretRef != nil {
		ref := *in.Spec.APIKeySecretRef
		out.Spec.APIKeySecretRef = &ref
	}
	return out
}

func (in *ModelBindingList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ModelBindingList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = *in.ListMeta.DeepCopy()
	out.Items = make([]ModelBinding, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*ModelBinding)
	}
	return out
}
