package meshmcp

import (
	"context"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newFakeDynamicClient(objs ...runtime.Object) dynamic.Interface {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvrDestinationRules:    "DestinationRuleList",
		gvrServiceEntries:      "ServiceEntryList",
		gvrPeerAuthentications: "PeerAuthenticationList",
	}, objs...)
}

// connect wires a real MCP client to the server over an in-memory transport
// — this exercises the actual wire protocol (tool schemas, JSON-RPC framing),
// not just calling Go methods directly.
func connect(t *testing.T, dyn dynamic.Interface) *mcp.ClientSession {
	t.Helper()
	s := New(dyn)
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := s.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := c.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func newDestinationRule(namespace, name, host string, subsets ...map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.istio.io/v1",
		"kind":       "DestinationRule",
		"metadata":   map[string]any{"namespace": namespace, "name": name},
		"spec":       map[string]any{"host": host, "subsets": toAnySlice(subsets)},
	}}
}

func toAnySlice(m []map[string]any) []any {
	out := make([]any, len(m))
	for i, v := range m {
		out[i] = v
	}
	return out
}

func TestGetDestinationRule(t *testing.T) {
	dyn := newFakeDynamicClient(newDestinationRule("demo", "httpbin", "httpbin",
		map[string]any{"name": "v1", "labels": map[string]any{"version": "v1"}}))
	cs := connect(t, dyn)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_destination_rule",
		Arguments: map[string]any{"namespace": "demo", "name": "httpbin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var spec map[string]any
	if err := json.Unmarshal([]byte(text), &spec); err != nil {
		t.Fatalf("tool result wasn't valid JSON: %v (%s)", err, text)
	}
	if spec["host"] != "httpbin" {
		t.Errorf("expected host=httpbin, got %+v", spec)
	}
}

func TestGetDestinationRuleNotFoundReturnsErrorResult(t *testing.T) {
	dyn := newFakeDynamicClient()
	cs := connect(t, dyn)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_destination_rule",
		Arguments: map[string]any{"namespace": "demo", "name": "nonexistent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected an error result for a nonexistent DestinationRule, not a protocol error nor a silent success")
	}
}

func TestListServiceEntries(t *testing.T) {
	se := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.istio.io/v1",
		"kind":       "ServiceEntry",
		"metadata":   map[string]any{"namespace": "demo", "name": "external-api"},
		"spec":       map[string]any{"hosts": []any{"api.example.com"}},
	}}
	dyn := newFakeDynamicClient(se)
	cs := connect(t, dyn)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_service_entries",
		Arguments: map[string]any{"namespace": "demo"},
	})
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v content=%v", err, res.IsError, res.Content)
	}
	var out []map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0]["name"] != "external-api" {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestGetMTLSStatus(t *testing.T) {
	pa := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "security.istio.io/v1",
		"kind":       "PeerAuthentication",
		"metadata":   map[string]any{"namespace": "demo", "name": "default"},
		"spec":       map[string]any{"mtls": map[string]any{"mode": "STRICT"}},
	}}
	dr := newDestinationRule("demo", "httpbin", "httpbin")
	dyn := newFakeDynamicClient(pa, dr)
	cs := connect(t, dyn)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_mtls_status",
		Arguments: map[string]any{"namespace": "demo"},
	})
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v content=%v", err, res.IsError, res.Content)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	peerAuths, _ := out["peerAuthentications"].([]any)
	if len(peerAuths) != 1 {
		t.Fatalf("expected 1 PeerAuthentication, got %+v", out)
	}
	first := peerAuths[0].(map[string]any)
	if first["mode"] != "STRICT" {
		t.Errorf("expected mode=STRICT, got %+v", first)
	}
}
