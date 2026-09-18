// ResolutionReconciler watches local MeshResolution objects for an outcome
// that some external system has reported (status.outcome, set by whatever
// applied the patch — a GitOps controller, a custom operator, a human via
// kubectl) and relays it to talam-server. talam-agent never applies a patch
// itself; see ADR-0007
// (docs/decisions/0007-agent-never-applies-remediation.md). It is the sole
// caller of OutcomeReporter.reportOutcome; nothing else in this package
// calls it.
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
)

type ResolutionReconciler struct {
	Namespace       string
	Dynamic         dynamic.Interface
	OutcomeReporter *OutcomeReporter
	Log             *slog.Logger
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
		outcome, _, _ := unstructured.NestedString(obj.Object, "status", "outcome")
		outcomeReported, _, _ := unstructured.NestedBool(obj.Object, "status", "outcomeReported")

		if outcome != "" && !outcomeReported {
			if err := r.relayOutcome(ctx, obj); err != nil {
				r.Log.Warn("outcome relay failed", "resolution", obj.GetName(), "err", err)
			}
		}
	}
}

// relayOutcome forwards an externally-reported outcome to talam-server and,
// once that succeeds, marks it reported (so it's never relayed twice) and
// copies Outcome into Phase — the local terminal fact, no longer subject to
// CRDSync's phase mirroring (see MeshResolutionStatus's doc comment).
func (r *ResolutionReconciler) relayOutcome(ctx context.Context, obj *unstructured.Unstructured) error {
	serverProposalID, _, _ := unstructured.NestedString(obj.Object, "spec", "serverProposalId")
	outcomeStr, _, _ := unstructured.NestedString(obj.Object, "status", "outcome")
	appliedBy, _, _ := unstructured.NestedString(obj.Object, "status", "appliedBy")
	detail, _, _ := unstructured.NestedString(obj.Object, "status", "detail")

	outcome := api.OutcomeRequest{
		Success:   outcomeStr == string(api.ProposalApplied),
		AppliedBy: appliedBy,
		Detail:    detail,
	}
	if err := r.OutcomeReporter.reportOutcome(ctx, serverProposalID, outcome); err != nil {
		return fmt.Errorf("report outcome: %w", err)
	}

	body, err := json.Marshal(map[string]any{
		"status": map[string]any{"outcomeReported": true, "phase": outcomeStr},
	})
	if err != nil {
		return err
	}
	_, err = r.Dynamic.Resource(gvrMeshResolution).Namespace(r.Namespace).
		Patch(ctx, obj.GetName(), types.MergePatchType, body, metav1.PatchOptions{}, "status")
	return err
}
