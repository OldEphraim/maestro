package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (s *Store) UpsertSchedule(ctx context.Context, agentID uuid.UUID, cronExpr, taskPrompt string) (Schedule, error) {
	sch := Schedule{
		ID:         uuid.New(),
		AgentID:    agentID,
		CronExpr:   cronExpr,
		TaskPrompt: taskPrompt,
		Enabled:    true,
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO agent_schedules (id, agent_id, cron_expr, task_prompt)
		 VALUES ($1, $2, $3, $4)`,
		sch.ID, sch.AgentID, sch.CronExpr, sch.TaskPrompt)
	if err != nil {
		return Schedule{}, fmt.Errorf("agent.UpsertSchedule: %w", err)
	}
	return sch, nil
}

func (s *Store) GetSchedules(ctx context.Context, agentID uuid.UUID) ([]Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_id, cron_expr, task_prompt, enabled, last_run, next_run
		 FROM agent_schedules WHERE agent_id = $1`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent.GetSchedules: %w", err)
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sch Schedule
		if err := rows.Scan(&sch.ID, &sch.AgentID, &sch.CronExpr, &sch.TaskPrompt,
			&sch.Enabled, &sch.LastRun, &sch.NextRun); err != nil {
			return nil, fmt.Errorf("agent.GetSchedules scan: %w", err)
		}
		schedules = append(schedules, sch)
	}
	return schedules, nil
}

func (s *Store) SetEnabled(ctx context.Context, scheduleID uuid.UUID, enabled bool) error {
	_, err := s.db.Exec(ctx,
		`UPDATE agent_schedules SET enabled = $2 WHERE id = $1`, scheduleID, enabled)
	if err != nil {
		return fmt.Errorf("agent.SetEnabled: %w", err)
	}
	return nil
}

func (s *Store) UpdateLastRun(ctx context.Context, scheduleID uuid.UUID, lastRun, nextRun time.Time) error {
	_, err := s.db.Exec(ctx,
		`UPDATE agent_schedules SET last_run = $2, next_run = $3 WHERE id = $1`,
		scheduleID, lastRun, nextRun)
	if err != nil {
		return fmt.Errorf("agent.UpdateLastRun: %w", err)
	}
	return nil
}
