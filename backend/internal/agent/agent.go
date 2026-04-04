package agent

import (
	"time"

	"github.com/google/uuid"
)

type Guardrails struct {
	MaxTokensPerRun int      `json:"max_tokens_per_run,omitempty"`
	MaxRunsPerHour  int      `json:"max_runs_per_hour,omitempty"`
	BlockedActions  []string `json:"blocked_actions,omitempty"`
}

type Agent struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	SystemPrompt string     `json:"system_prompt"`
	Model        string     `json:"model"`
	Tools        []string   `json:"tools"`
	Channels     []string   `json:"channels"`
	Guardrails   Guardrails `json:"guardrails"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (a Agent) HasChannel(name string) bool {
	for _, c := range a.Channels {
		if c == name {
			return true
		}
	}
	return false
}

type AgentWithMemory struct {
	Agent
	Memory map[string]string `json:"memory,omitempty"`
}

func (a AgentWithMemory) HasChannel(name string) bool {
	return a.Agent.HasChannel(name)
}

type Skill struct {
	ID          uuid.UUID `json:"id"`
	AgentID     uuid.UUID `json:"agent_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Steps       []string  `json:"steps"`
}

type Schedule struct {
	ID         uuid.UUID  `json:"id"`
	AgentID    uuid.UUID  `json:"agent_id"`
	CronExpr   string     `json:"cron_expr"`
	TaskPrompt string     `json:"task_prompt"`
	Enabled    bool       `json:"enabled"`
	LastRun    *time.Time `json:"last_run,omitempty"`
	NextRun    *time.Time `json:"next_run,omitempty"`
}
