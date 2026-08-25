package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestMCPServer starts a real MCP server over real HTTP (streamable
// transport), with one tool that echoes back whatever headers it saw and its
// arguments — enough to verify Client.Connect/CallTool and bearer-token auth
// actually work over the wire, not just against an in-memory transport.
func newTestMCPServer(t *testing.T, wantAuthHeader string) *httptest.Server {
	t.Helper()
	s := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "v0"}, nil)
	sdkmcp.AddTool(s, &sdkmcp.Tool{Name: "echo"}, func(ctx context.Context, req *sdkmcp.CallToolRequest, in struct {
		Msg string `json:"msg"`
	}) (*sdkmcp.CallToolResult, any, error) {
		got := req.Extra.Header.Get("Authorization")
		if wantAuthHeader != "" && got != wantAuthHeader {
			return &sdkmcp.CallToolResult{
				IsError: true,
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "bad auth header: " + got}},
			}, nil, nil
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "echo:" + in.Msg}}}, nil, nil
	})
	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return s }, nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestClientCallToolOverHTTP(t *testing.T) {
	srv := newTestMCPServer(t, "")
	client, err := Connect(context.Background(), "test", srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	text, err := client.CallTool(context.Background(), "echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "echo:hi" {
		t.Errorf("expected echo:hi, got %q", text)
	}
}

func TestClientSendsBearerToken(t *testing.T) {
	srv := newTestMCPServer(t, "Bearer secret-token")
	client, err := Connect(context.Background(), "test", srv.URL, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	text, err := client.CallTool(context.Background(), "echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "echo:hi" {
		t.Fatalf("expected the auth-checking tool to succeed, got %q", text)
	}
}

func TestClientListToolNames(t *testing.T) {
	srv := newTestMCPServer(t, "")
	client, err := Connect(context.Background(), "test", srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	names, err := client.ListToolNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "echo" {
		t.Errorf("expected [echo], got %v", names)
	}
}
