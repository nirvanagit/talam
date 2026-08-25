package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func newFakeMeshResolution(namespace, name, incidentRef, serverProposalID string, triggered bool, targetRV string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "talam.dev/v1alpha1",
		"kind":       "MeshResolution",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"incidentRef":           incidentRef,
			"serverProposalId":      serverProposalID,
			"target":                map[string]any{"kind": "DestinationRule", "namespace": "demo", "name": "httpbin"},
			"targetResourceVersion": targetRV,
			"riskTier":              "Low",
			"patch": []any{
				map[string]any{"op": "replace", "path": "/spec/subsets/1/labels/version", "value": "v2-fixed"},
			},
			"triggered": triggered,
		},
		"status": map[string]any{
			"phase":     "Pending",
			"performed": false,
		},
	}}
}

func TestResolutionReconcilerSkipsUntriggered(t *testing.T) {
	dr := newFakeDestinationRule("demo", "httpbin", "10")
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", false, "10")
	fake := newFakeDynamicClient(dr, res)
	rr := &ResolutionReconciler{
		Namespace: "talam-system",
		Dynamic:   fake,
		Applier:   testApplier(fake),
		Log:       testLogger(),
	}
	rr.reconcileOnce(context.Background())

	live := getFakeResolution(t, fake, "prop-1")
	performed, _, _ := unstructured.NestedBool(live.Object, "status", "performed")
	if performed {
		t.Fatal("an untriggered resolution must never be applied")
	}
}

func TestResolutionReconcilerAppliesTriggeredResolution(t *testing.T) {
	dr := newFakeDestinationRule("demo", "httpbin", "10")
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10")
	fake := newFakeDynamicClient(dr, res)
	rr := &ResolutionReconciler{
		Namespace: "talam-system",
		Dynamic:   fake,
		Applier:   testApplier(fake),
		Log:       testLogger(),
	}
	rr.reconcileOnce(context.Background())

	live := getFakeResolution(t, fake, "prop-1")
	performed, _, _ := unstructured.NestedBool(live.Object, "status", "performed")
	phase, _, _ := unstructured.NestedString(live.Object, "status", "phase")
	if !performed || phase != "Applied" {
		t.Fatalf("expected performed=true phase=Applied, got performed=%v phase=%q", performed, phase)
	}
}

func TestResolutionReconcilerSkipsAlreadyPerformed(t *testing.T) {
	dr := newFakeDestinationRule("demo", "httpbin", "10")
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10")
	res.Object["status"] = map[string]any{"phase": "Applied", "performed": true}
	fake := newFakeDynamicClient(dr, res)
	rr := &ResolutionReconciler{
		Namespace: "talam-system",
		Dynamic:   fake,
		Applier:   testApplier(fake),
		Log:       testLogger(),
	}
	rr.reconcileOnce(context.Background())

	live, err := fake.Resource(schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}).
		Namespace("demo").Get(context.Background(), "httpbin", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	subsets, _, _ := unstructured.NestedSlice(live.Object, "spec", "subsets")
	subset1, _ := subsets[1].(map[string]any)
	labels, _ := subset1["labels"].(map[string]any)
	if labels["version"] != "v2" {
		t.Fatal("an already-performed resolution must not be re-applied")
	}
}

func TestResolutionReconcilerRetriesFailedOutcomeReportWithoutReapplying(t *testing.T) {
	// The resolution was already applied locally (performed=true) but the
	// outcome report to talam-server never succeeded (outcomeReported=false)
	// — simulating a transient network failure on the first attempt.
	dr := newFakeDestinationRule("demo", "httpbin", "10") // still has 2 subsets
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10")
	res.Object["status"] = map[string]any{
		"phase": "Applied", "performed": true, "outcomeReported": false, "detail": "applied",
	}
	fake := newFakeDynamicClient(dr, res)

	var reported []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reported = append(reported, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace: "talam-system",
		Dynamic:   fake,
		Applier:   &Applier{ServerURL: srv.URL, Dynamic: fake, Client: srv.Client(), Log: testLogger()},
		Log:       testLogger(),
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

	// Crucially: the apply must NOT have run again.
	liveDR, err := fake.Resource(schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}).
		Namespace("demo").Get(context.Background(), "httpbin", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	subsets, _, _ := unstructured.NestedSlice(liveDR.Object, "spec", "subsets")
	if len(subsets) != 2 {
		t.Fatal("a resolution that's already performed must never trigger a second apply, even while retrying the outcome report")
	}
}

func TestResolutionReconcilerSkipsAlreadyReportedOutcome(t *testing.T) {
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10")
	res.Object["status"] = map[string]any{"phase": "Applied", "performed": true, "outcomeReported": true}
	fake := newFakeDynamicClient(res)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rr := &ResolutionReconciler{
		Namespace: "talam-system",
		Dynamic:   fake,
		Applier:   &Applier{ServerURL: srv.URL, Dynamic: fake, Client: srv.Client(), Log: testLogger()},
		Log:       testLogger(),
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
