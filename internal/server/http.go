package server

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nirvanagit/talam/internal/server/llm"
	"github.com/nirvanagit/talam/pkg/api"
)

//go:embed ui/dist
var uiFS embed.FS

// Server wires the store, orchestrator, and HTTP handlers together into the
// Fleet API + UI subsystem (docs/components/server/README.md).
type Server struct {
	Store        *Store
	Orchestrator *Orchestrator
	Resolver     *llm.BindingResolver
	Log          *slog.Logger
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /v1/findings", s.handleReport)
	mux.HandleFunc("GET /v1/incidents", s.handleListIncidents)
	mux.HandleFunc("GET /v1/incidents/{id}", s.handleGetIncident)
	mux.HandleFunc("GET /v1/proposals", s.handleListProposals)
	mux.HandleFunc("POST /v1/proposals/{id}/decision", s.handleDecide)
	mux.HandleFunc("POST /v1/proposals/{id}/outcome", s.handleOutcome)
	mux.HandleFunc("GET /v1/stats", s.handleStats)
	mux.Handle("/", uiHandler())
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReport is the Ingest API: receives a finding batch from an agent,
// deduplicates/correlates it into incidents, and kicks off the LLM gateway
// for anything newly opened. The LLM call runs synchronously so `make demo`
// can observe end-to-end completion by polling /v1/incidents, but it never
// blocks the agent — this handler returns to the agent immediately after
// storing, and processes the LLM flow after responding.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var req api.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newIncidents := s.Store.Ingest(req)
	writeJSON(w, http.StatusOK, api.ReportResponse{Accepted: len(req.Findings)})

	if len(newIncidents) > 0 {
		go s.Orchestrator.Handle(context.Background(), newIncidents)
	}
}

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.ListIncidents())
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	inc, ok := s.Store.GetIncident(r.PathValue("id"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

func (s *Server) handleListProposals(w http.ResponseWriter, r *http.Request) {
	state := api.ProposalState(r.URL.Query().Get("state"))
	cluster := r.URL.Query().Get("cluster")
	writeJSON(w, http.StatusOK, s.Store.ListProposals(state, cluster))
}

// handleDecide is the CLI/UI approval endpoint. It never applies anything
// itself — that's the agent's job after polling for Approved proposals
// (docs/concepts/remediation-flow.md).
func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var req api.DecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.DecidedBy == "" {
		http.Error(w, "decidedBy is required — remediation must be attributable to a person", http.StatusBadRequest)
		return
	}
	p, err := s.Store.Decide(r.PathValue("id"), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleOutcome(w http.ResponseWriter, r *http.Request) {
	var req api.OutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Store.RecordOutcome(r.PathValue("id"), req); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.Store.Stats()
	if s.Resolver != nil {
		stats.LLMProvider = s.Resolver.Current().Name()
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
