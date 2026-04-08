package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	// Seed template agents if DB is empty
	seedTemplateAgents(context.Background(), agentStore, envOrDefault("TEMPLATES_DIR", "templates"))

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
		// Clean up stale ngrok endpoints before connecting
		ngrokDomain := os.Getenv("NGROK_DOMAIN")
		if apiKey := os.Getenv("NGROK_API_KEY"); apiKey != "" && ngrokDomain != "" {
			cleanupStaleNgrokEndpoints(apiKey, ngrokDomain)
		}

		// Start ngrok tunnel. The context governs the tunnel's lifetime, so we
		// use Background() — not a timeout context — to keep it alive. The SDK
		// has its own internal connect timeout.
		log.Println("NGROK_AUTH_TOKEN set — starting ngrok tunnel...")
		endpointOpts := []ngrokconfig.HTTPEndpointOption{
			ngrokconfig.WithPoolingEnabled(true),
		}
		if ngrokDomain != "" {
			endpointOpts = append(endpointOpts, ngrokconfig.WithDomain(ngrokDomain))
			log.Printf("using reserved ngrok domain: %s", ngrokDomain)
		}
		tun, err := ngrok.Listen(context.Background(),
			ngrokconfig.HTTPEndpoint(endpointOpts...),
			ngrok.WithAuthtoken(ngrokAuthToken),
			ngrok.WithStopHandler(func(ctx context.Context, sess ngrok.Session) error {
				log.Println("ngrok remote stop requested — shutting down")
				os.Exit(0)
				return nil
			}),
		)

		if err != nil {
			log.Printf("WARNING: ngrok tunnel failed: %v", err)
			log.Println("falling back to local-only mode — WhatsApp inbound webhooks won't work")
		} else {
			defer tun.Close()
			tunnelURL := tun.URL()
			log.Printf("ngrok tunnel active — WhatsApp webhook: %s/api/webhooks/whatsapp", tunnelURL)

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

// seedTemplateAgents inserts agents from template JSON files into the database
// if the agents table is empty. On every startup, it also syncs system prompts
// of existing template agents to match the current template files.
func seedTemplateAgents(ctx context.Context, store *agent.Store, templatesDir string) {
	type templateAgent struct {
		Name         string   `json:"name"`
		Role         string   `json:"role"`
		SystemPrompt string   `json:"system_prompt"`
		Model        string   `json:"model"`
		Tools        []string `json:"tools"`
		Channels     []string `json:"channels"`
	}

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		log.Printf("WARNING: seed templates dir: %v", err)
		return
	}

	var allAgents []templateAgent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
		if err != nil {
			continue
		}
		var tf struct {
			Agents []templateAgent `json:"agents"`
		}
		if err := json.Unmarshal(data, &tf); err != nil {
			continue
		}
		allAgents = append(allAgents, tf.Agents...)
	}

	seeded, synced := 0, 0
	for _, ta := range allAgents {
		a := agent.Agent{
			Name:         ta.Name,
			Role:         ta.Role,
			SystemPrompt: ta.SystemPrompt,
			Model:        ta.Model,
			Tools:        ta.Tools,
			Channels:     ta.Channels,
		}
		if a.Model == "" {
			a.Model = "claude-sonnet-4-5-20250929"
		}
		if a.Tools == nil {
			a.Tools = []string{}
		}
		if a.Channels == nil {
			a.Channels = []string{}
		}

		existing, err := store.FindByName(ctx, ta.Name)
		if err == nil {
			// Agent exists — sync system prompt if changed
			if existing.SystemPrompt != ta.SystemPrompt {
				existing.SystemPrompt = ta.SystemPrompt
				existing.Channels = a.Channels
				if _, err := store.Update(ctx, existing); err != nil {
					log.Printf("WARNING: sync agent %s: %v", ta.Name, err)
				} else {
					synced++
				}
			}
			continue
		}
		// Agent doesn't exist — create it
		if _, err := store.Create(ctx, a); err != nil {
			log.Printf("WARNING: seed agent %s: %v", ta.Name, err)
		} else {
			seeded++
		}
	}
	if seeded > 0 {
		log.Printf("seeded %d template agents", seeded)
	}
	if synced > 0 {
		log.Printf("synced %d template agent prompts", synced)
	}
}

// cleanupStaleNgrokEndpoints finds any existing ngrok endpoints on the reserved
// domain and stops their tunnel sessions via the ngrok API. This handles the case
// where a previous server instance was killed without graceful shutdown.
func cleanupStaleNgrokEndpoints(apiKey, domain string) {
	log.Println("checking for stale ngrok endpoints...")

	req, err := http.NewRequest("GET", "https://api.ngrok.com/endpoints", nil)
	if err != nil {
		log.Printf("WARNING: ngrok cleanup: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Ngrok-Version", "2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("WARNING: ngrok cleanup request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("WARNING: ngrok endpoints API returned %d: %s", resp.StatusCode, string(body))
		return
	}

	var result struct {
		Endpoints []struct {
			ID            string `json:"id"`
			Hostport      string `json:"hostport"`
			TunnelSession struct {
				ID string `json:"id"`
			} `json:"tunnel_session"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("WARNING: ngrok cleanup decode: %v", err)
		return
	}

	stopped := 0
	for _, ep := range result.Endpoints {
		if ep.Hostport == domain+":443" || ep.Hostport == domain || strings.HasPrefix(ep.Hostport, domain) {
			sessionID := ep.TunnelSession.ID
			if sessionID == "" {
				continue
			}
			log.Printf("stopping stale tunnel session %s for endpoint %s", sessionID, ep.ID)
			stopReq, err := http.NewRequest("POST", "https://api.ngrok.com/tunnel_sessions/"+sessionID+"/stop", strings.NewReader("{}"))
			if err != nil {
				continue
			}
			stopReq.Header.Set("Authorization", "Bearer "+apiKey)
			stopReq.Header.Set("Ngrok-Version", "2")
			stopReq.Header.Set("Content-Type", "application/json")
			stopResp, err := http.DefaultClient.Do(stopReq)
			if err != nil {
				log.Printf("WARNING: failed to stop session %s: %v", sessionID, err)
				continue
			}
			stopResp.Body.Close()
			if stopResp.StatusCode < 300 || stopResp.StatusCode == 204 {
				stopped++
			} else {
				respBody, _ := io.ReadAll(stopResp.Body)
				log.Printf("WARNING: stop session %s returned %d: %s", sessionID, stopResp.StatusCode, string(respBody))
			}
		}
	}

	if stopped > 0 {
		log.Printf("stopped %d stale ngrok session(s) — waiting for cleanup", stopped)
		time.Sleep(2 * time.Second)
	} else {
		log.Println("no stale ngrok endpoints found")
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
