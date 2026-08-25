package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCLI shells out to the local `claude` CLI in non-interactive print
// mode. This lets talam run on a developer laptop with a Claude subscription
// but no ANTHROPIC_API_KEY. It is a local-dev convenience, not a deployment
// target — servers should use the Anthropic provider.
type ClaudeCLI struct {
	Binary  string
	Timeout time.Duration
}

func NewClaudeCLI() *ClaudeCLI {
	return &ClaudeCLI{Binary: "claude", Timeout: 3 * time.Minute}
}

func (c *ClaudeCLI) Name() string { return "claude-cli" }

func (c *ClaudeCLI) Complete(ctx context.Context, system, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	// --tools "" keeps the call a pure completion: no file or shell access.
	cmd := exec.CommandContext(ctx, c.Binary,
		"-p", user,
		"--append-system-prompt", system,
		"--tools", "",
		"--output-format", "text",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude CLI: %w: %s", err, truncate(string(out), 500))
	}
	return strings.TrimSpace(string(out)), nil
}
