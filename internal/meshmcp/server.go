// Package meshmcp is talam-mesh-mcp: a purpose-built, read-only MCP server
// exposing live Istio introspection that no generic MCP server would have —
// see ADR-0006 (docs/decisions/0006-mcp-evidence-enrichment.md). It runs
// per-cluster, next to the agent, with the same kind of scoped read-only
// RBAC (deploy/mesh-mcp/rbac.yaml) — never talam-server's own credentials.
package meshmcp

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	gvrDestinationRules    = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}
	gvrServiceEntries      = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "serviceentries"}
	gvrPeerAuthentications = schema.GroupVersionResource{Group: "security.istio.io", Version: "v1", Resource: "peerauthentications"}
)

// New builds the mesh MCP server, wired to a live cluster via dyn. Every
// tool here is read-only (docs/concepts/security-model.md): the dynamic
// client this is constructed with should carry the same read-only RBAC the
// agent's Istio access already has, nothing more.
func New(dyn dynamic.Interface) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "talam-mesh-mcp", Version: "v0.1.0"}, nil)
	t := &tools{dyn: dyn}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_destination_rule",
		Description: "Return the live spec of one DestinationRule — host and every subset with its labels — for cross-checking against analyzer evidence that may be stale.",
	}, t.getDestinationRule)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_service_entries",
		Description: "List every ServiceEntry in a namespace with its hosts, for checking whether a VirtualService destination that looks dangling is actually covered by one.",
	}, t.listServiceEntries)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_mtls_status",
		Description: "Return every PeerAuthentication in a namespace with its mTLS mode, and every DestinationRule's TLS mode for the same namespace, side by side — the two have to agree for mTLS to actually work.",
	}, t.getMTLSStatus)

	return s
}

type tools struct {
	dyn dynamic.Interface
}

type getDestinationRuleInput struct {
	Namespace string `json:"namespace" jsonschema:"the DestinationRule's namespace"`
	Name      string `json:"name" jsonschema:"the DestinationRule's name"`
}

func (t *tools) getDestinationRule(ctx context.Context, _ *mcp.CallToolRequest, in getDestinationRuleInput) (*mcp.CallToolResult, any, error) {
	obj, err := t.dyn.Resource(gvrDestinationRules).Namespace(in.Namespace).Get(ctx, in.Name, metav1.GetOptions{})
	if err != nil {
		return errResult(fmt.Errorf("get DestinationRule %s/%s: %w", in.Namespace, in.Name, err)), nil, nil
	}
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	return textResult(spec)
}

type listServiceEntriesInput struct {
	Namespace string `json:"namespace" jsonschema:"namespace to list ServiceEntries in"`
}

func (t *tools) listServiceEntries(ctx context.Context, _ *mcp.CallToolRequest, in listServiceEntriesInput) (*mcp.CallToolResult, any, error) {
	list, err := t.dyn.Resource(gvrServiceEntries).Namespace(in.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errResult(fmt.Errorf("list ServiceEntries in %s: %w", in.Namespace, err)), nil, nil
	}
	out := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		hosts, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "hosts")
		out = append(out, map[string]any{"name": item.GetName(), "hosts": hosts})
	}
	return textResult(out)
}

type getMTLSStatusInput struct {
	Namespace string `json:"namespace" jsonschema:"namespace to check mTLS posture in"`
}

func (t *tools) getMTLSStatus(ctx context.Context, _ *mcp.CallToolRequest, in getMTLSStatusInput) (*mcp.CallToolResult, any, error) {
	peerAuths, err := t.dyn.Resource(gvrPeerAuthentications).Namespace(in.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errResult(fmt.Errorf("list PeerAuthentications in %s: %w", in.Namespace, err)), nil, nil
	}
	destRules, err := t.dyn.Resource(gvrDestinationRules).Namespace(in.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return errResult(fmt.Errorf("list DestinationRules in %s: %w", in.Namespace, err)), nil, nil
	}

	var peerAuthModes []map[string]any
	for _, pa := range peerAuths.Items {
		mode, _, _ := unstructured.NestedString(pa.Object, "spec", "mtls", "mode")
		peerAuthModes = append(peerAuthModes, map[string]any{"name": pa.GetName(), "mode": mode})
	}
	var destRuleTLS []map[string]any
	for _, dr := range destRules.Items {
		mode, _, _ := unstructured.NestedString(dr.Object, "spec", "trafficPolicy", "tls", "mode")
		host, _, _ := unstructured.NestedString(dr.Object, "spec", "host")
		destRuleTLS = append(destRuleTLS, map[string]any{"name": dr.GetName(), "host": host, "tlsMode": mode})
	}
	return textResult(map[string]any{"peerAuthentications": peerAuthModes, "destinationRuleTLS": destRuleTLS})
}

func textResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}
