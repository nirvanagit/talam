# Analyzer interface

**Related:** reads [`ADR-0002`](../decisions/0002-deterministic-analyzers-then-llm.md), [`ADR-0004`](../decisions/0004-mesh-agnostic-analyzer-interface.md); read by [`../components/agent/README.md`](../components/agent/README.md), [`../api/analyzer-catalog.md`](../api/analyzer-catalog.md); links to [`finding-and-incident.md`](finding-and-incident.md)

An `Analyzer` is the unit of detection in talam. It reads a point-in-time snapshot of mesh state and returns facts — no network calls, no LLM calls, nothing but pure detection logic, which is what keeps it unit-testable against fixture snapshots ([ADR-0002](../decisions/0002-deterministic-analyzers-then-llm.md)).

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

The interface has no Istio-specific types anywhere in its signature — `Backend()` is a declared string tag, not a type parameter, so the [agent](../components/agent/README.md)'s engine can run analyzers for any mesh backend through the same loop ([ADR-0004](../decisions/0004-mesh-agnostic-analyzer-interface.md)). A `MeshSnapshot` is built once per scan tick from that backend's collectors and handed to every registered analyzer for that backend.

`Analyze` returns [`Finding`](finding-and-incident.md) objects — see that doc for the shape and for what happens to a Finding after an analyzer produces it.

## Concrete implementations

The v0.1 Istio analyzer set is documented as data, not code, in [`../api/analyzer-catalog.md`](../api/analyzer-catalog.md) — each row there is one type implementing this interface.
