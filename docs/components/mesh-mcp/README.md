# Component: mesh-mcp

**Related:** reads [`ADR-0006`](../../decisions/0006-mcp-evidence-enrichment.md), [`../../concepts/security-model.md`](../../concepts/security-model.md); links to [`../server/README.md`](../server/README.md), [`../agent/README.md`](../agent/README.md), [`../../api/crds.md`](../../api/crds.md#mcpserver)

talam-mesh-mcp is a purpose-built, read-only [MCP](https://modelcontextprotocol.io) server exposing live Istio introspection — the one MCP server this repo builds and owns, per [ADR-0006](../../decisions/0006-mcp-evidence-enrichment.md). Off-the-shelf MCP servers (metrics, generic Kubernetes) are expected to be run independently and registered the same way, by URL, via an [`MCPServer`](../../api/crds.md#mcpserver) object; this one exists because nothing generic provides Istio-specific state.

## Why it's a separate deployable

Same reasoning as the [agent](../agent/README.md) ([ADR-0001](../../decisions/0001-server-agent-operator-split.md)): it needs live read access to one cluster's mesh state, so it runs *in* that cluster with the agent's kind of scoped, read-only RBAC (`deploy/mesh-mcp/rbac.yaml`) — never talam-server's own credentials. [talam-server](../server/README.md) reaches it over HTTP as a registered `MCPServer`, which is a new connection direction ([ADR-0006](../../decisions/0006-mcp-evidence-enrichment.md) names it explicitly): everywhere else in this system, agents call *out* to the server, never the reverse.

## Tools (v0.1)

| Tool | Returns |
|---|---|
| `get_destination_rule` | One DestinationRule's live spec (host, subsets) — cross-checking analyzer evidence that may be stale by the time a human reviews it. |
| `list_service_entries` | Every ServiceEntry in a namespace with its hosts — whether a VirtualService destination that looks dangling is actually covered by one. |
| `get_mtls_status` | Every PeerAuthentication's mTLS mode alongside every DestinationRule's TLS mode in a namespace, side by side. |

Implemented in `internal/meshmcp`, reusing the same dynamic-client read patterns as [the agent's collector](../agent/README.md) — served over streamable HTTP by `cmd/talam-mesh-mcp`.

## Who calls it, and how

Never the LLM directly. talam-server's deterministic enrichment table (`internal/server/enrich`) maps a `Finding.AnalyzerID` to a fixed set of tool calls with arguments built from that finding's own `Resource` — the same kind of static mapping an analyzer's own detection logic already is. See [ADR-0006](../../decisions/0006-mcp-evidence-enrichment.md) for why this stays server-initiated rather than becoming an agentic tool-calling loop.
