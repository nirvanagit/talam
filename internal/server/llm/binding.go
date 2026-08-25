package llm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/nirvanagit/talam/api/v1alpha1"
)

var gvrModelBinding = schema.GroupVersionResource{
	Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "modelbindings",
}

// BindingResolver watches a ModelBinding custom resource and builds the
// Provider it describes — the Kubernetes-native way to pick which LLM
// talam-server uses, in place of (or on top of) TALAM_LLM_MODEL /
// ANTHROPIC_API_KEY env vars. Poll-based rather than a watch stream to avoid
// pulling in a full informer/controller-runtime dependency for one object.
type BindingResolver struct {
	Dynamic   dynamic.Interface
	Core      kubernetes.Interface
	Namespace string
	Name      string
	Log       *slog.Logger

	current  Provider
	fallback Provider
}

// NewBindingResolver returns a resolver that falls back to fallback whenever
// no ModelBinding is found — e.g. running outside Kubernetes, or before the
// first binding is created.
func NewBindingResolver(dyn dynamic.Interface, core kubernetes.Interface, namespace, name string, fallback Provider, log *slog.Logger) *BindingResolver {
	return &BindingResolver{Dynamic: dyn, Core: core, Namespace: namespace, Name: name, fallback: fallback, Log: log}
}

// Current returns the most recently resolved provider (or the fallback if no
// binding has ever resolved).
func (r *BindingResolver) Current() Provider {
	if r.current != nil {
		return r.current
	}
	return r.fallback
}

// Start polls the ModelBinding every interval until ctx is cancelled,
// swapping r.current whenever the binding's resolved provider changes.
func (r *BindingResolver) Start(ctx context.Context, interval time.Duration) {
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

func (r *BindingResolver) refresh(ctx context.Context) {
	if r.Dynamic == nil {
		return
	}
	u, err := r.Dynamic.Resource(gvrModelBinding).Namespace(r.Namespace).Get(ctx, r.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if r.current != nil {
			r.Log.Warn("ModelBinding not found, reverting to fallback provider", "name", r.Name)
		}
		r.current = nil
		return
	}
	if err != nil {
		r.Log.Warn("ModelBinding fetch failed, keeping current provider", "err", err)
		return
	}
	p, err := r.build(ctx, u)
	if err != nil {
		r.Log.Error("ModelBinding invalid, keeping current provider", "err", err)
		return
	}
	if r.current == nil || r.current.Name() != p.Name() {
		r.Log.Info("resolved LLM provider from ModelBinding", "provider", p.Name())
	}
	r.current = p
}

func (r *BindingResolver) build(ctx context.Context, u *unstructured.Unstructured) (Provider, error) {
	provider, _, _ := unstructured.NestedString(u.Object, "spec", "provider")
	model, _, _ := unstructured.NestedString(u.Object, "spec", "model")
	baseURL, _, _ := unstructured.NestedString(u.Object, "spec", "baseURL")

	switch provider {
	case "", "anthropic":
		key, err := r.secretKey(ctx, u)
		if err != nil {
			return nil, err
		}
		p := NewAnthropic()
		if key != "" {
			p.APIKey = key
		}
		if model != "" {
			p.Model = model
		}
		if p.APIKey == "" {
			return nil, fmt.Errorf("ModelBinding %q: provider anthropic needs apiKeySecretRef or ANTHROPIC_API_KEY", u.GetName())
		}
		return p, nil
	case "openai-compatible":
		key, err := r.secretKey(ctx, u)
		if err != nil {
			return nil, err
		}
		if baseURL == "" {
			return nil, fmt.Errorf("ModelBinding %q: provider openai-compatible requires baseURL", u.GetName())
		}
		return &OpenAICompatible{BaseURL: baseURL, APIKey: key, Model: model}, nil
	case "claude-cli":
		return NewClaudeCLI(), nil
	default:
		return nil, fmt.Errorf("ModelBinding %q: unknown provider %q", u.GetName(), provider)
	}
}

func (r *BindingResolver) secretKey(ctx context.Context, u *unstructured.Unstructured) (string, error) {
	name, _, _ := unstructured.NestedString(u.Object, "spec", "apiKeySecretRef", "name")
	key, _, _ := unstructured.NestedString(u.Object, "spec", "apiKeySecretRef", "key")
	if name == "" {
		return "", nil
	}
	if key == "" {
		key = "apiKey"
	}
	if r.Core == nil {
		return "", fmt.Errorf("apiKeySecretRef set but no Kubernetes client available")
	}
	sec, err := r.Core.CoreV1().Secrets(u.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("fetching secret %s/%s: %w", u.GetNamespace(), name, err)
	}
	return string(sec.Data[key]), nil
}
