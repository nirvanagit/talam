// MCPServer registers an MCP endpoint talam-server's evidence-enrichment
// step can call — see ADR-0006 for why these calls are server-initiated and
// deterministic, never a live tool-calling decision the LLM makes.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddMCPTypesToScheme registers MCPServer.
func AddMCPTypesToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &MCPServer{}, &MCPServerList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// MCPServerToolset tags what kind of tools an MCPServer exposes, so the
// enrichment table (internal/server/enrich) can reference "the metrics
// server" or "the mesh server" generically rather than a specific object
// name — see ADR-0006.
type MCPServerToolset string

const (
	MCPToolsetMetrics    MCPServerToolset = "metrics"
	MCPToolsetKubernetes MCPServerToolset = "kubernetes"
	MCPToolsetMesh       MCPServerToolset = "mesh"
	MCPToolsetCustom     MCPServerToolset = "custom"
)

// MCPServerSpec is a connection to one MCP endpoint.
type MCPServerSpec struct {
	// Toolset tags what this server is for — see MCPServerToolset.
	Toolset MCPServerToolset `json:"toolset"`
	// Endpoint is the streamable-HTTP MCP endpoint URL.
	Endpoint string `json:"endpoint"`
	// AuthSecretRef optionally points at a Secret (same namespace) holding a
	// bearer token to authenticate to Endpoint, under key "token".
	AuthSecretRef *SecretKeyRef `json:"authSecretRef,omitempty"`
}

// MCPServerStatus reflects the last time talam-server successfully connected.
type MCPServerStatus struct {
	Ready          bool     `json:"ready"`
	Message        string   `json:"message,omitempty"`
	AvailableTools []string `json:"availableTools,omitempty"`
	LastCheckedAt  string   `json:"lastCheckedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MCPServerSpec   `json:"spec,omitempty"`
	Status            MCPServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

func (in *MCPServer) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MCPServer)
	*out = *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if in.Spec.AuthSecretRef != nil {
		ref := *in.Spec.AuthSecretRef
		out.Spec.AuthSecretRef = &ref
	}
	out.Status.AvailableTools = append([]string(nil), in.Status.AvailableTools...)
	return out
}

func (in *MCPServerList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(MCPServerList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = *in.ListMeta.DeepCopy()
	out.Items = make([]MCPServer, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*MCPServer)
	}
	return out
}
