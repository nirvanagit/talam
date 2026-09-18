package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

func newTestServer(t *testing.T, incidents []api.Incident, proposals []api.RemediationProposal) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/incidents":
			_ = json.NewEncoder(w).Encode(incidents)
		case "/v1/proposals":
			_ = json.NewEncoder(w).Encode(proposals)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCRDSyncCreatesIncidentAndResolution(t *testing.T) {
	inc := api.Incident{
		ID:          "inc-1",
		Fingerprint: "fp1",
		State:       api.IncidentOpen,
		Findings:    []mesh.Finding{{AnalyzerID: "a", Cluster: "kind-local", Resource: mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"}}},
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Explanation: "root cause",
	}
	prop := api.RemediationProposal{
		ID:                    "prop-1",
		IncidentID:            "inc-1",
		Cluster:               "kind-local",
		Target:                mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
		TargetResourceVersion: "10",
		RiskTier:              api.RiskLow,
		Patch:                 []api.JSONPatchOp{{Op: "remove", Path: "/spec/subsets/1"}},
		State:                 api.ProposalApproved,
	}
	srv := newTestServer(t, []api.Incident{inc}, []api.RemediationProposal{prop})
	fake := newFakeDynamicClient()
	sync := &CRDSync{ServerURL: srv.URL, Cluster: "kind-local", Namespace: "talam-system", Dynamic: fake, Client: srv.Client(), Log: testLogger()}

	sync.syncOnce(context.Background())

	liveInc, err := fake.Resource(gvrMeshIncident).Namespace("talam-system").Get(context.Background(), "inc-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected MeshIncident created: %v", err)
	}
	explanation, _, _ := unstructured.NestedString(liveInc.Object, "status", "explanation")
	if explanation != "root cause" {
		t.Errorf("expected synced explanation, got %q", explanation)
	}

	liveRes, err := fake.Resource(gvrMeshResolution).Namespace("talam-system").Get(context.Background(), "prop-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected MeshResolution created: %v", err)
	}
	approved, _, _ := unstructured.NestedBool(liveRes.Object, "spec", "approved")
	if !approved {
		t.Error("an Approved proposal must sync to spec.approved = true")
	}
	incidentRef, _, _ := unstructured.NestedString(liveRes.Object, "spec", "incidentRef")
	if incidentRef != "inc-1" {
		t.Errorf("expected incidentRef=inc-1, got %q", incidentRef)
	}
}

func TestCRDSyncDoesNotApprovePendingOrRejected(t *testing.T) {
	for _, state := range []api.ProposalState{api.ProposalPending, api.ProposalRejected} {
		prop := api.RemediationProposal{
			ID: "prop-x", IncidentID: "inc-1", Cluster: "kind-local",
			Target: mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
			State:  state,
		}
		srv := newTestServer(t, nil, []api.RemediationProposal{prop})
		fake := newFakeDynamicClient()
		sync := &CRDSync{ServerURL: srv.URL, Cluster: "kind-local", Namespace: "talam-system", Dynamic: fake, Client: srv.Client(), Log: testLogger()}
		sync.syncOnce(context.Background())

		live, err := fake.Resource(gvrMeshResolution).Namespace("talam-system").Get(context.Background(), "prop-x", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		approved, _, _ := unstructured.NestedBool(live.Object, "spec", "approved")
		if approved {
			t.Errorf("state %s must not sync to spec.approved = true", state)
		}
	}
}

func TestCRDSyncUpdatesPhaseUntilOutcomeReported(t *testing.T) {
	// A resolution created while Pending, whose proposal is later Rejected
	// server-side, must have its status.phase updated to Rejected — nothing
	// else ever touches phase for an object no external system has reported
	// an outcome for.
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", false)
	res.Object["status"] = map[string]any{"phase": "Pending"}
	fake := newFakeDynamicClient(res)

	prop := api.RemediationProposal{ID: "prop-1", IncidentID: "inc-1", Cluster: "kind-local", Target: mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"}, State: api.ProposalRejected}
	srv := newTestServer(t, nil, []api.RemediationProposal{prop})
	sync := &CRDSync{ServerURL: srv.URL, Cluster: "kind-local", Namespace: "talam-system", Dynamic: fake, Client: srv.Client(), Log: testLogger()}
	sync.syncOnce(context.Background())

	live, err := fake.Resource(gvrMeshResolution).Namespace("talam-system").Get(context.Background(), "prop-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	phase, _, _ := unstructured.NestedString(live.Object, "status", "phase")
	if phase != "Rejected" {
		t.Fatalf("expected status.phase synced to Rejected, got %q", phase)
	}
}

func TestCRDSyncDoesNotClobberLocalResolutionStatus(t *testing.T) {
	// Simulates: an external system already reported an outcome for this
	// proposal and set status.outcome = Applied, but the relay to
	// talam-server hasn't landed yet, so the server still reports it as
	// Approved (not yet Applied). Sync must not overwrite the local outcome.
	res := newFakeMeshResolution("talam-system", "prop-1", "inc-1", "prop-1", true)
	res.Object["status"] = map[string]any{"phase": "Applied", "outcome": "Applied"}
	fake := newFakeDynamicClient(res)

	prop := api.RemediationProposal{ID: "prop-1", IncidentID: "inc-1", Cluster: "kind-local", Target: mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"}, State: api.ProposalApproved}
	srv := newTestServer(t, nil, []api.RemediationProposal{prop})
	sync := &CRDSync{ServerURL: srv.URL, Cluster: "kind-local", Namespace: "talam-system", Dynamic: fake, Client: srv.Client(), Log: testLogger()}
	sync.syncOnce(context.Background())

	live, err := fake.Resource(gvrMeshResolution).Namespace("talam-system").Get(context.Background(), "prop-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, _ := unstructured.NestedString(live.Object, "status", "outcome")
	if outcome != "Applied" {
		t.Fatal("sync must not overwrite status on an existing MeshResolution — status is owned by ResolutionReconciler after an outcome is reported")
	}
}
