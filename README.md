# talam — Take A Look At Mesh

talam is a k8sgpt-style diagnostics and remediation copilot for service meshes. It watches Istio the way a senior SRE would: reading proxy config, control-plane state, and mTLS posture, turning what it finds into plain-language diagnosis, and proposing fixes a human reviews before they touch the cluster.

This repository is organized as a **documentation graph**, not a flat wiki. Every doc is a node with a stated purpose and explicit links to the nodes it depends on or informs — start at any doc and you can walk to everything connected to it. See [`docs/README.md`](docs/README.md) for the map and the conventions that keep it that way.

## Start here

- **New to talam?** Read [`docs/architecture/overview.md`](docs/architecture/overview.md) — the single artifact this repo was seeded from.
- **Looking for a specific component?** [`docs/components/`](docs/components/) has one doc per deployable: [operator](docs/components/operator/README.md), [agent](docs/components/agent/README.md), [server](docs/components/server/README.md).
- **Why was X built this way?** [`docs/decisions/`](docs/decisions/) holds the ADRs — each one links to the components and concepts it affects.
- **What does term Y mean here?** [`docs/glossary/README.md`](docs/glossary/README.md) is the shared vocabulary every other doc links back to.

## Status

Design phase. No code yet — see the [roadmap](docs/architecture/overview.md#roadmap) for what ships in v0.1.

## License

TBD.
