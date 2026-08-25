// Package server implements talam-server: findings ingest, incident
// correlation, the LLM gateway wiring, the remediation broker, and the
// management UI/API.
package server

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// maxFindingsPerIncident bounds how much finding history one incident keeps.
const maxFindingsPerIncident = 20

// Store is the v0.1 history store: in-memory, protected by a mutex, and
// persisted as a JSON file on every mutation. Postgres is the roadmap
// (docs/components/server/README.md); the interface surface is kept small so
// swapping the backing store doesn't touch handlers.
type Store struct {
	mu        sync.Mutex
	path      string
	Incidents map[string]*api.Incident            `json:"incidents"` // by incident ID
	Proposals map[string]*api.RemediationProposal `json:"proposals"` // by proposal ID
	seq       int
	// proposing tracks incident IDs with a Propose call currently in flight —
	// not persisted, just an in-process lock closing the gap between checking
	// "does this incident have a proposal yet" and the LLM round-trip that
	// creates one. See ReserveProposalSlot.
	proposing map[string]bool
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:      path,
		Incidents: map[string]*api.Incident{},
		Proposals: map[string]*api.RemediationProposal{},
		proposing: map[string]bool{},
	}
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("corrupt store file %s: %w", path, err)
	}
	// Resume the ID sequence past whatever was already persisted, so reload
	// doesn't risk re-minting an ID collision.
	s.seq = len(s.Incidents) + len(s.Proposals)
	return s, nil
}

func (s *Store) persistLocked() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err == nil {
		_ = os.Rename(tmp, s.path)
	}
}

func (s *Store) nextIDLocked(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().Unix(), s.seq)
}

// Ingest merges one reported batch into incident state. It returns incidents
// newly created by this batch (candidates for the LLM gateway). Because every
// agent reports its full current finding set each scan, any open incident for
// that cluster whose fingerprint is absent from the batch has cleared and is
// marked Resolved.
func (s *Store) Ingest(req api.ReportRequest) (newIncidents []api.Incident) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]bool{}
	for _, f := range req.Findings {
		if f.Cluster == "" {
			f.Cluster = req.Cluster
		}
		fp := f.Fingerprint()
		seen[fp] = true
		inc := s.findByFingerprintLocked(fp)
		if inc == nil {
			inc = &api.Incident{
				ID:          s.nextIDLocked("inc"),
				Fingerprint: fp,
				State:       api.IncidentOpen,
				FirstSeen:   f.DetectedAt,
			}
			s.Incidents[inc.ID] = inc
			inc.Findings = append(inc.Findings, f)
			inc.LastSeen = f.DetectedAt
			newIncidents = append(newIncidents, *inc)
			continue
		}
		if inc.State == api.IncidentResolved {
			inc.State = api.IncidentOpen
			newIncidents = append(newIncidents, *inc)
		}
		inc.Findings = append(inc.Findings, f)
		if len(inc.Findings) > maxFindingsPerIncident {
			inc.Findings = inc.Findings[len(inc.Findings)-maxFindingsPerIncident:]
		}
		inc.LastSeen = f.DetectedAt
	}

	for _, inc := range s.Incidents {
		if inc.State != api.IncidentOpen || len(inc.Findings) == 0 {
			continue
		}
		if inc.Findings[0].Cluster == req.Cluster && !seen[inc.Fingerprint] {
			inc.State = api.IncidentResolved
		}
	}
	s.persistLocked()
	return newIncidents
}

func (s *Store) findByFingerprintLocked(fp string) *api.Incident {
	for _, inc := range s.Incidents {
		if inc.Fingerprint == fp {
			return inc
		}
	}
	return nil
}

// SetExplanation records the explain-call result (or its failure) on an incident.
func (s *Store) SetExplanation(incidentID, explanation, explainErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inc, ok := s.Incidents[incidentID]; ok {
		inc.Explanation = explanation
		inc.ExplainError = explainErr
		s.persistLocked()
	}
}

// AddProposal stores a validated proposal in Pending state.
func (s *Store) AddProposal(p *api.RemediationProposal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = s.nextIDLocked("prop")
	p.State = api.ProposalPending
	p.CreatedAt = time.Now().UTC()
	s.Proposals[p.ID] = p
	s.persistLocked()
}

// HasProposalForIncident reports whether a non-terminal proposal already
// exists for an incident. Exported for callers that just want a read (e.g.
// tests); the orchestrator uses ReserveProposalSlot instead, which combines
// this check with an in-flight lock so two concurrent propose attempts for
// the same incident can't both pass it.
func (s *Store) HasProposalForIncident(incidentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasProposalForIncidentLocked(incidentID)
}

func (s *Store) hasProposalForIncidentLocked(incidentID string) bool {
	for _, p := range s.Proposals {
		if p.IncidentID == incidentID && p.State != api.ProposalRejected && p.State != api.ProposalFailed {
			return true
		}
	}
	return false
}

// ReserveProposalSlot atomically checks "no proposal exists yet for this
// incident and no propose call is already in flight for it" and, if true,
// marks one in flight. Returns false if either condition fails — the caller
// must not call Propose. Always pair with a deferred ReleaseProposalSlot.
func (s *Store) ReserveProposalSlot(incidentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proposing[incidentID] || s.hasProposalForIncidentLocked(incidentID) {
		return false
	}
	s.proposing[incidentID] = true
	return true
}

