package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testApplier builds an Applier whose reportOutcome call has somewhere
// harmless to fail — reportOutcome only logs a warning on error, so a
// guaranteed-refused address is enough; tests that care about the outcome
// report itself point ServerURL at an httptest.Server instead.
func testApplier(dyn dynamic.Interface) *Applier {
	return &Applier{
		ServerURL: "http://127.0.0.1:0",
		Dynamic:   dyn,
		Client:    &http.Client{Timeout: time.Second},
		Log:       testLogger(),
	}
}

func newFakeDestinationRule(namespace, name, resourceVersion string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.istio.io/v1",
		"kind":       "DestinationRule",
		"metadata": map[string]any{
			"name":            name,
			"namespace":       namespace,
			"resourceVersion": resourceVersion,
		},
		"spec": map[string]any{
			"host": "httpbin",
			"subsets": []any{
				map[string]any{"name": "v1", "labels": map[string]any{"version": "v1"}},
				map[string]any{"name": "v2", "labels": map[string]any{"version": "v2"}},
			},
		},
	}}
}

func newFakeDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}: "DestinationRuleList",
		gvrMeshIncident:   "MeshIncidentList",
		gvrMeshResolution: "MeshResolutionList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

func TestApplyRejectsUnpatchableKind(t *testing.T) {
	fake := newFakeDynamicClient()
	ap := &Applier{Dynamic: fake, Log: testLogger()}
	p := api.RemediationProposal{Target: mesh.ResourceRef{Kind: "Secret", Namespace: "demo", Name: "creds"}, TargetResourceVersion: "1"}

	outcome := ap.apply(context.Background(), p)
	if outcome.Success {
		t.Fatal("Secret must never be applied — it's not in the patchable allowlist")
	}
	if len(fake.Actions()) != 0 {
		t.Errorf("no API calls should be made for a disallowed kind, got %d", len(fake.Actions()))
	}
}

func TestApplyRejectsMissingResourceVersion(t *testing.T) {
	obj := newFakeDestinationRule("demo", "httpbin", "10")
	fake := newFakeDynamicClient(obj)
	ap := &Applier{Dynamic: fake, Log: testLogger()}
	p := api.RemediationProposal{
		Target: mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
		Patch:  []api.JSONPatchOp{{Op: "remove", Path: "/spec/subsets/1"}},
		// TargetResourceVersion deliberately left empty.
	}
	outcome := ap.apply(context.Background(), p)
	if outcome.Success {
		t.Fatal("a proposal with no TargetResourceVersion must never be applied")
	}
}

func TestApplyRefusesWhenResourceVersionDrifted(t *testing.T) {
	// The live object is at resourceVersion "10", but the proposal was
	// generated from resourceVersion "9" — simulating something else having
	// changed the DestinationRule between propose time and approval.
	obj := newFakeDestinationRule("demo", "httpbin", "10")
	fake := newFakeDynamicClient(obj)
	ap := &Applier{Dynamic: fake, Log: testLogger()}
	p := api.RemediationProposal{
		Target:                mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
		TargetResourceVersion: "9",
		Patch:                 []api.JSONPatchOp{{Op: "remove", Path: "/spec/subsets/1"}},
	}
	outcome := ap.apply(context.Background(), p)
	if outcome.Success {
		t.Fatal("apply must fail when the live resourceVersion no longer matches the proposal's pinned version")
	}

	live, err := fake.Resource(schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}).
		Namespace("demo").Get(context.Background(), "httpbin", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	subsets, _, _ := unstructured.NestedSlice(live.Object, "spec", "subsets")
	if len(subsets) != 2 {
		t.Fatalf("a refused patch must leave the live object untouched, got %d subsets", len(subsets))
	}
}

func TestApplySucceedsWhenResourceVersionMatches(t *testing.T) {
	obj := newFakeDestinationRule("demo", "httpbin", "10")
	fake := newFakeDynamicClient(obj)
	ap := &Applier{Dynamic: fake, Log: testLogger()}
	p := api.RemediationProposal{
		Target:                mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
		TargetResourceVersion: "10",
		// A "replace", not "remove": the fake dynamic client drops DryRun
		// entirely (see dynamicfake's Patch impl — metav1.PatchOptions never
		// reaches a reactor), so apply()'s dry-run call and its real call
		// both mutate the fake's state. An idempotent replace tolerates
		// running twice with the correct end result; a "remove" would error
		// on the second call once the first already removed the element.
		// The real API server's dry run genuinely doesn't persist — see the
		// PR description's live-cluster validation for that end-to-end proof.
		Patch: []api.JSONPatchOp{{Op: "replace", Path: "/spec/subsets/1/labels/version", Value: "v2-fixed"}},
	}
	outcome := ap.apply(context.Background(), p)
	if !outcome.Success {
		t.Fatalf("expected apply to succeed when resourceVersion matches: %s", outcome.Detail)
	}

	live, err := fake.Resource(schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}).
		Namespace("demo").Get(context.Background(), "httpbin", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	subsets, _, _ := unstructured.NestedSlice(live.Object, "spec", "subsets")
	if len(subsets) != 2 {
		t.Fatalf("expected 2 subsets untouched in count, got %d", len(subsets))
	}
	subset1, _ := subsets[1].(map[string]any)
	labels, _ := subset1["labels"].(map[string]any)
	if got := labels["version"]; got != "v2-fixed" {
		t.Fatalf("expected subsets[1].labels.version = v2-fixed, got %v", got)
	}
}
