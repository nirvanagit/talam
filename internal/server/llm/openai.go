package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatible calls any endpoint implementing the OpenAI chat completions
// shape — used for self-hosted models on air-gapped clusters
// (docs/components/server/README.md).
type OpenAICompatible struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (o *OpenAICompatible) Name() string { return "openai-compatible:" + o.Model }

func (o *OpenAICompatible) Complete(ctx context.Context, system, user string) (string, error) {
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	payload := map[string]any{
		"model": o.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := strings.TrimSuffix(o.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai-compatible endpoint %s: %s", resp.Status, truncate(string(raw), 500))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible endpoint returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
