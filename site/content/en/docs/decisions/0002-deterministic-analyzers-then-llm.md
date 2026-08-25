---
title: "ADR-0002: Analyzers stay deterministic; LLM only explains and proposes"
weight: 20
---
**Status:** Accepted
**Related:** reads [`../concepts/analyzer-interface.md`](/docs/concepts/analyzer-interface/); affects [`../components/server/README.md`](/docs/components/server/)

## Context

An LLM could, in principle, be pointed at raw cluster state and asked to find problems directly. That's tempting for coverage but produces findings that are hard to test, hard to trust under audit, and expensive to run continuously — the same failure mode (e.g. an orphaned `DestinationRule` subset) would be independently "discovered" by the model each scan rather than mechanically detected once.

## Decision

Detection is rule-based. Every [`Analyzer`](/docs/concepts/analyzer-interface/) reads a point-in-time `MeshSnapshot` and returns structured [`Finding`](/docs/concepts/finding-and-incident/) objects — no network calls, no model calls, fully unit-testable against fixture snapshots. The LLM is invoked only after a Finding exists, and only for two jobs: turn the finding's raw evidence into a plain-language explanation, and (separately) propose a structured, schema-validated fix. See [`../concepts/finding-and-incident.md`](/docs/concepts/finding-and-incident/) for the Finding shape and [`../architecture/overview.md#llm-integration`](/docs/architecture/#llm-integration) for the two-call flow.

## Consequences

Detection quality is bounded by analyzer coverage, not model behavior — a missed failure mode is a missing analyzer, not a prompting problem, which is a debuggable gap. Explanations can still be swapped across model providers (see [`../components/server/README.md`](/docs/components/server/)) without touching detection logic at all. The trade-off is that talam won't catch a novel failure mode no analyzer yet encodes, even if an LLM reading raw state might have noticed it — new analyzers are the intended way to close that gap.
