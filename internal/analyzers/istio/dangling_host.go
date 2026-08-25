package istio

import (
	"context"

	"github.com/nirvanagit/talam/pkg/mesh"
)

// DanglingHost detects VirtualService route destinations that reference a host
// with no backing Service or ServiceEntry in the mesh — requests matching that
// route black-hole with a 503 while the VirtualService itself looks valid.
type DanglingHost struct{}

func (DanglingHost) ID() string                { return "istio.virtualservice.dangling-host" }
func (DanglingHost) Backend() mesh.MeshBackend { return mesh.BackendIstio }
func (DanglingHost) Trigger() mesh.TriggerKind { return mesh.TriggerOnChange }

func (a DanglingHost) Analyze(ctx context.Context, snap *mesh.MeshSnapshot) ([]mesh.Finding, error) {
	st, err := istioState(snap)
	if err != nil {
		return nil, err
	}
	var findings []mesh.Finding
	for _, vs := range st.VirtualServices {
		for _, dest := range vs.DestinationHosts {
			svcName, svcNS, inCluster := parseHost(dest, vs.Namespace)
			if inCluster {
				if findService(&snap.Core, svcName, svcNS) != nil {
					continue
				}
			}
			if serviceEntryCovers(st, dest) {
				continue
			}
			findings = append(findings, mesh.Finding{
				AnalyzerID: a.ID(),
				Severity:   mesh.SeverityCritical,
				Resource:   mesh.ResourceRef{Kind: "VirtualService", Namespace: vs.Namespace, Name: vs.Name},
				RawEvidence: map[string]any{
					"destinationHost":   dest,
					"parsedAsInCluster": inCluster,
					"vsHosts":           vs.Hosts,
				},
				DetectedAt: snap.CollectedAt,
				Cluster:    snap.Cluster,
			})
		}
	}
	return findings, nil
}
