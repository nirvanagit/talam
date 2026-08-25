# ADR-0004: Analyzer interface is mesh-agnostic; Istio ships as the first backend

**Status:** Accepted
**Related:** reads [`../concepts/analyzer-interface.md`](../concepts/analyzer-interface.md)

## Context

Istio is the largest install base and the richest CRD surface to build detection against, so it's the obvious first target. But service mesh isn't one technology — Linkerd has a much smaller CRD surface, and ambient/sidecar-less meshes like Cilium have no per-pod Envoy to inspect at all. Baking Istio's object model into the core engine would mean a rewrite, not an extension, to support a second mesh later.

## Decision

The `Analyzer` interface, the `MeshSnapshot` it reads, and the `Finding` it returns are defined without any Istio-specific types at the core-engine level. Each analyzer declares a `Backend()` (`istio`, and later `linkerd`, `cilium-ambient`, ...); collectors are backend-specific and populate the snapshot, but the engine, the LLM gateway, and the remediation broker never branch on which mesh produced a finding. See [`../concepts/analyzer-interface.md`](../concepts/analyzer-interface.md) for the interface definition and [`../api/analyzer-catalog.md`](../api/analyzer-catalog.md) for the Istio-specific analyzers that are the v0.1 concrete implementation of it.

## Consequences

Adding Linkerd or Cilium-ambient support later is additive — new collectors and analyzers behind the existing interface — not a rewrite of the server, remediation broker, or CLI. The cost paid now is designing the interface a level more abstract than "Istio CRDs," which takes more upfront thought than hard-coding Istio types would have.
