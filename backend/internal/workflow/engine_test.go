package workflow_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	agentpkg "github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/channels"
	"github.com/oldephraim/maestro/backend/internal/runtime"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

const defaultTestDB = "postgres://maestro:maestro@localhost:5432/maestro_test?sslmode=disable"

type mockRunner struct {
	responses []string
	callCount int
	delay     time.Duration
}

func (m *mockRunner) Run(ctx context.Context, ag agentpkg.AgentWithMemory, task string) (string, runtime.Usage, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", runtime.Usage{}, runtime.ErrStepTimeout
		}
	}
	idx := m.callCount
	m.callCount++
	if idx < len(m.responses) {
		return m.responses[idx], runtime.Usage{TokensIn: 100, TokensOut: 50, Source: "estimated"}, nil
	}
	return "default response", runtime.Usage{TokensIn: 100, TokensOut: 50, Source: "estimated"}, nil
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL_TEST")
	if dbURL == "" {
		dbURL = defaultTestDB
	}
	m, err := migrate.New("file://../../migrations", dbURL)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	_ = m.Up()
	m.Close()

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, "DELETE FROM execution_costs")
		pool.Exec(ctx, "DELETE FROM execution_logs")
		pool.Exec(ctx, "DELETE FROM agent_messages")
		pool.Exec(ctx, "DELETE FROM workflow_executions")
		pool.Exec(ctx, "DELETE FROM workflow_edges")
		pool.Exec(ctx, "DELETE FROM workflow_nodes")
		pool.Exec(ctx, "DELETE FROM workflows")
		pool.Exec(ctx, "DELETE FROM agent_schedules")
		pool.Exec(ctx, "DELETE FROM agent_skills")
		pool.Exec(ctx, "DELETE FROM agent_memory")
		pool.Exec(ctx, "DELETE FROM agents")
		pool.Close()
	})
	return pool
}

func createTestAgent(t *testing.T, store *agentpkg.Store, name, role string) agentpkg.Agent {
	t.Helper()
	a, err := store.Create(context.Background(), agentpkg.Agent{
		Name:         name,
		Role:         role,
		SystemPrompt: "test prompt for " + name,
		Model:        "claude-sonnet-4-5-20250929",
		Tools:        []string{},
		Channels:     []string{},
	})
	if err != nil {
		t.Fatalf("create agent %s: %v", name, err)
	}
	return a
}

func createLinearWorkflow(t *testing.T, wfStore *workflow.Store, agentIDs []uuid.UUID, labels []string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	wf, err := wfStore.CreateWorkflow(ctx, workflow.Workflow{
		Name:   "test-workflow",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	var nodeIDs []uuid.UUID
	for i, agentID := range agentIDs {
		n, err := wfStore.CreateNode(ctx, workflow.WorkflowNode{
			WorkflowID: wf.ID,
			AgentID:    agentID,
			Label:      labels[i],
			IsEntry:    i == 0,
		})
		if err != nil {
			t.Fatalf("create node %d: %v", i, err)
		}
		nodeIDs = append(nodeIDs, n.ID)
	}

	for i := 0; i < len(nodeIDs)-1; i++ {
		_, err := wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
			WorkflowID:   wf.ID,
			SourceNodeID: nodeIDs[i],
			TargetNodeID: nodeIDs[i+1],
			Condition:    "always",
			Priority:     0,
		})
		if err != nil {
			t.Fatalf("create edge %d→%d: %v", i, i+1, err)
		}
	}

	return wf.ID
}

