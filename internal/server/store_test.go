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

	incidents := s.ListIncidents("")
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
	incidents := s.ListIncidents("")
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

	incidents := s.ListIncidents("")
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

func TestIngestTruncatesFindingHistory(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	for i := 0; i < maxFindingsPerIncident+5; i++ {
		s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", t0.Add(time.Duration(i)*time.Minute))}})
	}
	incidents := s.ListIncidents("")
	if len(incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(incidents))
	}
	if got := len(incidents[0].Findings); got != maxFindingsPerIncident {
		t.Fatalf("expected findings truncated to %d, got %d", maxFindingsPerIncident, got)
	}
	// The truncation must keep the newest entries, not the oldest.
	last := incidents[0].Findings[len(incidents[0].Findings)-1]
	if !last.DetectedAt.Equal(t0.Add(time.Duration(maxFindingsPerIncident+4) * time.Minute)) {
		t.Errorf("expected the most recent finding retained, got detectedAt=%v", last.DetectedAt)
	}
}

func TestStoreReloadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/store.json"

	s1, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	s1.AddProposal(&api.RemediationProposal{IncidentID: "inc-1"})

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(s2.Incidents) != 1 || len(s2.Proposals) != 1 {
		t.Fatalf("expected reloaded store to have 1 incident and 1 proposal, got %d/%d", len(s2.Incidents), len(s2.Proposals))
	}
	// A freshly reloaded store must still be able to mint IDs and reserve
	// proposal slots — proposing map isn't persisted but must be usable.
	if !s2.ReserveProposalSlot("inc-2") {
		t.Fatal("reloaded store should allow reserving a slot for a fresh incident")
	}
}

func TestReserveProposalSlotIsExclusive(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	if !s.ReserveProposalSlot("inc-1") {
		t.Fatal("first reservation should succeed")
	}
	if s.ReserveProposalSlot("inc-1") {
		t.Fatal("a second concurrent reservation for the same incident must be refused")
	}
	s.ReleaseProposalSlot("inc-1")
	if !s.ReserveProposalSlot("inc-1") {
		t.Fatal("after release, a new reservation should succeed")
	}
}

func TestReserveProposalSlotRefusedIfProposalExists(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.AddProposal(&api.RemediationProposal{IncidentID: "inc-1"})
	if s.ReserveProposalSlot("inc-1") {
		t.Fatal("an incident that already has a live proposal must refuse a new reservation")
	}
}

func TestListIncidentsFiltersByCluster(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	s.Ingest(api.ReportRequest{Cluster: "c2", Findings: []mesh.Finding{finding("c2", "a", "y", time.Now())}})

	c1 := s.ListIncidents("c1")
	if len(c1) != 1 || c1[0].Findings[0].Cluster != "c1" {
		t.Fatalf("expected only c1's incident, got %+v", c1)
	}
	all := s.ListIncidents("")
	if len(all) != 2 {
		t.Fatalf("expected both incidents with no filter, got %d", len(all))
	}
}

func TestRecordOutcomeIsIdempotent(t *testing.T) {
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
		t.Fatal(err)
	}
	if err := s.RecordOutcome(id, api.OutcomeRequest{Success: true, Detail: "applied"}); err != nil {
		t.Fatalf("first RecordOutcome should succeed: %v", err)
	}
	// A retry of the same outcome (e.g. the agent never saw the first ack)
	// must succeed, not error with "not Approved" — the state transition it
	// wanted already happened.
	if err := s.RecordOutcome(id, api.OutcomeRequest{Success: true, Detail: "applied"}); err != nil {
		t.Fatalf("retrying an already-recorded matching outcome must be idempotent, got: %v", err)
	}
	if s.Proposals[id].State != api.ProposalApplied {
		t.Fatalf("expected state to remain Applied, got %s", s.Proposals[id].State)
	}
}

func TestRecordOutcomeMismatchStillRejectedOnceTerminal(t *testing.T) {
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
		t.Fatal(err)
	}
	if err := s.RecordOutcome(id, api.OutcomeRequest{Success: true}); err != nil {
		t.Fatal(err)
	}
	// A second call reporting a DIFFERENT (mismatched) outcome for an
	// already-terminal proposal should still error, not silently flip state.
	if err := s.RecordOutcome(id, api.OutcomeRequest{Success: false}); err == nil {
		t.Fatal("a conflicting outcome for an already-Applied proposal must be rejected")
	}
}

func TestIncidentCompleteRequiresAtLeastOneProposal(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	incidents := s.ListIncidents("")
	if incidents[0].Complete {
		t.Fatal("an incident with zero proposals must never be Complete")
	}
}

func TestIncidentCompleteFalseWhilePending(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	news := s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	s.AddProposal(&api.RemediationProposal{IncidentID: news[0].ID})
	inc, _ := s.GetIncident(news[0].ID)
	if inc.Complete {
		t.Fatal("a still-Pending proposal must leave the incident incomplete")
	}
}

func TestIncidentCompleteFalseWhenRejected(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	news := s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	s.AddProposal(&api.RemediationProposal{IncidentID: news[0].ID})
	var propID string
	for id := range s.Proposals {
		propID = id
	}
	if _, err := s.Decide(propID, api.DecisionRequest{Approve: false, DecidedBy: "sre"}); err != nil {
		t.Fatal(err)
	}
	inc, _ := s.GetIncident(news[0].ID)
	if inc.Complete {
		t.Fatal("a Rejected proposal must not count as performed — the incident must stay incomplete (ADR-0005)")
	}
}

func TestIncidentCompleteTrueOnceAllPerformed(t *testing.T) {
	s, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	news := s.Ingest(api.ReportRequest{Cluster: "c1", Findings: []mesh.Finding{finding("c1", "a", "x", time.Now())}})
	s.AddProposal(&api.RemediationProposal{IncidentID: news[0].ID})
	var propID string
	for id := range s.Proposals {
		propID = id
	}
	if _, err := s.Decide(propID, api.DecisionRequest{Approve: true, DecidedBy: "sre"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOutcome(propID, api.OutcomeRequest{Success: true}); err != nil {
		t.Fatal(err)
	}
	inc, _ := s.GetIncident(news[0].ID)
	if !inc.Complete {
		t.Fatal("an incident whose only proposal was Applied should be Complete")
	}
}
