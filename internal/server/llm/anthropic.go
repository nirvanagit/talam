package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Anthropic calls the Anthropic Messages API directly. Selected when
// ANTHROPIC_API_KEY is set.
type Anthropic struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewAnthropic() *Anthropic {
	model := os.Getenv("TALAM_LLM_MODEL")
	if model == "" {
		model = "claude-sonnet-5"
	}
	return &Anthropic{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:  model,
		Client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.Model }

func (a *Anthropic) Complete(ctx context.Context, system, user string) (string, error) {
	payload := map[string]any{
		"model":      a.Model,
		"max_tokens": 2048,
		"system":     system,
		"messages": []map[string]any{
			{"role": "user", "content": user},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API %s: %s", resp.Status, truncate(string(raw), 500))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	var out string
	for _, c := range parsed.Content {
		if c.Type == "text" {
			out += c.Text
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
