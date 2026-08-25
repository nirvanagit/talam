---
title: "Analyzer interface"
weight: 10
---
**Related:** reads [`ADR-0002`](/docs/decisions/0002-deterministic-analyzers-then-llm/), [`ADR-0004`](/docs/decisions/0004-mesh-agnostic-analyzer-interface/); read by [`../components/agent/README.md`](/docs/components/agent/), [`../api/analyzer-catalog.md`](/docs/api/analyzer-catalog/); links to [`finding-and-incident.md`](/docs/concepts/finding-and-incident/)

An `Analyzer` is the unit of detection in talam. It reads a point-in-time snapshot of mesh state and returns facts — no network calls, no LLM calls, nothing but pure detection logic, which is what keeps it unit-testable against fixture snapshots ([ADR-0002](/docs/decisions/0002-deterministic-analyzers-then-llm/)).

```go
// Analyzer is the unit of detection. Every check in talam implements this.
type Analyzer interface {
    // Unique, stable id — e.g. "istio.destinationrule.orphaned-subset"
    ID() string

    // Which backend this analyzer needs (istio, linkerd, core-k8s...)
    Backend() MeshBackend

    // Cheap watch-driven checks vs. expensive periodic ones
    Trigger() TriggerKind // OnChange | Interval

    // Run against a point-in-time snapshot of collected mesh state
    Analyze(ctx context.Context, snap *MeshSnapshot) ([]Finding, error)
}
```

The interface has no Istio-specific types anywhere in its signature — `Backend()` is a declared string tag, not a type parameter, so the [agent](/docs/components/agent/)'s engine can run analyzers for any mesh backend through the same loop ([ADR-0004](/docs/decisions/0004-mesh-agnostic-analyzer-interface/)). A `MeshSnapshot` is built once per scan tick from that backend's collectors and handed to every registered analyzer for that backend.

`Analyze` returns [`Finding`](/docs/concepts/finding-and-incident/) objects — see that doc for the shape and for what happens to a Finding after an analyzer produces it.

## Concrete implementations

The v0.1 Istio analyzer set is documented as data, not code, in [`../api/analyzer-catalog.md`](/docs/api/analyzer-catalog/) — each row there is one type implementing this interface.
