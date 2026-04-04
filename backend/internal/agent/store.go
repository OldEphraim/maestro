package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, a Agent) (Agent, error) {
	a.ID = uuid.New()
	toolsJSON, _ := json.Marshal(a.Tools)
	channelsJSON, _ := json.Marshal(a.Channels)
	guardrailsJSON, _ := json.Marshal(a.Guardrails)

	err := s.db.QueryRow(ctx,
		`INSERT INTO agents (id, name, role, system_prompt, model, tools, channels, guardrails)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at, updated_at`,
		a.ID, a.Name, a.Role, a.SystemPrompt, a.Model, toolsJSON, channelsJSON, guardrailsJSON,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Agent{}, fmt.Errorf("agent.Create: %w", err)
	}
	return a, nil
}

func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (Agent, error) {
	var a Agent
	var toolsJSON, channelsJSON, guardrailsJSON []byte
	err := s.db.QueryRow(ctx,
		`SELECT id, name, role, system_prompt, model, tools, channels, guardrails, created_at, updated_at
		 FROM agents WHERE id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.Role, &a.SystemPrompt, &a.Model,
		&toolsJSON, &channelsJSON, &guardrailsJSON, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Agent{}, fmt.Errorf("agent.GetByID: %w", err)
	}
	json.Unmarshal(toolsJSON, &a.Tools)
	json.Unmarshal(channelsJSON, &a.Channels)
	json.Unmarshal(guardrailsJSON, &a.Guardrails)
	return a, nil
}

func (s *Store) GetWithMemory(ctx context.Context, id uuid.UUID) (AgentWithMemory, error) {
	a, err := s.GetByID(ctx, id)
	if err != nil {
		return AgentWithMemory{}, err
	}
	mem, err := s.GetMemory(ctx, id)
	if err != nil {
		return AgentWithMemory{}, fmt.Errorf("agent.GetWithMemory: %w", err)
	}
	return AgentWithMemory{Agent: a, Memory: mem}, nil
}

func (s *Store) List(ctx context.Context) ([]Agent, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, role, system_prompt, model, tools, channels, guardrails, created_at, updated_at
		 FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("agent.List: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var toolsJSON, channelsJSON, guardrailsJSON []byte
		if err := rows.Scan(&a.ID, &a.Name, &a.Role, &a.SystemPrompt, &a.Model,
			&toolsJSON, &channelsJSON, &guardrailsJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("agent.List scan: %w", err)
		}
		json.Unmarshal(toolsJSON, &a.Tools)
		json.Unmarshal(channelsJSON, &a.Channels)
		json.Unmarshal(guardrailsJSON, &a.Guardrails)
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *Store) Update(ctx context.Context, a Agent) (Agent, error) {
	toolsJSON, _ := json.Marshal(a.Tools)
	channelsJSON, _ := json.Marshal(a.Channels)
	guardrailsJSON, _ := json.Marshal(a.Guardrails)

	err := s.db.QueryRow(ctx,
		`UPDATE agents SET name=$2, role=$3, system_prompt=$4, model=$5,
		 tools=$6, channels=$7, guardrails=$8, updated_at=NOW()
		 WHERE id=$1
		 RETURNING updated_at`,
		a.ID, a.Name, a.Role, a.SystemPrompt, a.Model, toolsJSON, channelsJSON, guardrailsJSON,
	).Scan(&a.UpdatedAt)
	if err != nil {
		return Agent{}, fmt.Errorf("agent.Update: %w", err)
	}
	return a, nil
}

func (s *Store) FindByChannel(ctx context.Context, channel string) (Agent, error) {
	var a Agent
	var toolsJSON, channelsJSON, guardrailsJSON []byte
	err := s.db.QueryRow(ctx,
		`SELECT id, name, role, system_prompt, model, tools, channels, guardrails, created_at, updated_at
		 FROM agents WHERE channels @> $1::jsonb LIMIT 1`,
		fmt.Sprintf(`[%q]`, channel),
	).Scan(&a.ID, &a.Name, &a.Role, &a.SystemPrompt, &a.Model,
		&toolsJSON, &channelsJSON, &guardrailsJSON, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Agent{}, fmt.Errorf("agent.FindByChannel: %w", err)
	}
	json.Unmarshal(toolsJSON, &a.Tools)
	json.Unmarshal(channelsJSON, &a.Channels)
	json.Unmarshal(guardrailsJSON, &a.Guardrails)
	return a, nil
}

func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("agent.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
