// Package llm is talam-server's LLM gateway. The gateway makes two calls per
// incident — explain, then propose — and schema-validates every structured
// response before storage (ADR-0002). A malformed or hallucinated patch is
// rejected at this boundary and the incident surfaces as "explanation only".
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// Provider is one chat completion backend. Implementations: Anthropic API
// (default) and the local claude CLI (for laptops with a Claude subscription
// but no API key). Both take a system prompt and user prompt, return text.
type Provider interface {
	Name() string
	Complete(ctx context.Context, system, user string) (string, error)
}

// Gateway turns incidents into explanations and remediation proposals.
// ProviderFunc is resolved on every call (not cached) so a live ModelBinding
// change takes effect on the next incident without restarting the server.
type Gateway struct {
	ProviderFunc func() Provider
}

const explainSystem = `You are talam, a service-mesh diagnostics assistant. You receive structured
evidence from deterministic Istio analyzers. Explain the root cause in plain
English for an SRE: what is broken, why traffic is affected, and what they
should look at. Be concrete and reference the actual resource names from the
evidence. 3-6 sentences, no markdown headings, no speculation beyond the evidence.`

const proposeSystem = `You are talam, a service-mesh remediation assistant. Given analyzer evidence
and a root-cause explanation, propose ONE minimal fix as an RFC 6902 JSON patch
against a single Istio resource. Respond with ONLY a JSON object, no prose, no
code fences, in this exact shape:
{
  "summary": "<one line>",
  "riskTier": "Low" | "Medium" | "High",
  "target": {"kind": "<DestinationRule|VirtualService|Gateway|ServiceEntry>", "namespace": "<ns>", "name": "<name>"},
  "patch": [{"op": "add|remove|replace", "path": "/spec/...", "value": <optional>}]
}
The patch must be valid against the resource's live shape as shown in the
evidence. Prefer removing broken config over inventing new config — if the
evidence gives you an exact array index (e.g. "subsetIndex"), use it directly
in the path (e.g. "/spec/subsets/2"); never guess an index that isn't in the
evidence. If no safe single-resource patch exists, respond with
{"noPatch": true, "reason": "<why>"}.`

// Explain produces the plain-language root cause for an incident.
func (g *Gateway) Explain(ctx context.Context, inc api.Incident) (string, error) {
	evidence, err := json.MarshalIndent(incidentEvidence(inc), "", "  ")
	if err != nil {
		return "", err
	}
	out, err := g.ProviderFunc().Complete(ctx, explainSystem, string(evidence))
	if err != nil {
		return "", fmt.Errorf("explain call: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", fmt.Errorf("explain call returned empty response")
	}
	return out, nil
}

// proposalWire is the shape the model must return for a proposal.
type proposalWire struct {
	NoPatch  bool              `json:"noPatch"`
	Reason   string            `json:"reason"`
	Summary  string            `json:"summary"`
	RiskTier api.RiskTier      `json:"riskTier"`
	Target   mesh.ResourceRef  `json:"target"`
	Patch    []api.JSONPatchOp `json:"patch"`
}

// ErrNoPatch is returned when the model declines to propose a patch; the
// incident then surfaces as explanation-only, which is a valid outcome.
type ErrNoPatch struct{ Reason string }

func (e ErrNoPatch) Error() string { return "no safe patch proposed: " + e.Reason }

// Propose asks for a structured fix and validates it before returning.
func (g *Gateway) Propose(ctx context.Context, inc api.Incident, explanation string) (*api.RemediationProposal, error) {
	input := map[string]any{
		"evidence":    incidentEvidence(inc),
		"explanation": explanation,
	}
	body, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, err
	}
	out, err := g.ProviderFunc().Complete(ctx, proposeSystem, string(body))
	if err != nil {
		return nil, fmt.Errorf("propose call: %w", err)
	}
	var wire proposalWire
	if err := json.Unmarshal([]byte(extractJSON(out)), &wire); err != nil {
		return nil, fmt.Errorf("proposal is not valid JSON (rejected at schema boundary): %w", err)
	}
	if wire.NoPatch {
		return nil, ErrNoPatch{Reason: wire.Reason}
	}
	if err := validateProposal(wire); err != nil {
		return nil, fmt.Errorf("proposal rejected at schema boundary: %w", err)
	}
	// Never trust a resourceVersion echoed by the model — derive it from the
	// finding evidence talam itself collected. This doubles as a check that
	// the model's target is actually one of the resources this incident's
	// evidence is about, not an invented one.
	targetRV, ok := targetResourceVersion(inc, wire.Target)
	if !ok {
		return nil, fmt.Errorf("proposal rejected at schema boundary: target %s does not match any finding's resource in this incident's evidence", wire.Target)
	}
	return &api.RemediationProposal{
		IncidentID:            inc.ID,
		Cluster:               clusterOf(inc),
		Target:                wire.Target,
		TargetResourceVersion: targetRV,
		Summary:               wire.Summary,
		Explanation:           explanation,
		RiskTier:              wire.RiskTier,
		Patch:                 wire.Patch,
	}, nil
}

// targetResourceVersion looks up the resourceVersion talam collected for the
// finding whose Resource matches target — the only source of truth for it;
// see RemediationProposal.TargetResourceVersion.
func targetResourceVersion(inc api.Incident, target mesh.ResourceRef) (string, bool) {
	for _, f := range inc.Findings {
		if f.Resource != target {
			continue
		}
		rv, ok := f.RawEvidence["resourceVersion"].(string)
		return rv, ok && rv != ""
	}
	return "", false
}

var patchableKinds = map[string]bool{
	"DestinationRule": true, "VirtualService": true, "Gateway": true, "ServiceEntry": true,
}

func validateProposal(w proposalWire) error {
	if w.Summary == "" {
		return fmt.Errorf("missing summary")
	}
	switch w.RiskTier {
	case api.RiskLow, api.RiskMedium, api.RiskHigh:
	default:
		return fmt.Errorf("invalid riskTier %q", w.RiskTier)
	}
	if !patchableKinds[w.Target.Kind] {
		return fmt.Errorf("target kind %q is not a patchable Istio resource", w.Target.Kind)
	}
	if w.Target.Name == "" || w.Target.Namespace == "" {
		return fmt.Errorf("target name/namespace missing")
	}
	if len(w.Patch) == 0 {
		return fmt.Errorf("empty patch")
	}
	for i, op := range w.Patch {
		switch op.Op {
		case "add", "remove", "replace":
		default:
			return fmt.Errorf("patch[%d]: op %q not allowed", i, op.Op)
		}
		if op.Path != "/spec" && !strings.HasPrefix(op.Path, "/spec/") {
			return fmt.Errorf("patch[%d]: path %q must stay under /spec", i, op.Path)
		}
	}
	return nil
}

// incidentEvidence strips the incident down to what the model needs: analyzer
// ids, resources, and raw structured evidence — no server-internal ids.
func incidentEvidence(inc api.Incident) []map[string]any {
	out := make([]map[string]any, 0, len(inc.Findings))
	for _, f := range inc.Findings {
		out = append(out, map[string]any{
			"analyzer":    f.AnalyzerID,
			"severity":    f.Severity,
			"resource":    f.Resource,
			"relatedRefs": f.RelatedRefs,
			"evidence":    f.RawEvidence,
			"cluster":     f.Cluster,
		})
	}
	return out
}

func clusterOf(inc api.Incident) string {
	if len(inc.Findings) > 0 {
		return inc.Findings[0].Cluster
	}
	return ""
}

// extractJSON tolerates models wrapping JSON in code fences or prose by
// slicing from the first '{' to the last '}'.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
