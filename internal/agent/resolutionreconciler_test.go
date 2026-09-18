package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func newFakeMeshResolution(namespace, name, incidentRef, serverProposalID string, approved bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "talam.dev/v1alpha1",
		"kind":       "MeshResolution",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"incidentRef":           incidentRef,
			"serverProposalId":      serverProposalID,
			"target":                map[string]any{"kind": "DestinationRule", "namespace": "demo", "name": "httpbin"},
			"targetResourceVersion": "10",
			"riskTier":              "Low",
			"patch": []any{
				map[string]any{"op": "replace", "path": "/spec/subsets/1/labels/version", "value": "v2-fixed"},
			},
			"approved": approved,
		},
		"status": map[string]any{
			"phase": "Pending",
		},
	}}
}

func TestResolutionReconcilerSkipsResolutionWithNoOutcomeYet(t *testing.T) {
	// No external system has reported anything yet — the reconciler must not
	// invent an outcome or call talam-server.
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true)
	fake := newFakeDynamicClient(res)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace:       "talam-system",
		Dynamic:         fake,
		OutcomeReporter: &OutcomeReporter{ServerURL: srv.URL, Client: srv.Client(), Log: testLogger()},
		Log:             testLogger(),
	}
	rr.reconcileOnce(context.Background())

	if called {
		t.Fatal("a resolution with no status.outcome must never trigger an outcome report")
	}
}

func TestResolutionReconcilerRelaysExternallyReportedOutcome(t *testing.T) {
	// Simulates an external system (GitOps controller, human via kubectl)
	// having applied the patch and patched status.outcome itself.
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true)
	res.Object["status"] = map[string]any{
		"phase": "Approved", "outcome": "Applied", "appliedBy": "argocd", "detail": "synced",
	}
	fake := newFakeDynamicClient(res)

	var reported []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reported = append(reported, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace:       "talam-system",
		Dynamic:         fake,
		OutcomeReporter: &OutcomeReporter{ServerURL: srv.URL, Client: srv.Client(), Log: testLogger()},
		Log:             testLogger(),
	}
	rr.reconcileOnce(context.Background())

	if len(reported) != 1 || reported[0] != "/v1/proposals/prop-1/outcome" {
		t.Fatalf("expected exactly one outcome report, got %v", reported)
	}

	live := getFakeResolution(t, fake, "prop-1")
	outcomeReported, _, _ := unstructured.NestedBool(live.Object, "status", "outcomeReported")
	phase, _, _ := unstructured.NestedString(live.Object, "status", "phase")
	if !outcomeReported {
		t.Error("outcomeReported should be true after a successful relay")
	}
	if phase != "Applied" {
		t.Errorf("expected phase to be copied from outcome, got %q", phase)
	}
}

func TestResolutionReconcilerRetriesFailedRelayWithoutDuplicating(t *testing.T) {
	// The outcome was recorded locally but the relay to talam-server never
	// succeeded (outcomeReported=false) — simulating a transient network
	// failure on the first attempt. Retrying must not require a fresh
	// outcome from the external system.
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true)
	res.Object["status"] = map[string]any{
		"phase": "Approved", "outcome": "Applied", "outcomeReported": false, "detail": "synced",
	}
	fake := newFakeDynamicClient(res)

	var reported []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reported = append(reported, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace:       "talam-system",
		Dynamic:         fake,
		OutcomeReporter: &OutcomeReporter{ServerURL: srv.URL, Client: srv.Client(), Log: testLogger()},
		Log:             testLogger(),
	}
	rr.reconcileOnce(context.Background())

	if len(reported) != 1 || reported[0] != "/v1/proposals/prop-1/outcome" {
		t.Fatalf("expected exactly one outcome report retry, got %v", reported)
	}

	live := getFakeResolution(t, fake, "prop-1")
	outcomeReported, _, _ := unstructured.NestedBool(live.Object, "status", "outcomeReported")
	if !outcomeReported {
		t.Error("outcomeReported should be true after a successful retry")
	}
}

func TestResolutionReconcilerSkipsAlreadyReportedOutcome(t *testing.T) {
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true)
	res.Object["status"] = map[string]any{"phase": "Applied", "outcome": "Applied", "outcomeReported": true}
	fake := newFakeDynamicClient(res)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace:       "talam-system",
		Dynamic:         fake,
		OutcomeReporter: &OutcomeReporter{ServerURL: srv.URL, Client: srv.Client(), Log: testLogger()},
		Log:             testLogger(),
	}
	rr.reconcileOnce(context.Background())

	if called {
		t.Fatal("a resolution with outcomeReported=true must not be re-reported")
	}
}

func getFakeResolution(t *testing.T, fake dynamic.Interface, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := fake.Resource(gvrMeshResolution).Namespace("talam-system").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}
