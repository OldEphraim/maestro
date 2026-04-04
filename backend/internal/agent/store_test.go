package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldephraim/maestro/backend/internal/agent"
)

const defaultTestDB = "postgres://maestro:maestro@localhost:5432/maestro_test?sslmode=disable"

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL_TEST")
	if dbURL == "" {
		dbURL = defaultTestDB
	}

	// Run migrations
	m, err := migrate.New("file://../../migrations", dbURL)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	_ = m.Up() // ignore ErrNoChange
	m.Close()

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Clean up test data
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
	})

	return pool
}

func TestAgentCRUD(t *testing.T) {
	pool := testDB(t)
	store := agent.NewStore(pool)
	ctx := context.Background()

	// Create
	a := agent.Agent{
		Name:         "Test Agent",
		Role:         "tester",
		SystemPrompt: "You are a test agent.",
		Model:        "claude-sonnet-4-5-20250929",
		Tools:        []string{"shell"},
		Channels:     []string{"internal"},
		Guardrails:   agent.Guardrails{MaxTokensPerRun: 1000},
	}
	created, err := store.Create(ctx, a)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID.String() == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.Name != "Test Agent" {
		t.Fatalf("expected name 'Test Agent', got %q", created.Name)
	}

	// Get
	got, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Role != "tester" {
		t.Fatalf("expected role 'tester', got %q", got.Role)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "shell" {
		t.Fatalf("expected tools [shell], got %v", got.Tools)
	}

	// Update
	got.Name = "Updated Agent"
	updated, err := store.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Updated Agent" {
		t.Fatalf("expected name 'Updated Agent', got %q", updated.Name)
	}

	// List
	agents, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) < 1 {
		t.Fatal("expected at least 1 agent")
	}

	// Delete
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = store.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestGetWithMemory(t *testing.T) {
	pool := testDB(t)
	store := agent.NewStore(pool)
	ctx := context.Background()

	a, err := store.Create(ctx, agent.Agent{
		Name:         "Memory Agent",
		Role:         "tester",
		SystemPrompt: "test",
		Model:        "claude-sonnet-4-5-20250929",
		Tools:        []string{},
		Channels:     []string{},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set memory
	if err := store.SetMemory(ctx, a.ID, "user_name", "Alice"); err != nil {
		t.Fatalf("SetMemory: %v", err)
	}
	if err := store.SetMemory(ctx, a.ID, "preference", "dark_mode"); err != nil {
		t.Fatalf("SetMemory: %v", err)
	}

	// Get with memory
	awm, err := store.GetWithMemory(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetWithMemory: %v", err)
	}
	if awm.Memory["user_name"] != "Alice" {
		t.Fatalf("expected memory user_name=Alice, got %q", awm.Memory["user_name"])
	}
	if awm.Memory["preference"] != "dark_mode" {
		t.Fatalf("expected memory preference=dark_mode, got %q", awm.Memory["preference"])
	}

	// Upsert memory
	if err := store.SetMemory(ctx, a.ID, "user_name", "Bob"); err != nil {
		t.Fatalf("SetMemory upsert: %v", err)
	}
	mem, err := store.GetMemory(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem["user_name"] != "Bob" {
		t.Fatalf("expected updated memory user_name=Bob, got %q", mem["user_name"])
	}

	// Delete memory key
	if err := store.DeleteMemoryKey(ctx, a.ID, "preference"); err != nil {
		t.Fatalf("DeleteMemoryKey: %v", err)
	}
	mem, err = store.GetMemory(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetMemory after delete: %v", err)
	}
	if _, ok := mem["preference"]; ok {
		t.Fatal("expected preference to be deleted")
	}
}
