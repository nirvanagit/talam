package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/nirvanagit/talam/pkg/api"
)

// patchableKinds is the allowlist of resources the applier will ever touch.
// Per the security model, remediation writes are scoped to Istio CRDs —
// never Secrets, never RBAC objects, never workloads.
var patchableKinds = map[string]schema.GroupVersionResource{
	"DestinationRule": {Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"},
	"VirtualService":  {Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"},
	"Gateway":         {Group: "networking.istio.io", Version: "v1", Resource: "gateways"},
	"ServiceEntry":    {Group: "networking.istio.io", Version: "v1", Resource: "serviceentries"},
}

// Applier polls talam-server for human-approved proposals targeting this
// cluster and applies them: server-side dry run first, then the real patch,
// then reports the outcome either way (docs/concepts/remediation-flow.md).
type Applier struct {
	ServerURL string
	Cluster   string
	Dynamic   dynamic.Interface
	Client    *http.Client
	Log       *slog.Logger
	Interval  time.Duration
}

func (ap *Applier) Run(ctx context.Context) {
	ticker := time.NewTicker(ap.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ap.pollOnce(ctx)
		}
	}
}

func (ap *Applier) pollOnce(ctx context.Context) {
	url := fmt.Sprintf("%s/v1/proposals?state=%s&cluster=%s", ap.ServerURL, api.ProposalApproved, ap.Cluster)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := ap.Client.Do(req)
	if err != nil {
		ap.Log.Warn("proposal poll failed", "err", err)
		return
	}
	defer resp.Body.Close()
	var proposals []api.RemediationProposal
	if err := json.NewDecoder(resp.Body).Decode(&proposals); err != nil {
		ap.Log.Warn("proposal poll decode failed", "err", err)
		return
	}
	for _, p := range proposals {
		outcome := ap.apply(ctx, p)
		ap.reportOutcome(ctx, p.ID, outcome)
	}
}

func (ap *Applier) apply(ctx context.Context, p api.RemediationProposal) api.OutcomeRequest {
	gvr, ok := patchableKinds[p.Target.Kind]
	if !ok {
		return api.OutcomeRequest{Success: false, Detail: fmt.Sprintf("kind %q is not in the patchable allowlist", p.Target.Kind)}
	}
	if p.TargetResourceVersion == "" {
		// Every proposal minted by the gateway carries this (internal/server/llm).
		// Its absence means the proposal predates that guarantee or was
		// tampered with — refuse rather than apply an index-based patch blind.
		return api.OutcomeRequest{Success: false, Detail: "proposal has no targetResourceVersion; refusing to apply an unpinned patch"}
	}
	// Prepend a "test" op asserting the live object's resourceVersion still
	// matches what the proposal was generated from. JSON Patch applies all
	// ops atomically server-side, so if the target has changed since the
	// proposal was created — another patch, a person, GitOps — the whole
	// patch is rejected instead of silently editing whatever now sits at the
	// patch's array index (see docs/concepts/remediation-flow.md).
	patchOps := append([]api.JSONPatchOp{
		{Op: "test", Path: "/metadata/resourceVersion", Value: p.TargetResourceVersion},
	}, p.Patch...)
	patch, err := json.Marshal(patchOps)
	if err != nil {
		return api.OutcomeRequest{Success: false, Detail: "marshal patch: " + err.Error()}
	}
	res := ap.Dynamic.Resource(gvr).Namespace(p.Target.Namespace)

	// Dry run first, always (--dry-run=server semantics).
	dry, err := res.Patch(ctx, p.Target.Name, types.JSONPatchType, patch,
		metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
	if err != nil {
		ap.Log.Error("dry run failed", "proposal", p.ID, "err", err)
		return api.OutcomeRequest{Success: false, Detail: "dry run failed: " + err.Error()}
	}
	diff := renderDryRun(dry.Object)

	if _, err := res.Patch(ctx, p.Target.Name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		ap.Log.Error("apply failed", "proposal", p.ID, "err", err)
		return api.OutcomeRequest{Success: false, DryRunDiff: diff, Detail: "apply failed: " + err.Error()}
	}
	ap.Log.Info("proposal applied", "proposal", p.ID, "target", p.Target.String())
	return api.OutcomeRequest{Success: true, DryRunDiff: diff, Detail: "applied"}
}

// renderDryRun renders the dry-run result's spec as YAML for the audit trail.
func renderDryRun(obj map[string]any) string {
	spec, ok := obj["spec"]
	if !ok {
		return ""
	}
	b, err := yaml.Marshal(spec)
	if err != nil {
		return ""
	}
	return string(b)
}

func (ap *Applier) reportOutcome(ctx context.Context, proposalID string, outcome api.OutcomeRequest) {
	body, _ := json.Marshal(outcome)
	url := fmt.Sprintf("%s/v1/proposals/%s/outcome", ap.ServerURL, proposalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ap.Client.Do(req)
	if err != nil {
		ap.Log.Warn("outcome report failed", "proposal", proposalID, "err", err)
		return
	}
	resp.Body.Close()
}
