// Package mcp is talam-server's MCP client: connecting to registered
// MCPServer objects and calling their tools. Per ADR-0006, every call here
// is initiated by talam-server's own deterministic enrichment table
// (internal/server/enrich) — nothing in this package lets an LLM choose a
// tool or its arguments.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps one MCP session for a single registered server.
type Client struct {
	Name    string
	session *mcp.ClientSession
}

// Connect dials a streamable-HTTP MCP endpoint and initializes a session.
func Connect(ctx context.Context, name, endpoint, bearerToken string) (*Client, error) {
	httpClient := http.DefaultClient
	if bearerToken != "" {
		httpClient = &http.Client{Transport: bearerRoundTripper{token: bearerToken, base: http.DefaultTransport}}
	}
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}
	client := mcp.NewClient(&mcp.Implementation{Name: "talam-server", Version: "v0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to MCP server %q at %s: %w", name, endpoint, err)
	}
	return &Client{Name: name, session: session}, nil
}

func (c *Client) Close() error {
	return c.session.Close()
}

// ListToolNames returns the names of every tool this server exposes —
// used only to populate MCPServer.status.availableTools for visibility,
// never to let anything choose a tool dynamically (ADR-0006).
func (c *Client) ListToolNames(ctx context.Context) ([]string, error) {
	res, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(res.Tools))
	for _, t := range res.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

// CallTool invokes one tool with the given arguments (already built by the
// deterministic enrichment table) and returns its text content joined —
// exactly what gets appended to a Finding's evidence.
func (c *Client) CallTool(ctx context.Context, tool string, args map[string]any) (string, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("calling tool %q on %q: %w", tool, c.Name, err)
	}
	if res.IsError {
		return "", fmt.Errorf("tool %q on %q returned an error result: %s", tool, c.Name, textOf(res.Content))
	}
	return textOf(res.Content), nil
}

func textOf(content []mcp.Content) string {
	var b strings.Builder
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
