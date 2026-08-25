package server

import (
	"testing"
	"time"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

func finding(cluster, id, resource string, t time.Time) mesh.Finding {
	return mesh.Finding{
		AnalyzerID: id,
		Severity:   mesh.SeverityCritical,
		Resource:   mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: resource},
		DetectedAt: t,
		Cluster:    cluster,
	}
}

func TestIngestCreatesOneIncidentPerFingerprint(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	news := s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{
		finding("c1", "a", "x", t0),
	}})
	if len(news) != 1 {
		t.Fatalf("expected 1 new incident, got %d", len(news))
	}

	// Same fingerprint reported again on the next tick: no new incident.
	news = s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{
		finding("c1", "a", "x", t0.Add(time.Minute)),
	}})
	if len(news) != 0 {
		t.Fatalf("expected 0 new incidents on repeat, got %d", len(news))
	}
	if len(s.Incidents) != 1 {
		t.Fatalf("expected 1 incident total, got %d", len(s.Incidents))
	}
}

func TestIngestResolvesIncidentWhenFindingClears(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", t0)}})

	// Next scan reports nothing for c1: the incident should resolve.
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: nil})

	incidents := s.ListIncidents()
	if len(incidents) != 1 || incidents[0].State != api.IncidentResolved {
		t.Fatalf("expected the incident to resolve, got %+v", incidents)
	}
}

func TestIngestReopensResolvedIncident(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", t0)}})
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: nil}) // resolves

	news := s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", t0.Add(time.Hour))}})
	if len(news) != 1 {
		t.Fatalf("reopening a resolved incident should be treated as new for LLM purposes, got %d", len(news))
	}
	incidents := s.ListIncidents()
	if incidents[0].State != api.IncidentOpen {
		t.Fatalf("expected incident reopened, got %+v", incidents[0])
	}
}

func TestIngestKeepsClustersIndependent(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", t0)}})
	// c2 reporting empty must not resolve c1's incident.
	s.Ingest(api.ReportRequest{Cluster: "c2", Findings: nil})

	incidents := s.ListIncidents()
	if incidents[0].State != api.IncidentOpen {
		t.Fatalf("c2's empty report incorrectly resolved c1's incident: %+v", incidents[0])
	}
}

func TestDecideRejectsNonPending(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.AddProposal(&api.RemediationProposal{IncidentID: "inc-1"})
	var id string
	for k := range s.Proposals {
		id = k
	}
	if _, err := s.Decide(id, api.DecisionRequest{Approve: true, DecidedBy: "sre"}); err != nil {
		t.Fatalf("first decide should succeed: %v", err)
	}
	if _, err := s.Decide(id, api.DecisionRequest{Approve: true, DecidedBy: "sre"}); err == nil {
		t.Fatal("deciding an already-decided proposal should error")
	}
}

func TestRecordOutcomeRequiresApproved(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.AddProposal(&api.RemediationProposal{IncidentID: "inc-1"})
	var id string
	for k := range s.Proposals {
		id = k
	}
	if err := s.RecordOutcome(id, api.OutcomeRequest{Success: true}); err == nil {
		t.Fatal("recording an outcome for a still-pending proposal should error")
	}
}
