// CRDSync is the one-way mirror from talam-server's REST API into this
// cluster's MeshIncident / MeshResolution objects — see ADR-0005
// (docs/decisions/0005-crd-native-incidents-and-resolutions.md) for why the
// agent, not talam-server, is the one writing these, and for the spec/status
// ownership split this file relies on: Sync owns spec plus an object's
// *initial* status on creation; ResolutionReconciler and IncidentReconciler
// own status from then on.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/nirvanagit/talam/pkg/api"
)

var (
	gvrMeshIncident   = schema.GroupVersionResource{Group: "talam.dev", Version: "v1alpha1", Resource: "meshincidents"}
	gvrMeshResolution = schema.GroupVersionResource{Group: "talam.dev", Version: "v1alpha1", Resource: "meshresolutions"}
)

type CRDSync struct {
	ServerURL string
	Cluster   string
	Namespace string
	Dynamic   dynamic.Interface
	Client    *http.Client
	Log       *slog.Logger
}

func (s *CRDSync) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.syncOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *CRDSync) syncOnce(ctx context.Context) {
	incidents, err := getJSON[[]api.Incident](ctx, s.Client, s.ServerURL+"/v1/incidents?cluster="+url.QueryEscape(s.Cluster))
	if err != nil {
		s.Log.Warn("incident sync fetch failed", "err", err)
	} else {
		for _, inc := range incidents {
			if err := s.upsertIncident(ctx, inc); err != nil {
				s.Log.Error("incident sync failed", "incident", inc.ID, "err", err)
			}
		}
	}

	proposals, err := getJSON[[]api.RemediationProposal](ctx, s.Client, s.ServerURL+"/v1/proposals?cluster="+url.QueryEscape(s.Cluster))
	if err != nil {
		s.Log.Warn("proposal sync fetch failed", "err", err)
		return
	}
	for _, p := range proposals {
		if err := s.upsertResolution(ctx, p); err != nil {
			s.Log.Error("resolution sync failed", "proposal", p.ID, "err", err)
		}
	}
}

func (s *CRDSync) upsertIncident(ctx context.Context, inc api.Incident) error {
	res := s.Dynamic.Resource(gvrMeshIncident).Namespace(s.Namespace)
	findings, err := toUnstructuredSlice(inc.Findings)
	if err != nil {
		return err
	}
	spec := map[string]any{
		"fingerprint":      inc.Fingerprint,
		"serverIncidentId": inc.ID,
		"findings":         findings,
		"firstSeen":        inc.FirstSeen.UTC().Format(time.RFC3339),
	}

	existing, err := res.Get(ctx, inc.ID, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "talam.dev/v1alpha1",
			"kind":       "MeshIncident",
			"metadata":   map[string]any{"name": inc.ID, "namespace": s.Namespace},
			"spec":       spec,
		}}
		created, err := res.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		return s.setIncidentSyncStatus(ctx, created, inc)
	}
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	existing.Object["spec"] = spec
	updated, err := res.Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	return s.setIncidentSyncStatus(ctx, updated, inc)
}

// setIncidentSyncStatus merge-patches only the fields Sync owns on
// MeshIncident.status, leaving status.resolutionRefs/complete — owned by
// IncidentReconciler — untouched.
func (s *CRDSync) setIncidentSyncStatus(ctx context.Context, obj *unstructured.Unstructured, inc api.Incident) error {
	patch := map[string]any{
		"status": map[string]any{
			"state":        string(inc.State),
			"lastSeen":     inc.LastSeen.UTC().Format(time.RFC3339),
			"explanation":  inc.Explanation,
			"explainError": inc.ExplainError,
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = s.Dynamic.Resource(gvrMeshIncident).Namespace(s.Namespace).
		Patch(ctx, obj.GetName(), types.MergePatchType, body, metav1.PatchOptions{}, "status")
	return err
}

func (s *CRDSync) upsertResolution(ctx context.Context, p api.RemediationProposal) error {
	res := s.Dynamic.Resource(gvrMeshResolution).Namespace(s.Namespace)
	patchOps, err := toUnstructuredSlice(p.Patch)
	if err != nil {
		return err
	}
	spec := map[string]any{
		"incidentRef":           p.IncidentID,
		"serverProposalId":      p.ID,
		"target":                map[string]any{"kind": p.Target.Kind, "namespace": p.Target.Namespace, "name": p.Target.Name},
		"targetResourceVersion": p.TargetResourceVersion,
		"summary":               p.Summary,
		"explanation":           p.Explanation,
		"riskTier":              string(p.RiskTier),
		"patch":                 patchOps,
		// Authorized once a human has approved (or the reconciler has already
		// acted on) this proposal server-side. Never true while Pending or
		// Rejected — see ADR-0005 and ADR-0003.
		"triggered": p.State != api.ProposalPending && p.State != api.ProposalRejected,
	}

	existing, err := res.Get(ctx, p.ID, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "talam.dev/v1alpha1",
			"kind":       "MeshResolution",
			"metadata":   map[string]any{"name": p.ID, "namespace": s.Namespace},
			"spec":       spec,
			"status": map[string]any{
				"phase":     string(p.State),
				"performed": false,
			},
		}}
		_, err := res.Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	specPatch, err := json.Marshal(map[string]any{"spec": spec})
	if err != nil {
		return err
	}
	if _, err := res.Patch(ctx, p.ID, types.MergePatchType, specPatch, metav1.PatchOptions{}); err != nil {
		return err
	}

	// Keep status.phase mirrored from the server for as long as this
	// resolution hasn't been locally performed — this is how a Rejected (or
	// any other non-apply) decision actually shows up via kubectl, since
	// nothing else ever touches phase for a resolution that never triggers.
	// Once Performed is true, ResolutionReconciler owns status exclusively —
	// see MeshResolutionStatus's doc comment — so phase is left alone here
	// even if the server hasn't caught up yet (e.g. outcome-report lag).
	performed, _, _ := unstructured.NestedBool(existing.Object, "status", "performed")
	if performed {
		return nil
	}
	statusPatch, err := json.Marshal(map[string]any{"status": map[string]any{"phase": string(p.State)}})
	if err != nil {
		return err
	}
	_, err = res.Patch(ctx, p.ID, types.MergePatchType, statusPatch, metav1.PatchOptions{}, "status")
	return err
}

func getJSON[T any](ctx context.Context, client *http.Client, url string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("%s: %s", url, resp.Status)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return zero, err
	}
	return out, nil
}

// toUnstructuredSlice round-trips a typed value through JSON into the
// map[string]any/[]any shape the dynamic client needs for spec fields.
func toUnstructuredSlice(v any) ([]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
