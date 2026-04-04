package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldephraim/maestro/backend/internal/agent"
)

var (
	ErrCostLimitExceeded = errors.New("cost limit exceeded")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// --- Workflow CRUD ---

func (s *Store) CreateWorkflow(ctx context.Context, w Workflow) (Workflow, error) {
	w.ID = uuid.New()
	err := s.db.QueryRow(ctx,
		`INSERT INTO workflows (id, name, description, template_id, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING created_at`,
		w.ID, w.Name, w.Description, w.TemplateID, w.Status,
	).Scan(&w.CreatedAt)
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow.Create: %w", err)
	}
	return w, nil
}

func (s *Store) GetWorkflow(ctx context.Context, id uuid.UUID) (Workflow, error) {
	var w Workflow
	err := s.db.QueryRow(ctx,
		`SELECT id, name, description, template_id, status, created_at FROM workflows WHERE id = $1`, id,
	).Scan(&w.ID, &w.Name, &w.Description, &w.TemplateID, &w.Status, &w.CreatedAt)
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow.Get: %w", err)
	}
	return w, nil
}

func (s *Store) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, description, template_id, status, created_at FROM workflows ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("workflow.List: %w", err)
	}
	defer rows.Close()
	var workflows []Workflow
	for rows.Next() {
		var w Workflow
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.TemplateID, &w.Status, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("workflow.List scan: %w", err)
		}
		workflows = append(workflows, w)
	}
	return workflows, nil
}

func (s *Store) UpdateWorkflow(ctx context.Context, w Workflow) (Workflow, error) {
	_, err := s.db.Exec(ctx,
		`UPDATE workflows SET name=$2, description=$3, status=$4 WHERE id=$1`,
		w.ID, w.Name, w.Description, w.Status)
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow.Update: %w", err)
	}
	return w, nil
}

func (s *Store) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM workflows WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("workflow.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// --- Nodes ---

func (s *Store) CreateNode(ctx context.Context, n WorkflowNode) (WorkflowNode, error) {
	n.ID = uuid.New()
	_, err := s.db.Exec(ctx,
		`INSERT INTO workflow_nodes (id, workflow_id, agent_id, label, position_x, position_y, is_entry)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		n.ID, n.WorkflowID, n.AgentID, n.Label, n.PositionX, n.PositionY, n.IsEntry)
	if err != nil {
		return WorkflowNode{}, fmt.Errorf("workflow.CreateNode: %w", err)
	}
	return n, nil
}

func (s *Store) DeleteNode(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM workflow_nodes WHERE id = $1`, id)
	return err
}

// --- Edges ---

func (s *Store) CreateEdge(ctx context.Context, e WorkflowEdge) (WorkflowEdge, error) {
	e.ID = uuid.New()
	_, err := s.db.Exec(ctx,
		`INSERT INTO workflow_edges (id, workflow_id, source_node_id, target_node_id, condition, priority)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.WorkflowID, e.SourceNodeID, e.TargetNodeID, e.Condition, e.Priority)
	if err != nil {
		return WorkflowEdge{}, fmt.Errorf("workflow.CreateEdge: %w", err)
	}
	return e, nil
}

func (s *Store) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM workflow_edges WHERE id = $1`, id)
	return err
}

// --- GetFull ---

func (s *Store) GetFull(ctx context.Context, workflowID uuid.UUID) (*FullWorkflow, error) {
	w, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	fw := &FullWorkflow{Workflow: w}

	// Nodes
	rows, err := s.db.Query(ctx,
		`SELECT id, workflow_id, agent_id, label, position_x, position_y, is_entry
		 FROM workflow_nodes WHERE workflow_id = $1`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow.GetFull nodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n WorkflowNode
		if err := rows.Scan(&n.ID, &n.WorkflowID, &n.AgentID, &n.Label,
			&n.PositionX, &n.PositionY, &n.IsEntry); err != nil {
			return nil, fmt.Errorf("workflow.GetFull node scan: %w", err)
		}
		fw.Nodes = append(fw.Nodes, n)
	}
	rows.Close()

	// Edges — ORDER BY priority ASC pinned in SQL
	rows, err = s.db.Query(ctx,
		`SELECT id, workflow_id, source_node_id, target_node_id, condition, priority
		 FROM workflow_edges WHERE workflow_id = $1 ORDER BY priority ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow.GetFull edges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e WorkflowEdge
		if err := rows.Scan(&e.ID, &e.WorkflowID, &e.SourceNodeID, &e.TargetNodeID,
			&e.Condition, &e.Priority); err != nil {
			return nil, fmt.Errorf("workflow.GetFull edge scan: %w", err)
		}
		fw.Edges = append(fw.Edges, e)
	}

	return fw, nil
}

// --- Executions ---

func (s *Store) CreateExecution(ctx context.Context, exec *Execution) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO workflow_executions (id, workflow_id, execution_type, status, triggered_by)
		 VALUES ($1, $2, $3, $4, $5) RETURNING started_at`,
		exec.ID, exec.WorkflowID, exec.ExecutionType, exec.Status, exec.TriggeredBy,
	).Scan(&exec.StartedAt)
}

func (s *Store) GetExecution(ctx context.Context, id uuid.UUID) (Execution, error) {
	var exec Execution
	err := s.db.QueryRow(ctx,
		`SELECT id, workflow_id, execution_type, status, triggered_by, iteration_count, started_at, completed_at
		 FROM workflow_executions WHERE id = $1`, id,
	).Scan(&exec.ID, &exec.WorkflowID, &exec.ExecutionType, &exec.Status,
		&exec.TriggeredBy, &exec.IterationCount, &exec.StartedAt, &exec.CompletedAt)
	if err != nil {
		return Execution{}, fmt.Errorf("workflow.GetExecution: %w", err)
	}
	return exec, nil
}

func (s *Store) ListExecutions(ctx context.Context) ([]Execution, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, workflow_id, execution_type, status, triggered_by, iteration_count, started_at, completed_at
		 FROM workflow_executions ORDER BY started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("workflow.ListExecutions: %w", err)
	}
	defer rows.Close()
	var execs []Execution
	for rows.Next() {
		var e Execution
		if err := rows.Scan(&e.ID, &e.WorkflowID, &e.ExecutionType, &e.Status,
			&e.TriggeredBy, &e.IterationCount, &e.StartedAt, &e.CompletedAt); err != nil {
			return nil, fmt.Errorf("workflow.ListExecutions scan: %w", err)
		}
		execs = append(execs, e)
	}
	return execs, nil
}

