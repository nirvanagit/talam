# Contributing

## Code layout

- `cmd/talam-operator/` — operator entrypoint. See [`docs/components/operator/README.md`](docs/components/operator/README.md).
- `cmd/talam-agent/` — agent entrypoint. See [`docs/components/agent/README.md`](docs/components/agent/README.md).
- `cmd/talam-server/` — server entrypoint. See [`docs/components/server/README.md`](docs/components/server/README.md).
- `api/v1alpha1/` — CRD types (`MeshDiagnostics`, `MeshFinding`). See [`docs/api/crds.md`](docs/api/crds.md).
- `internal/` — private implementation shared across components (analyzer engine, collectors, LLM gateway).
- `pkg/` — code intended for reuse outside this module (the `Analyzer` interface itself lives here so a future out-of-tree analyzer plugin can implement it).

## Documentation

`docs/` is organized as a linked graph, not a flat wiki — read [`docs/README.md`](docs/README.md) before adding a new doc. In short: every doc opens with a `**Related:**` line linking what it depends on, link to a concept's definition on first use rather than redefining it inline, and a decision that changes gets a new ADR rather than an edit to an accepted one.

No doc should be added without being linked from at least one existing node.