// ReleaseProposalSlot clears the in-flight marker set by ReserveProposalSlot.
// Safe to call unconditionally once a propose attempt finishes, whether it
// succeeded, failed, or declined.
func (s *Store) ReleaseProposalSlot(incidentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.proposing, incidentID)
}

// Decide records a human approval or rejection of a pending proposal.
func (s *Store) Decide(id string, d api.DecisionRequest) (*api.RemediationProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Proposals[id]
	if !ok {
		return nil, fmt.Errorf("no such proposal %q", id)
	}
	if p.State != api.ProposalPending {
		return nil, fmt.Errorf("proposal %q is %s, not Pending", id, p.State)
	}
	now := time.Now().UTC()
	p.DecidedAt = &now
	p.DecidedBy = d.DecidedBy
	if d.Approve {
		p.State = api.ProposalApproved
	} else {
		p.State = api.ProposalRejected
		p.Outcome = "rejected: " + d.Reason
	}
	s.persistLocked()
	return p, nil
}

// RecordOutcome records the agent's apply result. Outcomes are recorded
// regardless of success — rejected patches and failed dry-runs are history
// too (docs/concepts/remediation-flow.md). Idempotent: a proposal already in
// the terminal state this same outcome would produce is treated as success,
// not an error — the agent retries a report whose HTTP response it never
// saw (e.g. the POST landed and was processed, but the ack was lost), and
// that retry must not fail just because the state transition already
// happened (see internal/agent/resolutionreconciler.go's outcomeReported
// bookkeeping, ADR-0005).
func (s *Store) RecordOutcome(id string, o api.OutcomeRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Proposals[id]
	if !ok {
		return fmt.Errorf("no such proposal %q", id)
	}
	wantState := api.ProposalFailed
	if o.Success {
		wantState = api.ProposalApplied
	}
	if p.State == wantState {
		return nil
	}
	if p.State != api.ProposalApproved {
		return fmt.Errorf("proposal %q is %s, not Approved", id, p.State)
	}
	p.DryRunDiff = o.DryRunDiff
	p.Outcome = o.Detail
	p.State = wantState
	s.persistLocked()
	return nil
}

// ListIncidents returns incidents newest-first, optionally filtered to one
// cluster (matched against the incident's own findings, same as proposals).
func (s *Store) ListIncidents(cluster string) []api.Incident {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]api.Incident, 0, len(s.Incidents))
	for _, inc := range s.Incidents {
		if cluster != "" && !incidentInCluster(inc, cluster) {
			continue
		}
		out = append(out, s.withCompleteLocked(*inc))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

func incidentInCluster(inc *api.Incident, cluster string) bool {
	return len(inc.Findings) > 0 && inc.Findings[0].Cluster == cluster
}

// withCompleteLocked computes Incident.Complete: true once every proposal for
// this incident has been Performed (Applied or Failed) and there's at least
// one. A Rejected proposal leaves the incident incomplete — see ADR-0005.
// Caller must hold s.mu.
func (s *Store) withCompleteLocked(inc api.Incident) api.Incident {
	n := 0
	for _, p := range s.Proposals {
		if p.IncidentID != inc.ID {
			continue
		}
		n++
		if p.State != api.ProposalApplied && p.State != api.ProposalFailed {
			inc.Complete = false
			return inc
		}
	}
	inc.Complete = n > 0
	return inc
}

// GetIncident returns one incident by ID.
func (s *Store) GetIncident(id string) (api.Incident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inc, ok := s.Incidents[id]
	if !ok {
		return api.Incident{}, false
	}
	return s.withCompleteLocked(*inc), true
}

// ListProposals returns proposals newest-first, optionally filtered.
func (s *Store) ListProposals(state api.ProposalState, cluster string) []api.RemediationProposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.RemediationProposal{}
	for _, p := range s.Proposals {
		if state != "" && p.State != state {
			continue
		}
		if cluster != "" && p.Cluster != cluster {
			continue
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Stats summarizes store contents for the dashboard.
type Stats struct {
	OpenIncidents     int            `json:"openIncidents"`
	ResolvedIncidents int            `json:"resolvedIncidents"`
	BySeverity        map[string]int `json:"bySeverity"`
	PendingProposals  int            `json:"pendingProposals"`
	AppliedProposals  int            `json:"appliedProposals"`
	Clusters          []string       `json:"clusters"`
	LLMProvider       string         `json:"llmProvider"`
}

func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Stats{BySeverity: map[string]int{}}
	clusters := map[string]bool{}
	for _, inc := range s.Incidents {
		if inc.State == api.IncidentOpen {
			st.OpenIncidents++
			if len(inc.Findings) > 0 {
				st.BySeverity[string(latestSeverity(inc))]++
			}
		} else {
			st.ResolvedIncidents++
		}
		for _, f := range inc.Findings {
			clusters[f.Cluster] = true
		}
	}
	for _, p := range s.Proposals {
		switch p.State {
		case api.ProposalPending:
			st.PendingProposals++
		case api.ProposalApplied:
			st.AppliedProposals++
		}
	}
	for c := range clusters {
		st.Clusters = append(st.Clusters, c)
	}
	sort.Strings(st.Clusters)
	return st
}

func latestSeverity(inc *api.Incident) mesh.Severity {
	return inc.Findings[len(inc.Findings)-1].Severity
}
