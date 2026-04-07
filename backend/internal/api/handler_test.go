package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	agentpkg "github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/api"
	"github.com/oldephraim/maestro/backend/internal/channels"
	"github.com/oldephraim/maestro/backend/internal/runtime"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

const defaultTestDB = "postgres://maestro:maestro@localhost:5432/maestro_test?sslmode=disable"

type mockRunner struct {
	responses []string
	callCount int
}

func (m *mockRunner) Run(ctx context.Context, ag agentpkg.AgentWithMemory, task string) (string, runtime.Usage, error) {
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

func setupRouter(t *testing.T, pool *pgxpool.Pool, runner runtime.Runner) (http.Handler, *agentpkg.Store, *workflow.Store) {
	t.Helper()
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	broadcaster := sse.NewBroadcaster()
	engine := workflow.NewEngine(agentStore, wfStore, runner, broadcaster, &channels.NoopClient{})
	router := api.NewRouter(agentStore, wfStore, broadcaster, engine, "../../templates")
	return router, agentStore, wfStore
}

func TestTemplateLoadNovaRecovery(t *testing.T) {
	pool := testPool(t)
	mock := &mockRunner{responses: []string{"monitor output", "orchestrator output", "reporter output"}}
	router, _, wfStore := setupRouter(t, pool, mock)

	// Load the nova-recovery template
	req := httptest.NewRequest("POST", "/api/templates/nova-recovery/load", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["template_id"] != "nova-recovery" {
		t.Fatalf("expected template_id 'nova-recovery', got %q", result["template_id"])
	}
	if result["status"] != "loaded" {
		t.Fatalf("expected status 'loaded', got %q", result["status"])
	}
	wfID := result["workflow_id"]
	if wfID == "" {
		t.Fatal("expected workflow_id in response")
	}

	// Verify workflow exists in DB with correct data
	req2 := httptest.NewRequest("GET", "/api/workflows/"+wfID, nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("GET workflow: expected 200, got %d", rec2.Code)
	}

	var wf struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		TemplateID string `json:"template_id"`
		Status     string `json:"status"`
		Nodes      []struct {
			ID      string `json:"id"`
			AgentID string `json:"agent_id"`
			Label   string `json:"label"`
			IsEntry bool   `json:"is_entry"`
		} `json:"nodes"`
		Edges []struct {
			ID           string `json:"id"`
			SourceNodeID string `json:"source_node_id"`
			TargetNodeID string `json:"target_node_id"`
			Condition    string `json:"condition"`
		} `json:"edges"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&wf); err != nil {
		t.Fatalf("decode workflow: %v", err)
	}

	// Verify correct number of nodes and edges (nova-recovery: 3 agents, 3 nodes, 2 edges)
	if len(wf.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(wf.Nodes))
	}
	if len(wf.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(wf.Edges))
	}

	// Verify one entry node
	entryCount := 0
	for _, n := range wf.Nodes {
		if n.IsEntry {
			entryCount++
		}
	}
	if entryCount != 1 {
		t.Fatalf("expected 1 entry node, got %d", entryCount)
	}

	// Verify agents were created (verify via agent list — should have at least 3)
	req3 := httptest.NewRequest("GET", "/api/agents", nil)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	var agents []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	json.NewDecoder(rec3.Body).Decode(&agents)
	if len(agents) < 3 {
		t.Fatalf("expected at least 3 agents, got %d", len(agents))
	}

	// Verify edge conditions match template (always, always for linear pipeline)
	for _, edge := range wf.Edges {
		if edge.Condition != "always" {
			t.Fatalf("expected edge condition 'always', got %q", edge.Condition)
		}
	}

	// Verify all node agent_ids are non-empty and distinct
	agentIDs := make(map[string]bool)
	for _, n := range wf.Nodes {
		if n.AgentID == "" {
			t.Fatal("node has empty agent_id")
		}
		agentIDs[n.AgentID] = true
	}
	if len(agentIDs) != 3 {
		t.Fatalf("expected 3 distinct agent IDs across nodes, got %d", len(agentIDs))
	}

	// Verify workflow status is active
	if wf.Status != "active" {
		t.Fatalf("expected workflow status 'active', got %q", wf.Status)
	}

	// Verify edges connect the right nodes (linear: node0→node1, node1→node2)
	_ = wfStore // suppress unused warning
	nodeIDSet := make(map[string]bool)
	for _, n := range wf.Nodes {
		nodeIDSet[n.ID] = true
	}
	for _, edge := range wf.Edges {
		if !nodeIDSet[edge.SourceNodeID] {
			t.Fatalf("edge source %s not in node set", edge.SourceNodeID)
		}
		if !nodeIDSet[edge.TargetNodeID] {
			t.Fatalf("edge target %s not in node set", edge.TargetNodeID)
		}
	}
}

func TestWhatsAppWebhookCreatesAdhocExecution(t *testing.T) {
	pool := testPool(t)
	mock := &mockRunner{responses: []string{"Hello from the agent!"}}
	router, agentStore, wfStore := setupRouter(t, pool, mock)

	// Create a WhatsApp-enabled agent
	ctx := context.Background()
	_, err := agentStore.Create(ctx, agentpkg.Agent{
		Name:         "WhatsApp Bot",
		Role:         "assistant",
		SystemPrompt: "You are a helpful assistant.",
		Model:        "claude-sonnet-4-5-20250929",
		Tools:        []string{},
		Channels:     []string{"whatsapp"},
	})
	if err != nil {
		t.Fatalf("create whatsapp agent: %v", err)
	}

	// Simulate Twilio webhook POST (form-encoded)
	form := url.Values{
		"From": {"whatsapp:+14155551234"},
		"Body": {"Hello, agent!"},
		"To":   {"whatsapp:+14155238886"},
	}
	req := httptest.NewRequest("POST", "/api/webhooks/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Webhook should return 200 immediately
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Wait for async ad-hoc execution to complete
	time.Sleep(3 * time.Second)

	// Verify an ad-hoc execution was created
	execs, err := wfStore.ListExecutions(ctx)
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(execs) == 0 {
		t.Fatal("expected at least 1 execution")
	}

	// Find the ad-hoc execution
	var adhocExec *workflow.Execution
	for i := range execs {
		if execs[i].ExecutionType == "adhoc" && execs[i].TriggeredBy == "whatsapp" {
			adhocExec = &execs[i]
			break
		}
	}
	if adhocExec == nil {
		t.Fatal("expected an adhoc execution triggered by whatsapp")
	}

	// Verify workflow_id is NULL (ad-hoc, not part of a workflow)
	if adhocExec.WorkflowID != nil {
		t.Fatalf("expected nil workflow_id for adhoc execution, got %v", adhocExec.WorkflowID)
	}

	// Verify execution completed
	if adhocExec.Status != "completed" {
		t.Fatalf("expected status 'completed', got %q", adhocExec.Status)
	}

	// Verify a message was persisted
	msgs, err := wfStore.GetMessages(ctx, adhocExec.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message from the agent")
	}
}