func TestLinearWorkflowCompletes(t *testing.T) {
	pool := testPool(t)
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	broadcaster := sse.NewBroadcaster()

	agent1 := createTestAgent(t, agentStore, "Agent1", "step1")
	agent2 := createTestAgent(t, agentStore, "Agent2", "step2")

	wfID := createLinearWorkflow(t, wfStore, []uuid.UUID{agent1.ID, agent2.ID}, []string{"Step1", "Step2"})

	mock := &mockRunner{responses: []string{"output from step 1", "output from step 2"}}
	engine := workflow.NewEngine(agentStore, wfStore, mock, broadcaster, &channels.NoopClient{})

	os.Setenv("MAX_ITERATIONS", "10")
	defer os.Unsetenv("MAX_ITERATIONS")

	execID, err := engine.Execute(context.Background(), wfID, "test")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Wait for async execution (includes 2s first-step delay)
	time.Sleep(5 * time.Second)

	// Verify execution completed
	exec, err := wfStore.GetExecution(context.Background(), execID)
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if exec.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", exec.Status)
	}

	// Verify both messages persisted
	msgs, err := wfStore.GetMessages(context.Background(), execID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestCycleGuard(t *testing.T) {
	pool := testPool(t)
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	broadcaster := sse.NewBroadcaster()

	builder := createTestAgent(t, agentStore, "Builder", "builder")
	reviewer := createTestAgent(t, agentStore, "Reviewer", "reviewer")

	ctx := context.Background()
	wf, _ := wfStore.CreateWorkflow(ctx, workflow.Workflow{Name: "cycle-test", Status: "active"})

	builderNode, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: builder.ID, Label: "Builder", IsEntry: true,
	})
	reviewerNode, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: reviewer.ID, Label: "Reviewer",
	})

	// Builder → Reviewer (always)
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: builderNode.ID, TargetNodeID: reviewerNode.ID,
		Condition: "always", Priority: 0,
	})
	// Reviewer → Builder (rejected) — creates cycle
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: reviewerNode.ID, TargetNodeID: builderNode.ID,
		Condition: "rejected", Priority: 0,
	})

	// Mock always returns REJECTED — should trigger cycle guard
	mock := &mockRunner{responses: []string{
		"code v1", "REJECTED: missing error handling",
		"code v2", "REJECTED: still missing",
		"code v3", "REJECTED: nope",
		"code v4", "REJECTED: again",
		"code v5", "REJECTED: forever",
	}}

	os.Setenv("MAX_ITERATIONS", "5")
	defer os.Unsetenv("MAX_ITERATIONS")

	engine := workflow.NewEngine(agentStore, wfStore, mock, broadcaster, &channels.NoopClient{})
	execID, err := engine.Execute(ctx, wf.ID, "test")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	time.Sleep(6 * time.Second)

	exec, _ := wfStore.GetExecution(ctx, execID)
	if exec.Status != "failed" {
		t.Fatalf("expected status 'failed' (cycle guard), got %q", exec.Status)
	}
}

func TestApprovalPathCompletes(t *testing.T) {
	pool := testPool(t)
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	broadcaster := sse.NewBroadcaster()

	builder := createTestAgent(t, agentStore, "Builder", "builder")
	reviewer := createTestAgent(t, agentStore, "Reviewer", "reviewer")

	ctx := context.Background()
	wf, _ := wfStore.CreateWorkflow(ctx, workflow.Workflow{Name: "approval-test", Status: "active"})

	builderNode, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: builder.ID, Label: "Builder", IsEntry: true,
	})
	reviewerNode, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: reviewer.ID, Label: "Reviewer",
	})

	// Builder → Reviewer (always)
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: builderNode.ID, TargetNodeID: reviewerNode.ID,
		Condition: "always", Priority: 0,
	})
	// Reviewer → Builder (rejected only) — no approved edge
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: reviewerNode.ID, TargetNodeID: builderNode.ID,
		Condition: "rejected", Priority: 0,
	})

	// Mock: builder outputs code, reviewer outputs APPROVED
	mock := &mockRunner{responses: []string{"generated code", "APPROVED"}}

	os.Setenv("MAX_ITERATIONS", "10")
	defer os.Unsetenv("MAX_ITERATIONS")

	engine := workflow.NewEngine(agentStore, wfStore, mock, broadcaster, &channels.NoopClient{})
	execID, _ := engine.Execute(ctx, wf.ID, "test")

	time.Sleep(5 * time.Second)

	exec, _ := wfStore.GetExecution(ctx, execID)
	if exec.Status != "completed" {
		t.Fatalf("expected status 'completed' (approval path), got %q", exec.Status)
	}
}

func TestStepTimeout(t *testing.T) {
	pool := testPool(t)
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	broadcaster := sse.NewBroadcaster()

	agent1 := createTestAgent(t, agentStore, "SlowAgent", "slow")
	wfID := createLinearWorkflow(t, wfStore, []uuid.UUID{agent1.ID}, []string{"Slow"})

	// Mock with 2s delay
	mock := &mockRunner{responses: []string{"should not reach this"}, delay: 2 * time.Second}

	os.Setenv("AGENT_STEP_TIMEOUT_SECS", "1")
	os.Setenv("MAX_ITERATIONS", "10")
	defer os.Unsetenv("AGENT_STEP_TIMEOUT_SECS")
	defer os.Unsetenv("MAX_ITERATIONS")

	engine := workflow.NewEngine(agentStore, wfStore, mock, broadcaster, &channels.NoopClient{})
	execID, _ := engine.Execute(context.Background(), wfID, "test")

	time.Sleep(6 * time.Second)

	exec, _ := wfStore.GetExecution(context.Background(), execID)
	if exec.Status != "timed_out" {
		t.Fatalf("expected status 'timed_out', got %q", exec.Status)
	}
}
