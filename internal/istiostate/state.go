// Package istiostate defines the lightweight typed views of Istio API objects
// that the Istio collector populates and Istio analyzers read. These are
// deliberately minimal projections — only the fields analyzers need — parsed
// from unstructured objects so talam does not depend on istio/client-go.
package istiostate

// State is the Istio-specific portion of a MeshSnapshot, carried in
// MeshSnapshot.BackendState.
type State struct {
	DestinationRules []DestinationRule `json:"destinationRules"`
	VirtualServices  []VirtualService  `json:"virtualServices"`
	Gateways         []Gateway         `json:"gateways"`
	ServiceEntries   []ServiceEntry    `json:"serviceEntries"`
}

// DestinationRule is a minimal view of networking.istio.io/v1 DestinationRule.
type DestinationRule struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Subsets   []Subset `json:"subsets,omitempty"`
	// ResourceVersion is carried into evidence so a remediation proposal can
	// be gated on it at apply time: if the live object has moved on since the
	// proposal was generated, the applier refuses to apply an index-based
	// patch against what may now be a different array element.
	ResourceVersion string `json:"resourceVersion"`
}

// Subset is one subset entry of a DestinationRule.
type Subset struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// VirtualService is a minimal view of networking.istio.io/v1 VirtualService.
type VirtualService struct {
	Namespace        string   `json:"namespace"`
	Name             string   `json:"name"`
	Hosts            []string `json:"hosts,omitempty"`
	Gateways         []string `json:"gateways,omitempty"`
	DestinationHosts []string `json:"destinationHosts,omitempty"`
	ResourceVersion  string   `json:"resourceVersion"`
}

// Gateway is a minimal view of networking.istio.io/v1 Gateway.
type Gateway struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Selector        map[string]string `json:"selector,omitempty"`
	ResourceVersion string            `json:"resourceVersion"`
}

// ServiceEntry is a minimal view of networking.istio.io/v1 ServiceEntry.
type ServiceEntry struct {
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	Hosts           []string `json:"hosts,omitempty"`
	ResourceVersion string   `json:"resourceVersion"`
}
