# talam — Take A Look At Mesh

talam is a k8sgpt-style diagnostics and remediation copilot for service meshes. It watches Istio the way a senior SRE would: reading proxy config, control-plane state, and mTLS posture, turning what it finds into plain-language diagnosis, and proposing fixes a human reviews before they touch the cluster.

This repository is organized as a **documentation graph**, not a flat wiki. Every doc is a node with a stated purpose and explicit links to the nodes it depends on or informs — start at any doc and you can walk to everything connected to it. See [`docs/README.md`](docs/README.md) for the map and the conventions that keep it that way.

## Start here

- **New to talam?** Read [`docs/architecture/overview.md`](docs/architecture/overview.md) — the single artifact this repo was seeded from.
- **Looking for a specific component?** [`docs/components/`](docs/components/) has one doc per deployable: [operator](docs/components/operator/README.md), [agent](docs/components/agent/README.md), [server](docs/components/server/README.md).
- **Why was X built this way?** [`docs/decisions/`](docs/decisions/) holds the ADRs — each one links to the components and concepts it affects.
- **What does term Y mean here?** [`docs/glossary/README.md`](docs/glossary/README.md) is the shared vocabulary every other doc links back to.

## Run it locally

Requires Go, Docker, [kind](https://kind.sigs.k8s.io/), and [istioctl](https://istio.io/latest/docs/setup/getting-started/) on your `PATH`.

```bash
make up
```

This creates a local kind cluster, installs Istio, builds and deploys talam-operator + talam-server + (via the operator) talam-agent, applies a demo mesh with two deliberately broken configs, and polls until the LLM gateway has produced a real explanation for each — proving the full collector → analyzer → ingest → correlation → LLM pipeline against a live cluster.

- `make deploy` — build/load images and install talam without the demo workload.
- `make demo` — apply just the broken demo mesh (after `make deploy`).
- `make port-forward` — expose the management dashboard at http://localhost:8443.
- `./bin/talamctl -server http://localhost:8443 proposals` — list/approve/reject remediation proposals from the CLI.
- `make down` — delete the kind cluster.
- `make help` — full target list.

talam-server needs an LLM credential to explain findings: set `ANTHROPIC_API_KEY` before `make up`, or apply a [`ModelBinding`](docs/api/crds.md#modelbinding) — see that doc for the Kubernetes-native way to pick the provider/model. With neither, and a local `claude` CLI on `PATH`, it falls back to that (useful for a laptop with a Claude subscription but no API key).

talam-server itself can run in or out of the cluster — see [`docs/components/server/README.md`](docs/components/server/README.md).

## Status

v0.1 in progress: talam-agent, talam-server, and talam-operator are implemented per the architecture below, with an initial 3-analyzer slice ([`docs/api/analyzer-catalog.md`](docs/api/analyzer-catalog.md) lists 8; the remaining 5 are tracked follow-up work) — see the [roadmap](docs/architecture/overview.md#roadmap).

## License

TBD.