func (s *Store) IncrementIterationCount(ctx context.Context, execID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`UPDATE workflow_executions SET iteration_count = iteration_count + 1
		 WHERE id = $1 RETURNING iteration_count`, execID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("workflow.IncrementIterationCount: %w", err)
	}
	return count, nil
}

func (s *Store) SetStatus(ctx context.Context, execID uuid.UUID, status string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE workflow_executions SET status = $2 WHERE id = $1`, execID, status)
	return err
}

func (s *Store) SetCompletedAt(ctx context.Context, execID uuid.UUID, t time.Time) error {
	_, err := s.db.Exec(ctx,
		`UPDATE workflow_executions SET completed_at = $2 WHERE id = $1`, execID, t)
	return err
}

// --- Messages ---

func (s *Store) CreateMessage(ctx context.Context, execID, fromAgentID uuid.UUID, toAgentID *uuid.UUID, content, channel string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO agent_messages (id, execution_id, from_agent_id, to_agent_id, content, channel)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)`,
		execID, fromAgentID, toAgentID, content, channel)
	return err
}

func (s *Store) GetMessages(ctx context.Context, execID uuid.UUID) ([]Message, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, execution_id, from_agent_id, to_agent_id, content, channel, status, created_at
		 FROM agent_messages WHERE execution_id = $1 ORDER BY created_at ASC`, execID)
	if err != nil {
		return nil, fmt.Errorf("workflow.GetMessages: %w", err)
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ExecutionID, &m.FromAgentID, &m.ToAgentID,
			&m.Content, &m.Channel, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("workflow.GetMessages scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// --- Logs ---

func (s *Store) LogEvent(ctx context.Context, execID, agentID uuid.UUID, level, message string, metadata map[string]any) error {
	metaJSON, _ := json.Marshal(metadata)
	_, err := s.db.Exec(ctx,
		`INSERT INTO execution_logs (id, execution_id, agent_id, level, message, metadata)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)`,
		execID, agentID, level, message, metaJSON)
	return err
}

func (s *Store) GetLogs(ctx context.Context, execID uuid.UUID) ([]Log, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, execution_id, agent_id, level, message, metadata, created_at
		 FROM execution_logs WHERE execution_id = $1 ORDER BY created_at ASC`, execID)
	if err != nil {
		return nil, fmt.Errorf("workflow.GetLogs: %w", err)
	}
	defer rows.Close()
	var logs []Log
	for rows.Next() {
		var l Log
		var metaJSON []byte
		if err := rows.Scan(&l.ID, &l.ExecutionID, &l.AgentID, &l.Level,
			&l.Message, &metaJSON, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("workflow.GetLogs scan: %w", err)
		}
		if metaJSON != nil {
			json.Unmarshal(metaJSON, &l.Metadata)
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// --- Costs ---

type Usage struct {
	TokensIn         int     `json:"tokens_in"`
	TokensOut        int     `json:"tokens_out"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	Source           string  `json:"source"`
}

func (s *Store) RecordCost(ctx context.Context, execID, agentID uuid.UUID, u Usage) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO execution_costs (id, execution_id, agent_id, tokens_in, tokens_out, estimated_cost_usd, source)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)`,
		execID, agentID, u.TokensIn, u.TokensOut, u.EstimatedCostUSD, u.Source)
	return err
}

// --- Guardrails ---

func (s *Store) CheckGuardrails(ctx context.Context, agentID uuid.UUID, g agent.Guardrails) error {
	if g.MaxTokensPerRun > 0 {
		var total int
		err := s.db.QueryRow(ctx,
			`SELECT COALESCE(SUM(tokens_in + tokens_out), 0)
			 FROM execution_costs WHERE agent_id = $1 AND created_at > NOW() - INTERVAL '1 hour'`,
			agentID).Scan(&total)
		if err != nil {
			return fmt.Errorf("workflow.CheckGuardrails tokens: %w", err)
		}
		if total >= g.MaxTokensPerRun {
			return ErrCostLimitExceeded
		}
	}
	if g.MaxRunsPerHour > 0 {
		var count int
		err := s.db.QueryRow(ctx,
			`SELECT COUNT(*) FROM execution_costs WHERE agent_id = $1 AND created_at > NOW() - INTERVAL '1 hour'`,
			agentID).Scan(&count)
		if err != nil {
			return fmt.Errorf("workflow.CheckGuardrails runs: %w", err)
		}
		if count >= g.MaxRunsPerHour {
			return ErrRateLimitExceeded
		}
	}
	return nil
}
