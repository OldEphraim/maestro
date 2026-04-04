package runtime

import (
	"context"
	"errors"

	"github.com/oldephraim/maestro/backend/internal/agent"
)

var ErrStepTimeout = errors.New("agent step timed out")

type Usage struct {
	TokensIn         int     `json:"tokens_in"`
	TokensOut        int     `json:"tokens_out"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	Source           string  `json:"source"`
}

type Runner interface {
	Run(ctx context.Context, ag agent.AgentWithMemory, task string) (string, Usage, error)
}
