package operator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestMeshDiagnostics(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "talam.dev/v1alpha1",
		"kind":       "MeshDiagnostics",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"serverEndpoint": "http://talam-server:8443", "clusterName": name},
	}}
}

func newTestReconciler(t *testing.T) (*Reconciler, *fake.Clientset) {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvrMeshDiagnostics: "MeshDiagnosticsList",
	})
	core := fake.NewSimpleClientset()
	return &Reconciler{Dynamic: dyn, Core: core, Namespace: "talam-system", Log: testLogger()}, core
}

// grants a set of (group, resource, verb) tuples a set of rbacv1.PolicyRule
// covers — used to assert the agent RBAC this operator creates actually
// includes what internal/agent's CRD Sync / ResolutionReconciler /
// IncidentReconciler call, per ADR-0005. A drift here manifests at runtime
// as a "forbidden" error, which is easy to miss without this check — see PR
// history for the RBAC gap this test was added to catch.
func grants(rules []rbacv1.PolicyRule, group, resource, verb string) bool {
	for _, r := range rules {
		hasGroup, hasResource, hasVerb := false, false, false
		for _, g := range r.APIGroups {
			if g == group {
				hasGroup = true
			}
		}
		for _, res := range r.Resources {
			if res == resource {
				hasResource = true
			}
		}
		for _, v := range r.Verbs {
			if v == verb {
				hasVerb = true
			}
		}
		if hasGroup && hasResource && hasVerb {
			return true
		}
	}
	return false
}

func TestEnsureRBACGrantsWhatCRDSyncAndReconcilersNeed(t *testing.T) {
	rec, core := newTestReconciler(t)
	if err := rec.reconcileOne(context.Background(), newTestMeshDiagnostics("kind-local")); err != nil {
		t.Fatalf("reconcileOne failed: %v", err)
	}

	// The agent Role, namespaced in talam-system — everything CRDSync (spec
	// updates, create) and the two reconcilers (status patches) call.
	role, err := core.RbacV1().Roles("talam-system").Get(context.Background(), "talam-agent-kind-local", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent Role not created: %v", err)
	}
	needed := []struct{ group, resource, verb string }{
		{"talam.dev", "meshincidents", "get"},
		{"talam.dev", "meshincidents", "create"},
		{"talam.dev", "meshincidents", "update"},
		{"talam.dev", "meshincidents", "patch"},
		{"talam.dev", "meshresolutions", "get"},
		{"talam.dev", "meshresolutions", "create"},
		{"talam.dev", "meshresolutions", "update"},
		{"talam.dev", "meshresolutions", "patch"},
		{"talam.dev", "meshincidents/status", "patch"},
		{"talam.dev", "meshresolutions/status", "patch"},
	}
	for _, n := range needed {
		if !grants(role.Rules, n.group, n.resource, n.verb) {
			t.Errorf("agent Role missing grant: apiGroup=%q resource=%q verb=%q", n.group, n.resource, n.verb)
		}
	}

	// The agent ClusterRole — Istio access CRDSync/reconcilers don't touch,
	// but the scan/apply path (internal/agent/engine.go, applier.go) does.
	cr, err := core.RbacV1().ClusterRoles().Get(context.Background(), "talam-agent-kind-local", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("agent ClusterRole not created: %v", err)
	}
	for _, n := range []struct{ group, resource, verb string }{
		{"networking.istio.io", "destinationrules", "patch"},
		{"", "pods", "get"},
	} {
		if !grants(cr.Rules, n.group, n.resource, n.verb) {
			t.Errorf("agent ClusterRole missing grant: apiGroup=%q resource=%q verb=%q", n.group, n.resource, n.verb)
		}
	}
}
