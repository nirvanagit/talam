package istio

import (
	"context"

	"github.com/nirvanagit/talam/pkg/mesh"
)

// OrphanedSubset detects DestinationRule subsets whose label selector matches
// zero ready pods backing the rule's host. Traffic routed to such a subset
// gets "no healthy upstream" while every pod involved looks healthy.
type OrphanedSubset struct{}

func (OrphanedSubset) ID() string                { return "istio.destinationrule.orphaned-subset" }
func (OrphanedSubset) Backend() mesh.MeshBackend { return mesh.BackendIstio }
func (OrphanedSubset) Trigger() mesh.TriggerKind { return mesh.TriggerOnChange }

func (a OrphanedSubset) Analyze(ctx context.Context, snap *mesh.MeshSnapshot) ([]mesh.Finding, error) {
	st, err := istioState(snap)
	if err != nil {
		return nil, err
	}
	var findings []mesh.Finding
	for _, dr := range st.DestinationRules {
		svcName, svcNS, ok := parseHost(dr.Host, dr.Namespace)
		if !ok {
			continue
		}
		svc := findService(&snap.Core, svcName, svcNS)
		for i, subset := range dr.Subsets {
			if len(subset.Labels) == 0 {
				continue
			}
			// The pods a subset selects are the service's pods narrowed by the
			// subset labels; without a resolvable service, fall back to the
			// subset labels alone in the host's namespace.
			selector := subset.Labels
			if svc != nil {
				selector = merged(svc.Selector, subset.Labels)
			}
			if n := readyPodsMatching(&snap.Core, svcNS, selector); n == 0 {
				related := []mesh.ResourceRef{{Kind: "Service", Namespace: svcNS, Name: svcName}}
				findings = append(findings, mesh.Finding{
					AnalyzerID:  a.ID(),
					Severity:    mesh.SeverityCritical,
					Resource:    mesh.ResourceRef{Kind: "DestinationRule", Namespace: dr.Namespace, Name: dr.Name},
					RelatedRefs: related,
					RawEvidence: map[string]any{
						"host":             dr.Host,
						"subset":           subset.Name,
						"subsetIndex":      i, // position in spec.subsets — the array index a JSON patch removing this entry needs
						"subsetLabels":     subset.Labels,
						"serviceResolved":  svc != nil,
						"readyPodsMatched": 0,
						// resourceVersion at scan time — a proposal must echo this back so
						// the applier can refuse to apply an index-based patch against a
						// DestinationRule that has since changed (see internal/agent/applier.go).
						"resourceVersion": dr.ResourceVersion,
					},
					DetectedAt: snap.CollectedAt,
					Cluster:    snap.Cluster,
				})
			}
		}
	}
	return findings, nil
}
