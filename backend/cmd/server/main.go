package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/api"
	"github.com/oldephraim/maestro/backend/internal/channels"
	"github.com/oldephraim/maestro/backend/internal/db"
	"github.com/oldephraim/maestro/backend/internal/runtime"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

func main() {
	databaseURL := envOrDefault("DATABASE_URL", "postgres://maestro:maestro@localhost:5432/maestro?sslmode=disable")
	port := envOrDefault("PORT", "8080")

	// Run migrations
	migrationsPath := envOrDefault("MIGRATIONS_PATH", "file://migrations")
	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		log.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate.Up: %v", err)
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		log.Fatalf("migrate close src: %v", srcErr)
	}
	if dbErr != nil {
		log.Fatalf("migrate close db: %v", dbErr)
	}
	log.Println("migrations applied successfully")

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("db.Connect: %v", err)
	}
	defer pool.Close()
	log.Println("connected to database")

	// Initialize stores
	agentStore := agent.NewStore(pool)
	workflowStore := workflow.NewStore(pool)

	// Initialize runtime
	var runner runtime.Runner
	switch os.Getenv("MAESTRO_RUNTIME") {
	case "anthropic_direct":
		runner = runtime.NewAnthropicDirectRunner(os.Getenv("ANTHROPIC_API_KEY"))
		log.Println("runtime: anthropic_direct")
	default:
		runner = runtime.NewGooseRunner(
			envOrDefault("GOOSE_BINARY_PATH", "/usr/local/bin/goose"),
			os.Getenv("ANTHROPIC_API_KEY"),
		)
		log.Println("runtime: goose")
	}
	// Initialize SSE broadcaster
	broadcaster := sse.NewBroadcaster()

	// Initialize WhatsApp client (NoopClient until Phase 3 wires Twilio)
	var whatsappClient channels.WhatsAppClient
	accountSID := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	fromNumber := os.Getenv("TWILIO_WHATSAPP_FROM")
	if accountSID != "" && authToken != "" {
		whatsappClient = channels.NewTwilioClient(accountSID, authToken, fromNumber)
	} else {
		log.Println("TWILIO credentials not set — using NoopClient")
		whatsappClient = &channels.NoopClient{}
	}

	// Initialize workflow engine
	engine := workflow.NewEngine(agentStore, workflowStore, runner, broadcaster, whatsappClient)
	_ = engine

	// Templates directory
	templatesDir := envOrDefault("TEMPLATES_DIR", "templates")

	// Initialize router
	router := api.NewRouter(agentStore, workflowStore, broadcaster, engine, templatesDir)

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("starting server on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
