// Package mesh defines talam's mesh-agnostic core: the Analyzer interface,
// the MeshSnapshot it reads, and the Finding it produces. Per ADR-0004 this
// package must never import mesh-backend-specific types; backend state rides
// in MeshSnapshot.BackendState and is type-asserted by backend analyzers.
package mesh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// MeshBackend tags which mesh implementation an analyzer or snapshot targets.
type MeshBackend string

const (
	BackendIstio MeshBackend = "istio"
)

// TriggerKind distinguishes cheap watch-driven checks from expensive periodic ones.
type TriggerKind string

const (
	TriggerOnChange TriggerKind = "OnChange"
	TriggerInterval TriggerKind = "Interval"
)

// Severity of a Finding.
type Severity string

const (
	SeverityCritical Severity = "Critical"
	SeverityWarning  Severity = "Warning"
	SeverityInfo     Severity = "Info"
)

// ResourceRef identifies a Kubernetes resource a finding is about.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (r ResourceRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.Kind, r.Namespace, r.Name)
}

// Finding is the output of one Analyzer run. RawEvidence is structured facts,
// never prose — the plain-language explanation is the server-side LLM
// gateway's job (ADR-0002).
type Finding struct {
	AnalyzerID  string         `json:"analyzerId"`
	Severity    Severity       `json:"severity"`
	Resource    ResourceRef    `json:"resource"`
	RelatedRefs []ResourceRef  `json:"relatedRefs,omitempty"`
	RawEvidence map[string]any `json:"rawEvidence"`
	DetectedAt  time.Time      `json:"detectedAt"`
	Cluster     string         `json:"cluster"`
}

// Fingerprint identifies "the same problem" across scan ticks so the server
// can deduplicate: same analyzer, same resource, same cluster.
func (f Finding) Fingerprint() string {
	h := sha256.Sum256([]byte(f.Cluster + "|" + f.AnalyzerID + "|" + f.Resource.String()))
	return hex.EncodeToString(h[:8])
}

// CoreState is the mesh-agnostic slice of cluster state every backend collector
// populates: services, pods, and which pods back which service.
type CoreState struct {
	Services []Service `json:"services"`
	Pods     []Pod     `json:"pods"`
}

// Service is a minimal view of a Kubernetes Service.
type Service struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Selector  map[string]string `json:"selector,omitempty"`
	Ports     []int32           `json:"ports,omitempty"`
}

// Pod is a minimal view of a running Pod.
type Pod struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Ready     bool              `json:"ready"`
}

// MeshSnapshot is a point-in-time view of collected mesh state, built once per
// scan tick and handed to every registered analyzer for its backend.
// BackendState holds the backend-specific portion (e.g. *istio.State); the
// engine never inspects it.
type MeshSnapshot struct {
	Backend      MeshBackend `json:"backend"`
	Cluster      string      `json:"cluster"`
	CollectedAt  time.Time   `json:"collectedAt"`
	Core         CoreState   `json:"core"`
	BackendState any         `json:"-"`
}

// Analyzer is the unit of detection. Every check in talam implements this.
// Analyze must be pure detection logic: no network calls, no LLM calls
// (ADR-0002), which is what keeps it unit-testable against fixture snapshots.
type Analyzer interface {
	// ID is a unique, stable id — e.g. "istio.destinationrule.orphaned-subset".
	ID() string

	// Backend declares which mesh backend this analyzer needs.
	Backend() MeshBackend

	// Trigger distinguishes cheap watch-driven checks from expensive periodic ones.
	Trigger() TriggerKind

	// Analyze runs against a point-in-time snapshot of collected mesh state.
	Analyze(ctx context.Context, snap *MeshSnapshot) ([]Finding, error)
}

// MarshalEvidence is a helper for analyzers to build RawEvidence maps from
// typed structs without hand-writing map literals.
func MarshalEvidence(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"marshalError": err.Error()}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"unmarshalError": err.Error()}
	}
	return m
}
