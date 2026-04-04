package workflow

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

type Workflow struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	TemplateID  string    `json:"template_id,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type WorkflowNode struct {
	ID         uuid.UUID `json:"id"`
	WorkflowID uuid.UUID `json:"workflow_id"`
	AgentID    uuid.UUID `json:"agent_id"`
	Label      string    `json:"label,omitempty"`
	PositionX  float64   `json:"position_x"`
	PositionY  float64   `json:"position_y"`
	IsEntry    bool      `json:"is_entry"`
}

type WorkflowEdge struct {
	ID           uuid.UUID `json:"id"`
	WorkflowID   uuid.UUID `json:"workflow_id"`
	SourceNodeID uuid.UUID `json:"source_node_id"`
	TargetNodeID uuid.UUID `json:"target_node_id"`
	Condition    string    `json:"condition,omitempty"`
	Priority     int       `json:"priority"`
}

type FullWorkflow struct {
	Workflow
	Nodes []WorkflowNode `json:"nodes"`
	Edges []WorkflowEdge `json:"edges"`
}

func (fw *FullWorkflow) EntryNode() *WorkflowNode {
	for i := range fw.Nodes {
		if fw.Nodes[i].IsEntry {
			return &fw.Nodes[i]
		}
	}
	return nil
}

func (fw *FullWorkflow) OutgoingEdges(nodeID uuid.UUID) []WorkflowEdge {
	var edges []WorkflowEdge
	for _, e := range fw.Edges {
		if e.SourceNodeID == nodeID {
			edges = append(edges, e)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Priority < edges[j].Priority
	})
	return edges
}

func (fw *FullWorkflow) Node(nodeID uuid.UUID) *WorkflowNode {
	for i := range fw.Nodes {
		if fw.Nodes[i].ID == nodeID {
			return &fw.Nodes[i]
		}
	}
	return nil
}

type Execution struct {
	ID             uuid.UUID  `json:"id"`
	WorkflowID     *uuid.UUID `json:"workflow_id,omitempty"`
	ExecutionType  string     `json:"execution_type"`
	Status         string     `json:"status"`
	TriggeredBy    string     `json:"triggered_by,omitempty"`
	IterationCount int        `json:"iteration_count"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type Message struct {
	ID          uuid.UUID  `json:"id"`
	ExecutionID uuid.UUID  `json:"execution_id"`
	FromAgentID uuid.UUID  `json:"from_agent_id"`
	ToAgentID   *uuid.UUID `json:"to_agent_id,omitempty"`
	Content     string     `json:"content"`
	Channel     string     `json:"channel"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Log struct {
	ID          uuid.UUID      `json:"id"`
	ExecutionID uuid.UUID      `json:"execution_id"`
	AgentID     uuid.UUID      `json:"agent_id"`
	Level       string         `json:"level"`
	Message     string         `json:"message"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}
