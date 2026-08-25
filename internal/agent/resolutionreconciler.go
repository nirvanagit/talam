// ResolutionReconciler watches local MeshResolution objects and applies the
// ones a human has authorized — spec.triggered set true, mirrored down by
// CRDSync from a server-side approval (ADR-0005) — reusing the same
// resourceVersion-gated apply path as before. It is the sole trigger for
// Applier.apply; nothing else in this package calls it anymore.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

type ResolutionReconciler struct {
	Namespace string
	Dynamic   dynamic.Interface
	Applier   *Applier
	Log       *slog.Logger
}

func (r *ResolutionReconciler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r *ResolutionReconciler) reconcileOnce(ctx context.Context) {
	list, err := r.Dynamic.Resource(gvrMeshResolution).Namespace(r.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		r.Log.Error("listing MeshResolutions failed", "err", err)
		return
	}
	for i := range list.Items {
		obj := &list.Items[i]
		triggered, _, _ := unstructured.NestedBool(obj.Object, "spec", "triggered")
		performed, _, _ := unstructured.NestedBool(obj.Object, "status", "performed")
		outcomeReported, _, _ := unstructured.NestedBool(obj.Object, "status", "outcomeReported")

		switch {
		case triggered && !performed:
			// Not yet applied: apply now. This never re-runs once performed
			// is true, however reportOutcome below turns out — an apply is
			// never retried, only the report of it (see MeshResolutionStatus).
			if err := r.applyAndRecord(ctx, obj); err != nil {
				r.Log.Error("resolution reconcile failed", "resolution", obj.GetName(), "err", err)
			}
		case performed && !outcomeReported:
			// Already applied (or failed to), but talam-server never
			// acknowledged it — retry only the report, never the apply.
			if err := r.retryOutcomeReport(ctx, obj); err != nil {
				r.Log.Warn("outcome report retry failed", "resolution", obj.GetName(), "err", err)
			}
		}
	}
}

func (r *ResolutionReconciler) applyAndRecord(ctx context.Context, obj *unstructured.Unstructured) error {
	p, err := proposalFromResolutionSpec(obj)
	if err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	outcome := r.Applier.apply(ctx, p)

	// "performed" is unconditionally true here: reaching this call at all
	// means an apply was attempted (Applied) or refused/failed (Failed) —
	// either way it's no longer "not yet tried", which is the only state
	// "performed" needs to distinguish (see ADR-0005). It never flips back,
	// and it never causes another call to Applier.apply.
	serverProposalID, _, _ := unstructured.NestedString(obj.Object, "spec", "serverProposalId")
	reportErr := r.Applier.reportOutcome(ctx, serverProposalID, outcome)

	status := map[string]any{
		"performed":       true,
		"outcomeReported": reportErr == nil,
		"dryRunDiff":      outcome.DryRunDiff,
		"detail":          outcome.Detail,
	}
	if outcome.Success {
		status["phase"] = string(api.ProposalApplied)
		status["appliedAt"] = time.Now().UTC().Format(time.RFC3339)
	} else {
		status["phase"] = string(api.ProposalFailed)
	}

	body, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return err
	}
	if _, err := r.Dynamic.Resource(gvrMeshResolution).Namespace(r.Namespace).
		Patch(ctx, obj.GetName(), types.MergePatchType, body, metav1.PatchOptions{}, "status"); err != nil {
		return fmt.Errorf("status patch: %w", err)
	}
	return nil
}

// retryOutcomeReport re-sends the already-computed outcome to talam-server
// for a resolution that was performed locally but never got acknowledged —
// no apply happens here, just the report.
func (r *ResolutionReconciler) retryOutcomeReport(ctx context.Context, obj *unstructured.Unstructured) error {
	serverProposalID, _, _ := unstructured.NestedString(obj.Object, "spec", "serverProposalId")
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	dryRunDiff, _, _ := unstructured.NestedString(obj.Object, "status", "dryRunDiff")
	detail, _, _ := unstructured.NestedString(obj.Object, "status", "detail")

	outcome := api.OutcomeRequest{
		Success:    phase == string(api.ProposalApplied),
		DryRunDiff: dryRunDiff,
		Detail:     detail,
	}
	if err := r.Applier.reportOutcome(ctx, serverProposalID, outcome); err != nil {
		return err
	}

	body, err := json.Marshal(map[string]any{"status": map[string]any{"outcomeReported": true}})
	if err != nil {
		return err
	}
	_, err = r.Dynamic.Resource(gvrMeshResolution).Namespace(r.Namespace).
		Patch(ctx, obj.GetName(), types.MergePatchType, body, metav1.PatchOptions{}, "status")
	return err
}

// proposalFromResolutionSpec adapts a MeshResolution's spec into the
// api.RemediationProposal shape Applier.apply already knows how to execute.
func proposalFromResolutionSpec(obj *unstructured.Unstructured) (api.RemediationProposal, error) {
	spec, _, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil || spec == nil {
		return api.RemediationProposal{}, fmt.Errorf("missing spec")
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return api.RemediationProposal{}, err
	}
	var wire struct {
		Target                mesh.ResourceRef  `json:"target"`
		TargetResourceVersion string            `json:"targetResourceVersion"`
		Patch                 []api.JSONPatchOp `json:"patch"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return api.RemediationProposal{}, err
	}
	return api.RemediationProposal{
		ID:                    obj.GetName(),
		Target:                wire.Target,
		TargetResourceVersion: wire.TargetResourceVersion,
		Patch:                 wire.Patch,
	}, nil
}
