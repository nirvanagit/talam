// IncidentReconciler watches local MeshIncident objects and, purely from
// local MeshResolution objects (no talam-server round-trip needed), computes
// status.resolutionRefs and status.complete — true once every resolution
// referencing the incident has been Performed. A Rejected resolution does
// not count as performed, so a rejected-only incident stays incomplete
// (ADR-0005).
package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type IncidentReconciler struct {
	Namespace string
	Dynamic   dynamic.Interface
	Log       *slog.Logger
}

func (r *IncidentReconciler) Run(ctx context.Context, interval time.Duration) {
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

func (r *IncidentReconciler) reconcileOnce(ctx context.Context) {
	incidents, err := r.Dynamic.Resource(gvrMeshIncident).Namespace(r.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		r.Log.Error("listing MeshIncidents failed", "err", err)
		return
	}
	resolutions, err := r.Dynamic.Resource(gvrMeshResolution).Namespace(r.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		r.Log.Error("listing MeshResolutions failed", "err", err)
		return
	}

	byIncident := map[string][]*unstructured.Unstructured{}
	for i := range resolutions.Items {
		res := &resolutions.Items[i]
		ref, _, _ := unstructured.NestedString(res.Object, "spec", "incidentRef")
		byIncident[ref] = append(byIncident[ref], res)
	}

	for i := range incidents.Items {
		inc := &incidents.Items[i]
		refs := byIncident[inc.GetName()]
		if err := r.reconcileOne(ctx, inc, refs); err != nil {
			r.Log.Error("incident reconcile failed", "incident", inc.GetName(), "err", err)
		}
	}
}

func (r *IncidentReconciler) reconcileOne(ctx context.Context, inc *unstructured.Unstructured, resolutions []*unstructured.Unstructured) error {
	refs := make([]map[string]any, 0, len(resolutions))
	complete := len(resolutions) > 0
	for _, res := range resolutions {
		refs = append(refs, map[string]any{"name": res.GetName()})
		performed, _, _ := unstructured.NestedBool(res.Object, "status", "performed")
		if !performed {
			complete = false
		}
	}

	current, _, _ := unstructured.NestedBool(inc.Object, "status", "complete")
	currentRefs, _, _ := unstructured.NestedSlice(inc.Object, "status", "resolutionRefs")
	if current == complete && len(currentRefs) == len(refs) {
		return nil // no change — skip the API call
	}

	body, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"resolutionRefs": refs,
			"complete":       complete,
		},
	})
	if err != nil {
		return err
	}
	_, err = r.Dynamic.Resource(gvrMeshIncident).Namespace(r.Namespace).
		Patch(ctx, inc.GetName(), types.MergePatchType, body, metav1.PatchOptions{}, "status")
	return err
}
