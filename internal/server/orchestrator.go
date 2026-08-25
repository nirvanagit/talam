package server

import (
	"context"
	"log/slog"

	"github.com/nirvanagit/talam/internal/server/enrich"
	"github.com/nirvanagit/talam/internal/server/llm"
	"github.com/nirvanagit/talam/pkg/api"
)

// Orchestrator runs the two-call LLM flow (ADR-0002) against every newly
// opened or reopened incident: explain, then propose. A malformed or
// declined proposal leaves the incident as "explanation only" — never a
// fatal error for the incident.
type Orchestrator struct {
	Store    *Store
	Gateway  *llm.Gateway
	Enricher *enrich.Enricher // nil is fine — Enrich is a no-op then (ADR-0006)
	Log      *slog.Logger
}

// Handle processes one batch of newly (re)opened incidents synchronously.
// Called from the ingest handler; v0.1 scale (single fleet, low finding
// volume) doesn't need a queue in front of this yet.
func (o *Orchestrator) Handle(ctx context.Context, incidents []api.Incident) {
	for _, inc := range incidents {
		o.process(ctx, inc)
	}
}

func (o *Orchestrator) process(ctx context.Context, inc api.Incident) {
	inc = o.Enricher.Enrich(ctx, inc)
	explanation, err := o.Gateway.Explain(ctx, inc)
	if err != nil {
		o.Log.Error("explain failed", "incident", inc.ID, "err", err)
		o.Store.SetExplanation(inc.ID, "", err.Error())
		return
	}
	o.Store.SetExplanation(inc.ID, explanation, "")
	o.Log.Info("incident explained", "incident", inc.ID, "provider", o.Gateway.ProviderFunc().Name())

	if !o.Store.ReserveProposalSlot(inc.ID) {
		// Either a proposal already exists, or another process() call for
		// this same incident (e.g. it resolved and reopened while an earlier
		// Propose was still in flight) got there first.
		return
	}
	defer o.Store.ReleaseProposalSlot(inc.ID)

	// Re-fetch: SetExplanation mutated the stored copy, and Propose wants it.
	// Re-enrich too — the store never persists enriched evidence, only the
	// raw Finding data reported by the agent.
	current, ok := o.Store.GetIncident(inc.ID)
	if !ok {
		return
	}
	current = o.Enricher.Enrich(ctx, current)
	proposal, err := o.Gateway.Propose(ctx, current, explanation)
	if err != nil {
		if _, noPatch := err.(llm.ErrNoPatch); noPatch {
			o.Log.Info("no patch proposed", "incident", inc.ID, "reason", err)
		} else {
			o.Log.Error("propose failed", "incident", inc.ID, "err", err)
		}
		return
	}
	o.Store.AddProposal(proposal)
	o.Log.Info("remediation proposed", "incident", inc.ID, "proposal", proposal.ID, "risk", proposal.RiskTier)
}
