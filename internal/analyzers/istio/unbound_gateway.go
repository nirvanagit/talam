package istio

import (
	"context"

	"github.com/nirvanagit/talam/pkg/mesh"
)

// UnboundGateway detects Gateway resources whose workload selector matches no
// ready pod anywhere in the cluster — listeners that will never be programmed
// onto any ingress proxy.
type UnboundGateway struct{}

func (UnboundGateway) ID() string                { return "istio.gateway.unbound-selector" }
func (UnboundGateway) Backend() mesh.MeshBackend { return mesh.BackendIstio }
func (UnboundGateway) Trigger() mesh.TriggerKind { return mesh.TriggerOnChange }

func (a UnboundGateway) Analyze(ctx context.Context, snap *mesh.MeshSnapshot) ([]mesh.Finding, error) {
	st, err := istioState(snap)
	if err != nil {
		return nil, err
	}
	var findings []mesh.Finding
	for _, gw := range st.Gateways {
		if len(gw.Selector) == 0 {
			continue
		}
		// Gateway selectors match workloads across all namespaces.
		if n := readyPodsMatching(&snap.Core, "", gw.Selector); n == 0 {
			findings = append(findings, mesh.Finding{
				AnalyzerID: a.ID(),
				Severity:   mesh.SeverityWarning,
				Resource:   mesh.ResourceRef{Kind: "Gateway", Namespace: gw.Namespace, Name: gw.Name},
				RawEvidence: map[string]any{
					"selector":         gw.Selector,
					"readyPodsMatched": 0,
					"resourceVersion":  gw.ResourceVersion,
				},
				DetectedAt: snap.CollectedAt,
				Cluster:    snap.Cluster,
			})
		}
	}
	return findings, nil
}
