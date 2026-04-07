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
	"github.com/oldephraim/maestro/backend/internal/scheduler"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
	ngrok "golang.ngrok.com/ngrok"
	ngrokconfig "golang.ngrok.com/ngrok/config"
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

	// Initialize scheduler
	sched := scheduler.New(engine, agentStore, workflowStore)
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	go sched.Start(schedCtx)

	// Templates directory
	templatesDir := envOrDefault("TEMPLATES_DIR", "templates")

	// Initialize router
	router := api.NewRouter(agentStore, workflowStore, broadcaster, engine, templatesDir)

	// Start server — with optional ngrok tunnel
	addr := fmt.Sprintf(":%s", port)

	ngrokAuthToken := os.Getenv("NGROK_AUTH_TOKEN")
	if ngrokAuthToken != "" {
		// Start ngrok tunnel (30s timeout so we don't block forever on auth issues)
		log.Println("NGROK_AUTH_TOKEN set — starting ngrok tunnel...")
		tunnelCtx, tunnelCancel := context.WithTimeout(context.Background(), 30*time.Second)
		endpointOpts := []ngrokconfig.HTTPEndpointOption{}
		if domain := os.Getenv("NGROK_DOMAIN"); domain != "" {
			endpointOpts = append(endpointOpts, ngrokconfig.WithDomain(domain))
			log.Printf("using reserved ngrok domain: %s", domain)
		}
		tun, err := ngrok.Listen(tunnelCtx,
			ngrokconfig.HTTPEndpoint(endpointOpts...),
			ngrok.WithAuthtoken(ngrokAuthToken),
		)
		tunnelCancel()

		if err != nil {
			log.Printf("WARNING: ngrok tunnel failed: %v", err)
			log.Println("falling back to local-only mode — WhatsApp inbound webhooks won't work")
		} else {
			defer tun.Close()
			tunnelURL := tun.URL()
			log.Printf("ngrok tunnel established: %s", tunnelURL)

			// Log the webhook URL for Twilio sandbox configuration.
			// The sandbox webhook can only be configured through the Twilio Console
			// (there is no REST API for sandbox webhook updates).
			webhookURL := tunnelURL + "/api/webhooks/whatsapp"
			log.Printf("WhatsApp webhook URL: %s", webhookURL)
			log.Println("Set this URL in the Twilio sandbox console: https://console.twilio.com/us1/develop/sms/try-it-out/whatsapp-learn")

			// Serve via ngrok tunnel in background
			go func() {
				log.Printf("serving via ngrok tunnel: %s", tunnelURL)
				if err := http.Serve(tun, router); err != nil {
					log.Printf("ngrok listener closed: %v", err)
				}
			}()
		}
	} else {
		log.Println("WARNING: NGROK_AUTH_TOKEN not set — WhatsApp inbound webhooks won't work without it")
	}

	// Always listen locally
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
