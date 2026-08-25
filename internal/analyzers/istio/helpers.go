// Package istio holds the v0.1 Istio analyzer set. Each type here is one row
// of docs/api/analyzer-catalog.md and one implementation of mesh.Analyzer.
package istio

import (
	"fmt"
	"strings"

	"github.com/nirvanagit/talam/internal/istiostate"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// istioState extracts the backend-specific state from a snapshot, or errors if
// the snapshot was built for a different backend.
func istioState(snap *mesh.MeshSnapshot) (*istiostate.State, error) {
	st, ok := snap.BackendState.(*istiostate.State)
	if !ok || st == nil {
		return nil, fmt.Errorf("snapshot has no istio state (backend=%q)", snap.Backend)
	}
	return st, nil
}

// parseHost splits an Istio host reference into (service, namespace).
// Accepts "reviews", "reviews.default", "reviews.default.svc.cluster.local".
// Returns ok=false for hosts that don't look like in-cluster services
// (wildcards, external domains).
func parseHost(host, defaultNamespace string) (svc, ns string, ok bool) {
	if host == "" || strings.HasPrefix(host, "*") {
		return "", "", false
	}
	host = strings.TrimSuffix(host, ".svc.cluster.local")
	parts := strings.Split(host, ".")
	switch len(parts) {
	case 1:
		return parts[0], defaultNamespace, true
	case 2:
		return parts[0], parts[1], true
	default:
		// Anything deeper that wasn't a cluster-local FQDN is an external
		// host (e.g. api.example.com) — not resolvable as a Service.
		return "", "", false
	}
}

// findService looks a (name, namespace) pair up in core state.
func findService(core *mesh.CoreState, name, namespace string) *mesh.Service {
	for i := range core.Services {
		s := &core.Services[i]
		if s.Name == name && s.Namespace == namespace {
			return s
		}
	}
	return nil
}

// serviceEntryCovers reports whether any ServiceEntry declares the given host.
// ServiceEntry hosts may be wildcards like "*.example.com".
func serviceEntryCovers(st *istiostate.State, host string) bool {
	for _, se := range st.ServiceEntries {
		for _, h := range se.Hosts {
			if h == host {
				return true
			}
			if suffix, isWild := strings.CutPrefix(h, "*"); isWild && strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

// labelsMatch reports whether pod labels satisfy every selector label.
// An empty selector matches nothing here — callers deal with that case
// explicitly, because "no selector" means different things per resource.
func labelsMatch(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// readyPodsMatching counts ready pods in a namespace matching a selector.
// namespace "" means all namespaces (Gateway selectors are cluster-wide).
func readyPodsMatching(core *mesh.CoreState, namespace string, selector map[string]string) int {
	n := 0
	for _, p := range core.Pods {
		if !p.Ready {
			continue
		}
		if namespace != "" && p.Namespace != namespace {
			continue
		}
		if labelsMatch(selector, p.Labels) {
			n++
		}
	}
	return n
}

// merged returns a copy of base with overlay applied on top.
func merged(base, overlay map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// All returns the registered v0.1 Istio analyzer set.
func All() []mesh.Analyzer {
	return []mesh.Analyzer{
		OrphanedSubset{},
		DanglingHost{},
		UnboundGateway{},
	}
}
