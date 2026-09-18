---
title: "Contribute"
weight: 7
description: "Help build talam — report issues, submit code, write docs."
---

talam is early and welcomes contributors. Here's how to help.

## Code of Conduct

Be kind. Assume good intent. If you experience or witness a problem, [reach out](https://github.com/nirvanagit/talam/discussions).

## Getting Started

### Prerequisites

- Go 1.21+
- Kubernetes 1.26+ (kind or similar for local testing)
- Istio 1.17+ (to test analyzers)
- kubectl, kustomize

### Clone and Build

```bash
git clone https://github.com/nirvanagit/talam.git
cd talam

# Build all binaries
make build

# Run tests
make test

# Run locally against a live cluster
make deploy
```

### Project Structure

```
talam/
├── api/v1alpha1/          # CRD type definitions
├── cmd/
│   ├── talam-operator/    # Operator binary
│   ├── talam-server/      # Server binary
│   ├── talam-agent/       # Agent binary
│   └── talam-mesh-mcp/    # Istio introspection MCP server
├── internal/
│   ├── analyzer/          # Detection logic (deterministic)
│   ├── operator/          # Operator reconcilers
│   ├── agent/             # Agent reconcilers (CRD sync, apply)
│   ├── server/            # API, LLM coordination, MCP registry
│   └── meshmcp/           # Mesh-specific MCP tools
├── deploy/                # Kubernetes manifests (CRDs, RBAC, deployments)
├── docs/                  # Architecture and design documentation (markdown)
└── site/                  # Hugo documentation website
```

## Contribution Types

### 1. Bug Reports

Found a problem? [Open an issue](https://github.com/nirvanagit/talam/issues) with:

- **What you were doing** — exact steps to reproduce
- **What you expected** — the correct behavior
- **What happened** — error message, logs, screenshot
- **Environment** — Kubernetes version, Istio version, talam version

### 2. Feature Requests

Have an idea? [Discuss it first](https://github.com/nirvanagit/talam/discussions) to get feedback. Then:

- Check [ADRs]({{< relref "/docs/decisions" >}}) to understand the design philosophy
- If it's an analyzer, see [analyzer interface]({{< relref "/docs/concepts/analyzer-interface" >}})
- Open an issue or draft PR with a sketch of the solution

### 3. Code Changes

**Small fixes (tests, docs, typos):**
1. Fork the repo
2. Create a branch: `git checkout -b fix/thing`
3. Make changes
4. Run tests: `make test`
5. Commit with clear message
6. Push and open a PR

**Larger features (new analyzer, new CRD, new component):**
1. Open an issue first — discuss the approach
2. Create an ADR if it affects architecture (see [template](https://github.com/nirvanagit/talam/blob/main/docs/decisions/0000-template.md))
3. Submit a draft PR early for feedback
4. Iterate based on review
5. Once approved, merge to main (and retroactively to feat/* branch if needed)

### 4. Documentation

- **Architecture docs:** See [docs/](https://github.com/nirvanagit/talam/tree/main/docs)
- **Website content:** See [site/content/](https://github.com/nirvanagit/talam/tree/main/site/content)
- **API docs:** Update [site/content/en/docs/api/](https://github.com/nirvanagit/talam/tree/main/site/content/en/docs/api/) if you change CRDs

To rebuild the website locally:

```bash
cd site
hugo serve
```

Then visit `http://localhost:1313`.

### 5. Analyzers

Analyzers are deterministic rule-based checks. To add one:

1. Implement the [analyzer interface]({{< relref "/docs/concepts/analyzer-interface" >}})
2. Add tests (use real xDS data from istioctl or KIND)
3. Document the check in [analyzer-catalog.md]({{< relref "/docs/api/analyzer-catalog" >}})
4. Wire it into the server's [analyzer catalog](https://github.com/nirvanagit/talam/blob/main/internal/analyzer/catalog.go)

Example: [istio-mismatch-routes](https://github.com/nirvanagit/talam/blob/main/internal/analyzer/matchers.go)

## Review Process

PRs are reviewed for:

- **Correctness:** Does the code do what it claims?
- **Tests:** Are new features tested? Do tests pass?
- **Design:** Does it follow [ADR principles]({{< relref "/docs/decisions" >}})?
- **Docs:** Are docs updated? Is the change clear to future readers?

Maintainers will request changes, ask questions, or approve. Respond to feedback within a week or ping us.

## Commit Messages

Follow conventional commits:

```
type(scope): short summary

Longer explanation of why this change is needed. Reference issues:
Closes #123

Co-Authored-By: Your Name <your.email@example.com>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`

## Running Tests Locally

```bash
# Unit tests
make test

# Integration tests (requires a kind cluster)
make test-integration

# Coverage report
make test-coverage

# Lint
make lint
```

## Development Workflow

```bash
# 1. Create a branch
git checkout -b feat/thing-you-are-adding

# 2. Make changes and test
make test

# 3. Commit
git commit -m "feat(server): add new thing"

# 4. Push and open PR
git push origin feat/thing-you-are-adding

# 5. Address review feedback
git commit -m "Address review comments"
git push

# 6. Maintainer merges when ready
```

## Asking for Help

- **Architecture questions?** Open a [discussion](https://github.com/nirvanagit/talam/discussions)
- **Stuck on a bug?** Comment on the issue
- **Want to pair on something?** Reach out in the issue

We're here to help.

## License

By contributing, you agree that your changes are licensed under the same license as talam (check [LICENSE](https://github.com/nirvanagit/talam/blob/main/LICENSE)).

---

Thank you for contributing! 💙
