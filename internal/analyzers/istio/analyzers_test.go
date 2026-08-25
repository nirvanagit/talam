package istio

import (
	"context"
	"testing"
	"time"

	"github.com/nirvanagit/talam/internal/istiostate"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// fixture builds a healthy baseline snapshot: httpbin Service backed by one
// ready pod labeled version=v1, a DestinationRule with a v1 subset, a
// VirtualService routing to httpbin, and a Gateway bound to the default
// istio ingress pod.
func fixture() *mesh.MeshSnapshot {
	return &mesh.MeshSnapshot{
		Backend:     mesh.BackendIstio,
		Cluster:     "test",
		CollectedAt: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		Core: mesh.CoreState{
			Services: []mesh.Service{
				{Namespace: "demo", Name: "httpbin", Selector: map[string]string{"app": "httpbin"}},
			},
			Pods: []mesh.Pod{
				{Namespace: "demo", Name: "httpbin-v1", Labels: map[string]string{"app": "httpbin", "version": "v1"}, Ready: true},
				{Namespace: "istio-system", Name: "ingress", Labels: map[string]string{"istio": "ingressgateway"}, Ready: true},
			},
		},
		BackendState: &istiostate.State{
			DestinationRules: []istiostate.DestinationRule{
				{Namespace: "demo", Name: "httpbin", Host: "httpbin", Subsets: []istiostate.Subset{
					{Name: "v1", Labels: map[string]string{"version": "v1"}},
				}},
			},
			VirtualServices: []istiostate.VirtualService{
				{Namespace: "demo", Name: "httpbin", Hosts: []string{"httpbin"}, DestinationHosts: []string{"httpbin"}},
			},
			Gateways: []istiostate.Gateway{
				{Namespace: "demo", Name: "web", Selector: map[string]string{"istio": "ingressgateway"}},
			},
		},
	}
}

func run(t *testing.T, a mesh.Analyzer, snap *mesh.MeshSnapshot) []mesh.Finding {
	t.Helper()
	findings, err := a.Analyze(context.Background(), snap)
	if err != nil {
		t.Fatalf("%s: %v", a.ID(), err)
	}
	return findings
}

func TestHealthyMeshProducesNoFindings(t *testing.T) {
	snap := fixture()
	for _, a := range All() {
		if f := run(t, a, snap); len(f) != 0 {
			t.Errorf("%s: expected no findings on healthy fixture, got %+v", a.ID(), f)
		}
	}
}

func TestOrphanedSubsetDetected(t *testing.T) {
	snap := fixture()
	st := snap.BackendState.(*istiostate.State)
	st.DestinationRules[0].Subsets = append(st.DestinationRules[0].Subsets,
		istiostate.Subset{Name: "v2", Labels: map[string]string{"version": "v2"}})

	findings := run(t, OrphanedSubset{}, snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != mesh.SeverityCritical || f.RawEvidence["subset"] != "v2" {
		t.Errorf("unexpected finding: %+v", f)
	}
	if f.Resource.Kind != "DestinationRule" || f.Resource.Name != "httpbin" {
		t.Errorf("unexpected resource ref: %+v", f.Resource)
	}
}

func TestOrphanedSubsetHonorsFQDNHost(t *testing.T) {
	snap := fixture()
	st := snap.BackendState.(*istiostate.State)
	st.DestinationRules[0].Host = "httpbin.demo.svc.cluster.local"
	st.DestinationRules[0].Subsets = []istiostate.Subset{
		{Name: "v1", Labels: map[string]string{"version": "v1"}},
		{Name: "gone", Labels: map[string]string{"version": "v9"}},
	}
	findings := run(t, OrphanedSubset{}, snap)
	if len(findings) != 1 || findings[0].RawEvidence["subset"] != "gone" {
		t.Fatalf("expected only the 'gone' subset flagged, got %+v", findings)
	}
}

func TestOrphanedSubsetIgnoresNotReadyPods(t *testing.T) {
	snap := fixture()
	snap.Core.Pods[0].Ready = false
	findings := run(t, OrphanedSubset{}, snap)
	if len(findings) != 1 {
		t.Fatalf("subset backed only by a not-ready pod should be flagged, got %+v", findings)
	}
}

func TestDanglingHostDetected(t *testing.T) {
	snap := fixture()
	st := snap.BackendState.(*istiostate.State)
	st.VirtualServices[0].DestinationHosts = []string{"nosuchsvc"}

	findings := run(t, DanglingHost{}, snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", findings)
	}
	if findings[0].RawEvidence["destinationHost"] != "nosuchsvc" {
		t.Errorf("unexpected evidence: %+v", findings[0].RawEvidence)
	}
}

func TestDanglingHostSatisfiedByServiceEntry(t *testing.T) {
	snap := fixture()
	st := snap.BackendState.(*istiostate.State)
	st.VirtualServices[0].DestinationHosts = []string{"api.example.com"}
	st.ServiceEntries = []istiostate.ServiceEntry{
		{Namespace: "demo", Name: "external-api", Hosts: []string{"*.example.com"}},
	}
	if findings := run(t, DanglingHost{}, snap); len(findings) != 0 {
		t.Fatalf("host covered by ServiceEntry wildcard should not be flagged: %+v", findings)
	}
}

func TestUnboundGatewayDetected(t *testing.T) {
	snap := fixture()
	st := snap.BackendState.(*istiostate.State)
	st.Gateways[0].Selector = map[string]string{"istio": "nonexistent-gw"}

	findings := run(t, UnboundGateway{}, snap)
	if len(findings) != 1 || findings[0].Severity != mesh.SeverityWarning {
		t.Fatalf("expected 1 warning finding, got %+v", findings)
	}
}

func TestFingerprintStableAcrossTicks(t *testing.T) {
	f1 := mesh.Finding{Cluster: "c", AnalyzerID: "a", Resource: mesh.ResourceRef{Kind: "K", Namespace: "n", Name: "x"}, DetectedAt: time.Now()}
	f2 := f1
	f2.DetectedAt = f1.DetectedAt.Add(5 * time.Minute)
	if f1.Fingerprint() != f2.Fingerprint() {
		t.Error("fingerprint must not depend on detection time")
	}
}
