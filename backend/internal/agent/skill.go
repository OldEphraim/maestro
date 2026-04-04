package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) AddSkill(ctx context.Context, agentID uuid.UUID, name, description string, steps []string) (Skill, error) {
	sk := Skill{
		ID:          uuid.New(),
		AgentID:     agentID,
		Name:        name,
		Description: description,
		Steps:       steps,
	}
	stepsJSON, _ := json.Marshal(steps)
	_, err := s.db.Exec(ctx,
		`INSERT INTO agent_skills (id, agent_id, name, description, steps)
		 VALUES ($1, $2, $3, $4, $5)`,
		sk.ID, sk.AgentID, sk.Name, sk.Description, stepsJSON)
	if err != nil {
		return Skill{}, fmt.Errorf("agent.AddSkill: %w", err)
	}
	return sk, nil
}

func (s *Store) GetSkills(ctx context.Context, agentID uuid.UUID) ([]Skill, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, name, description, steps FROM agent_skills WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent.GetSkills: %w", err)
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var sk Skill
		var stepsJSON []byte
		if err := rows.Scan(&sk.ID, &sk.AgentID, &sk.Name, &sk.Description, &stepsJSON); err != nil {
			return nil, fmt.Errorf("agent.GetSkills scan: %w", err)
		}
		json.Unmarshal(stepsJSON, &sk.Steps)
		skills = append(skills, sk)
	}
	return skills, nil
}

func (s *Store) DeleteSkill(ctx context.Context, skillID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM agent_skills WHERE id = $1`, skillID)
	if err != nil {
		return fmt.Errorf("agent.DeleteSkill: %w", err)
	}
	return nil
}
