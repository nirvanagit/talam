package agent

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newFakeMeshIncident(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "talam.dev/v1alpha1",
		"kind":       "MeshIncident",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"fingerprint": "abc", "serverIncidentId": name},
		"status":     map[string]any{"state": "Open"},
	}}
}

func withResolutionStatus(res *unstructured.Unstructured, performed bool) *unstructured.Unstructured {
	res.Object["status"] = map[string]any{"performed": performed}
	return res
}

func TestIncidentReconcilerIncompleteWithNoResolutions(t *testing.T) {
	inc := newFakeMeshIncident("talam-system", "inc-1")
	fake := newFakeDynamicClient(inc)
	ir := &IncidentReconciler{Namespace: "talam-system", Dynamic: fake, Log: testLogger()}
	ir.reconcileOnce(context.Background())

	live, err := fake.Resource(gvrMeshIncident).Namespace("talam-system").Get(context.Background(), "inc-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	complete, _, _ := unstructured.NestedBool(live.Object, "status", "complete")
	if complete {
		t.Fatal("an incident with zero resolutions must never be Complete")
	}
}

func TestIncidentReconcilerIncompleteUntilAllPerformed(t *testing.T) {
	inc := newFakeMeshIncident("talam-system", "inc-1")
	r1 := withResolutionStatus(newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10"), true)
	r2 := withResolutionStatus(newFakeMeshResolution("talam-system", "prop-2", "inc-1", "prop-2", true, "10"), false)
	fake := newFakeDynamicClient(inc, r1, r2)
	ir := &IncidentReconciler{Namespace: "talam-system", Dynamic: fake, Log: testLogger()}
	ir.reconcileOnce(context.Background())

	live, err := fake.Resource(gvrMeshIncident).Namespace("talam-system").Get(context.Background(), "inc-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	complete, _, _ := unstructured.NestedBool(live.Object, "status", "complete")
	if complete {
		t.Fatal("an incident with one unperformed resolution must not be Complete")
	}
	refs, _, _ := unstructured.NestedSlice(live.Object, "status", "resolutionRefs")
	if len(refs) != 2 {
		t.Fatalf("expected 2 resolutionRefs, got %d", len(refs))
	}
}

func TestIncidentReconcilerCompleteWhenAllPerformed(t *testing.T) {
	inc := newFakeMeshIncident("talam-system", "inc-1")
	r1 := withResolutionStatus(newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true, "10"), true)
	fake := newFakeDynamicClient(inc, r1)
	ir := &IncidentReconciler{Namespace: "talam-system", Dynamic: fake, Log: testLogger()}
	ir.reconcileOnce(context.Background())

	live, err := fake.Resource(gvrMeshIncident).Namespace("talam-system").Get(context.Background(), "inc-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	complete, _, _ := unstructured.NestedBool(live.Object, "status", "complete")
	if !complete {
		t.Fatal("an incident whose only resolution was performed should be Complete")
	}
}

func TestIncidentReconcilerIgnoresResolutionsForOtherIncidents(t *testing.T) {
	inc := newFakeMeshIncident("talam-system", "inc-1")
	other := withResolutionStatus(newFakeMeshResolution("talam-system", "prop-9", "inc-9", "prop-9", true, "10"), false)
	fake := newFakeDynamicClient(inc, other)
	ir := &IncidentReconciler{Namespace: "talam-system", Dynamic: fake, Log: testLogger()}
	ir.reconcileOnce(context.Background())

	live, err := fake.Resource(gvrMeshIncident).Namespace("talam-system").Get(context.Background(), "inc-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	refs, _, _ := unstructured.NestedSlice(live.Object, "status", "resolutionRefs")
	if len(refs) != 0 {
		t.Fatalf("a resolution for a different incident must not be attributed here, got refs=%v", refs)
	}
}
