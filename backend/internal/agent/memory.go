package agent

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) SetMemory(ctx context.Context, agentID uuid.UUID, key, value string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO agent_memory (id, agent_id, key, value)
		 VALUES (gen_random_uuid(), $1, $2, $3)
		 ON CONFLICT (agent_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
		agentID, key, value)
	if err != nil {
		return fmt.Errorf("agent.SetMemory: %w", err)
	}
	return nil
}

func (s *Store) GetMemory(ctx context.Context, agentID uuid.UUID) (map[string]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT key, value FROM agent_memory WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent.GetMemory: %w", err)
	}
	defer rows.Close()

	mem := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("agent.GetMemory scan: %w", err)
		}
		mem[k] = v
	}
	return mem, nil
}

func (s *Store) DeleteMemoryKey(ctx context.Context, agentID uuid.UUID, key string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM agent_memory WHERE agent_id = $1 AND key = $2`, agentID, key)
	if err != nil {
		return fmt.Errorf("agent.DeleteMemoryKey: %w", err)
	}
	return nil
}
