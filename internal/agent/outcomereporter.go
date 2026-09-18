package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nirvanagit/talam/pkg/api"
)

// OutcomeReporter forwards to talam-server what an external system reported
// after it acted on an approved MeshResolution. talam-agent never applies a
// patch itself — see ADR-0007 (docs/decisions/0007-agent-never-applies-remediation.md)
// — this type only knows how to relay an outcome someone else produced. What
// notices a new outcome to relay is ResolutionReconciler's job
// (internal/agent/resolutionreconciler.go).
type OutcomeReporter struct {
	ServerURL string
	Cluster   string
	Client    *http.Client
	Log       *slog.Logger
}

// reportOutcome tells talam-server what an external system reported.
// Returns an error rather than only logging one — callers that track whether
// the report actually landed (e.g. ResolutionReconciler's outcomeReported
// bookkeeping) need to know so they can retry; a report that's merely
// logged-and-forgotten on failure is what produced the split-brain this
// return value exists to fix.
func (r *OutcomeReporter) reportOutcome(ctx context.Context, proposalID string, outcome api.OutcomeRequest) error {
	body, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v1/proposals/%s/outcome", r.ServerURL, proposalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		r.Log.Warn("outcome report failed", "proposal", proposalID, "err", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("server returned %s", resp.Status)
		r.Log.Warn("outcome report rejected", "proposal", proposalID, "err", err)
		return err
	}
	return nil
}
