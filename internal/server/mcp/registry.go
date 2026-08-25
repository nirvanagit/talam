package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var gvrMCPServer = schema.GroupVersionResource{Group: "talam.dev", Version: "v1alpha1", Resource: "mcpservers"}

// Registry polls MCPServer objects in one namespace and keeps a connected
// Client for each — see ADR-0006. Poll-based, same pattern as
// internal/server/llm.BindingResolver, for the same reason: avoid pulling in
// a full informer for a handful of objects that change rarely.
type Registry struct {
	Dynamic   dynamic.Interface
	Core      kubernetes.Interface
	Namespace string
	Log       *slog.Logger

	mu      sync.RWMutex
	clients map[string]*Client // by toolset ("metrics", "mesh", ...); last-registered wins if duplicates
	conns   map[string]connKey // by toolset — what the current client was connected with, to detect drift
}

// connKey is everything about an MCPServer that requires a reconnect if it
// changes — including the resolved token, so rotating a Secret's value (not
// just its name) is caught, not only a change to the object's own fields.
type connKey struct {
	name     string
	endpoint string
	token    string
}

func NewRegistry(dyn dynamic.Interface, core kubernetes.Interface, namespace string, log *slog.Logger) *Registry {
	return &Registry{Dynamic: dyn, Core: core, Namespace: namespace, Log: log, clients: map[string]*Client{}, conns: map[string]connKey{}}
}

// Close closes every currently connected client. Call once on shutdown —
// Start's ctx.Done() stops polling for new state but doesn't close sessions
// already open.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.clients {
		_ = c.Close()
	}
	r.clients = map[string]*Client{}
	r.conns = map[string]connKey{}
}

// ClientFor returns the connected client for a toolset, or false if none is
// currently registered/reachable.
func (r *Registry) ClientFor(toolset string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[toolset]
	return c, ok
}

// CallTool looks up the client for toolset and calls tool on it — the single
// method internal/server/enrich needs, kept this narrow so that package can
// depend on an interface instead of *Registry (easy to fake in tests).
func (r *Registry) CallTool(ctx context.Context, toolset, tool string, args map[string]any) (string, error) {
	client, ok := r.ClientFor(toolset)
	if !ok {
		return "", fmt.Errorf("no MCPServer registered for toolset %q", toolset)
	}
	return client.CallTool(ctx, tool, args)
}

func (r *Registry) Start(ctx context.Context, interval time.Duration) {
	r.refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

func (r *Registry) refresh(ctx context.Context) {
	if r.Dynamic == nil {
		return
	}
	list, err := r.Dynamic.Resource(gvrMCPServer).Namespace(r.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Warn("MCPServer list failed", "err", err)
		}
		return
	}

	next := map[string]*Client{}
	nextConns := map[string]connKey{}
	for i := range list.Items {
		obj := &list.Items[i]
		toolset, _, _ := unstructured.NestedString(obj.Object, "spec", "toolset")
		endpoint, _, _ := unstructured.NestedString(obj.Object, "spec", "endpoint")
		if toolset == "" || endpoint == "" {
			continue
		}

		token, err := r.authToken(ctx, obj)
		if err != nil {
			r.Log.Error("MCPServer auth lookup failed", "server", obj.GetName(), "err", err)
			continue
		}
		want := connKey{name: obj.GetName(), endpoint: endpoint, token: token}

		r.mu.RLock()
		existing, hasExisting := r.clients[toolset]
		existingKey, hasKey := r.conns[toolset]
		r.mu.RUnlock()
		if hasExisting && hasKey && existingKey == want {
			if prev, dup := next[toolset]; dup {
				r.Log.Warn("two MCPServer objects share a toolset; keeping the last one seen", "toolset", toolset, "kept", obj.GetName())
				_ = prev.Close()
			}
			next[toolset] = existing // unchanged: name, endpoint, and resolved token all match
			nextConns[toolset] = want
			continue
		}

		client, err := Connect(ctx, obj.GetName(), endpoint, token)
		if err != nil {
			r.Log.Error("MCPServer connect failed", "server", obj.GetName(), "toolset", toolset, "err", err)
			r.setStatus(ctx, obj.GetName(), false, err.Error(), nil)
			continue
		}
		tools, err := client.ListToolNames(ctx)
		if err != nil {
			r.Log.Warn("MCPServer tool listing failed", "server", obj.GetName(), "err", err)
		}
		r.setStatus(ctx, obj.GetName(), true, "connected", tools)
		r.Log.Info("MCP server connected", "server", obj.GetName(), "toolset", toolset, "tools", tools)

		if prev, dup := next[toolset]; dup {
			r.Log.Warn("two MCPServer objects share a toolset; keeping the last one seen", "toolset", toolset, "kept", obj.GetName())
			_ = prev.Close()
		}
		next[toolset] = client
		nextConns[toolset] = want
	}

	r.mu.Lock()
	old := r.clients
	r.clients = next
	r.conns = nextConns
	r.mu.Unlock()
	for toolset, c := range old {
		if next[toolset] != c {
			_ = c.Close()
		}
	}
}

func (r *Registry) authToken(ctx context.Context, obj *unstructured.Unstructured) (string, error) {
	name, _, _ := unstructured.NestedString(obj.Object, "spec", "authSecretRef", "name")
	if name == "" {
		return "", nil
	}
	key, _, _ := unstructured.NestedString(obj.Object, "spec", "authSecretRef", "key")
	if key == "" {
		key = "token"
	}
	if r.Core == nil {
		return "", fmt.Errorf("authSecretRef set but no Kubernetes client available")
	}
	sec, err := r.Core.CoreV1().Secrets(obj.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(sec.Data[key]), nil
}

func (r *Registry) setStatus(ctx context.Context, name string, ready bool, message string, tools []string) {
	patch := map[string]any{
		"status": map[string]any{
			"ready":          ready,
			"message":        message,
			"availableTools": tools,
			"lastCheckedAt":  time.Now().UTC().Format(time.RFC3339),
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return
	}
	_, _ = r.Dynamic.Resource(gvrMCPServer).Namespace(r.Namespace).
		Patch(ctx, name, types.MergePatchType, body, metav1.PatchOptions{}, "status")
}
