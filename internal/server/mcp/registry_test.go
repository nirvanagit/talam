package mcp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newCountingMCPServer starts a real MCP server over real HTTP and returns
// it plus a counter of how many sessions have been initialized — used to
// prove whether Registry actually reconnects or reuses a session.
func newCountingMCPServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	connects := 0
	s := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "v0"}, nil)
	sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "ping"}, func(context.Context, *sdkmcp.CallToolRequest, struct{}) (*sdkmcp.CallToolResult, any, error) {
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong"}}}, nil, nil
	})
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		connects++
		return s
	}, nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, &connects
}

func newFakeMCPServerObj(namespace, name, toolset, endpoint string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "talam.dev/v1alpha1",
		"kind":       "MCPServer",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"toolset": toolset, "endpoint": endpoint},
	}}
}

func newRegistryFakeDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvrMCPServer: "MCPServerList",
	}, objs...)
}

// One Registry.Connect makes more than one HTTP request to the MCP server
// (an initialize POST plus a standalone SSE GET by default), so
// newCountingMCPServer's counter increments by some SDK-internal constant
// per session, not exactly one. Tests below compare deltas across refreshes
// rather than asserting an absolute request count.

func TestRegistryReusesConnectionWhenNothingChanged(t *testing.T) {
	srv, connects := newCountingMCPServer(t)
	obj := newFakeMCPServerObj("talam-system", "mesh", "mesh", srv.URL)
	dyn := newRegistryFakeDynamicClient(obj)
	r := NewRegistry(dyn, fake.NewSimpleClientset(), "talam-system", testLogger())
	t.Cleanup(r.Close)

	r.refresh(context.Background())
	afterFirst := *connects
	if afterFirst == 0 {
		t.Fatal("expected the first refresh to connect at all")
	}

	r.refresh(context.Background())
	r.refresh(context.Background())

	if *connects != afterFirst {
		t.Fatalf("expected no new HTTP requests across 2 unchanged refreshes (reuse the existing session), went from %d to %d", afterFirst, *connects)
	}
	if _, ok := r.ClientFor("mesh"); !ok {
		t.Fatal("expected a client registered for toolset mesh")
	}
}

func TestRegistryReconnectsWhenEndpointChanges(t *testing.T) {
	srv1, connects1 := newCountingMCPServer(t)
	srv2, connects2 := newCountingMCPServer(t)
	obj := newFakeMCPServerObj("talam-system", "mesh", "mesh", srv1.URL)
	dyn := newRegistryFakeDynamicClient(obj)
	r := NewRegistry(dyn, fake.NewSimpleClientset(), "talam-system", testLogger())
	t.Cleanup(r.Close)

	r.refresh(context.Background())
	if *connects1 == 0 {
		t.Fatal("expected a connection to srv1")
	}
	if *connects2 != 0 {
		t.Fatalf("srv2 shouldn't have been contacted yet, got %d", *connects2)
	}

	// Simulate the object's endpoint changing (e.g. a human edited it).
	updated := newFakeMCPServerObj("talam-system", "mesh", "mesh", srv2.URL)
	if err := dyn.Resource(gvrMCPServer).Namespace("talam-system").Delete(context.Background(), "mesh", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := dyn.Resource(gvrMCPServer).Namespace("talam-system").Create(context.Background(), updated, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	r.refresh(context.Background())
	if *connects2 == 0 {
		t.Fatal("expected the registry to connect to the new endpoint after spec.endpoint changed")
	}
}

func TestRegistryReconnectsWhenTokenRotates(t *testing.T) {
	srv, connects := newCountingMCPServer(t)
	obj := newFakeMCPServerObj("talam-system", "mesh", "mesh", srv.URL)
	unstructured.SetNestedMap(obj.Object, map[string]any{"name": "creds", "key": "token"}, "spec", "authSecretRef")
	dyn := newRegistryFakeDynamicClient(obj)

	core := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "talam-system"},
		Data:       map[string][]byte{"token": []byte("v1")},
	})
	r := NewRegistry(dyn, core, "talam-system", testLogger())
	t.Cleanup(r.Close)

	r.refresh(context.Background())
	afterFirst := *connects
	if afterFirst == 0 {
		t.Fatal("expected an initial connection")
	}

	sec, err := core.CoreV1().Secrets("talam-system").Get(context.Background(), "creds", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sec.Data["token"] = []byte("v2-rotated")
	if _, err := core.CoreV1().Secrets("talam-system").Update(context.Background(), sec, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	r.refresh(context.Background())
	if *connects == afterFirst {
		t.Fatal("expected a reconnect once the Secret's token value changed, but no new requests were made")
	}
}

func TestRegistryClosesDuplicateToolsetWithinOneRefresh(t *testing.T) {
	srvA, _ := newCountingMCPServer(t)
	srvB, _ := newCountingMCPServer(t)
	objA := newFakeMCPServerObj("talam-system", "a", "mesh", srvA.URL)
	objB := newFakeMCPServerObj("talam-system", "b", "mesh", srvB.URL)
	dyn := newRegistryFakeDynamicClient(objA, objB)
	r := NewRegistry(dyn, fake.NewSimpleClientset(), "talam-system", testLogger())
	t.Cleanup(r.Close)

	r.refresh(context.Background())

	// Exactly one client should survive registered under "mesh" — the other
	// must have been closed, not leaked. We can't directly assert Close()
	// was called without instrumenting Client, but we can assert the
	// registry only kept one and didn't panic/leave duplicate bookkeeping.
	client, ok := r.ClientFor("mesh")
	if !ok || client == nil {
		t.Fatal("expected exactly one client registered for the shared toolset")
	}
}

func TestRegistryClose(t *testing.T) {
	srv, _ := newCountingMCPServer(t)
	obj := newFakeMCPServerObj("talam-system", "mesh", "mesh", srv.URL)
	dyn := newRegistryFakeDynamicClient(obj)
	r := NewRegistry(dyn, fake.NewSimpleClientset(), "talam-system", testLogger())

	r.refresh(context.Background())
	if _, ok := r.ClientFor("mesh"); !ok {
		t.Fatal("expected a client before Close")
	}

	r.Close()
	if _, ok := r.ClientFor("mesh"); ok {
		t.Fatal("expected no clients registered after Close")
	}
}

func TestRegistryCallToolNoServerRegistered(t *testing.T) {
	dyn := newRegistryFakeDynamicClient()
	r := NewRegistry(dyn, fake.NewSimpleClientset(), "talam-system", testLogger())
	_, err := r.CallTool(context.Background(), "mesh", "get_destination_rule", nil)
	if err == nil {
		t.Fatal("expected an error calling a tool on an unregistered toolset")
	}
}
