package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/oldephraim/maestro/backend/internal/agent"
)

type AnthropicDirectRunner struct {
	apiKey string
	client *http.Client
}

func NewAnthropicDirectRunner(apiKey string) *AnthropicDirectRunner {
	return &AnthropicDirectRunner{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (r *AnthropicDirectRunner) Run(ctx context.Context, ag agent.AgentWithMemory, task string) (string, Usage, error) {
	payload := map[string]any{
		"model":      ag.Model,
		"max_tokens": 4096,
		"system":     buildSystemPrompt(ag),
		"messages":   []map[string]any{{"role": "user", "content": task}},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic request: %w", err)
	}
	req.Header.Set("x-api-key", r.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("anthropic API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", Usage{}, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", Usage{}, fmt.Errorf("anthropic decode: %w", err)
	}

	text := ""
	if len(result.Content) > 0 {
		text = result.Content[0].Text
	}

	usage := Usage{
		TokensIn:         result.Usage.InputTokens,
		TokensOut:        result.Usage.OutputTokens,
		Source:           "anthropic_api",
		EstimatedCostUSD: estimateCost(result.Usage.InputTokens, result.Usage.OutputTokens, ag.Model),
	}
	return text, usage, nil
}
