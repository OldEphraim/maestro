package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oldephraim/maestro/backend/internal/agent"
)

type GooseRunner struct {
	binaryPath string
	apiKey     string
	lastPrompt string
}

func NewGooseRunner(binaryPath, apiKey string) *GooseRunner {
	return &GooseRunner{binaryPath: binaryPath, apiKey: apiKey}
}

type gooseMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type gooseOutput struct {
	Messages []gooseMessage `json:"messages"`
	Metadata struct {
		TotalTokens *int   `json:"total_tokens"`
		Status      string `json:"status"`
	} `json:"metadata"`
}

func gooseModelName(model string) string {
	// Strip date suffix: "claude-sonnet-4-5-20250929" → "claude-sonnet-4-5"
	parts := strings.Split(model, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if len(parts[i]) == 8 {
			allDigit := true
			for _, c := range parts[i] {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit {
				return strings.Join(parts[:i], "-")
			}
		}
	}
	return model
}

func (r *GooseRunner) Run(ctx context.Context, ag agent.AgentWithMemory, task string) (string, Usage, error) {
	fullPrompt := buildFullPrompt(ag, task)
	r.lastPrompt = fullPrompt

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binaryPath, "run",
		"--no-session", "--provider", "anthropic",
		"--model", gooseModelName(ag.Model),
		"--output-format", "json",
		"-t", fullPrompt,
	)
	cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+r.apiKey)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", Usage{}, ErrStepTimeout
		}
		return "", Usage{}, fmt.Errorf("goose run: %w\nstderr: %s", err, stderr.String())
	}

	return r.parseOutput(stdout.Bytes(), ag.Model)
}

func (r *GooseRunner) parseOutput(raw []byte, model string) (string, Usage, error) {
	// Skip banner text before JSON
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return string(raw), r.estimateUsage(string(raw), model), nil
	}

	var out gooseOutput
	if err := json.Unmarshal(raw[start:], &out); err != nil {
		return string(raw), r.estimateUsage(string(raw), model), nil
	}

	// Extract last assistant message
	var responseText string
	for i := len(out.Messages) - 1; i >= 0; i-- {
		if out.Messages[i].Role == "assistant" && len(out.Messages[i].Content) > 0 {
			responseText = out.Messages[i].Content[0].Text
			break
		}
	}

	usage := r.estimateUsage(responseText, model)
	if out.Metadata.TotalTokens != nil {
		total := *out.Metadata.TotalTokens
		// Split roughly: assume 70% input, 30% output
		usage.TokensIn = total * 7 / 10
		usage.TokensOut = total - usage.TokensIn
		usage.Source = "goose_json"
	}
	usage.EstimatedCostUSD = estimateCost(usage.TokensIn, usage.TokensOut, model)

	return responseText, usage, nil
}

func (r *GooseRunner) estimateUsage(response, model string) Usage {
	tokensIn := len(r.lastPrompt) / 4
	tokensOut := len(response) / 4
	return Usage{
		TokensIn:         tokensIn,
		TokensOut:        tokensOut,
		Source:           "estimated",
		EstimatedCostUSD: estimateCost(tokensIn, tokensOut, model),
	}
}

func estimateCost(tokensIn, tokensOut int, model string) float64 {
	// Pricing per 1M tokens (approximate)
	var inRate, outRate float64
	switch {
	case strings.Contains(model, "opus"):
		inRate, outRate = 15.0, 75.0
	case strings.Contains(model, "haiku"):
		inRate, outRate = 0.25, 1.25
	default: // sonnet
		inRate, outRate = 3.0, 15.0
	}
	return (float64(tokensIn)*inRate + float64(tokensOut)*outRate) / 1_000_000
}
