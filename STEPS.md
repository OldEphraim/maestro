# STEPS.md — Maestro Build Log

> Hiring assessment for Yuno. Built over one weekend.
> Stack: Go + PostgreSQL + Next.js 14 + React Flow + Goose + Twilio WhatsApp Sandbox.

---

## Phase 0 — Pre-flight + Goose Verification (Friday evening, ~2 hours)

This phase is non-negotiable. The Goose CLI invocation is the highest-risk unknown in the project.
Verify it completely before writing any backend code.

### 0.1 — Environment setup

- [ ] The repo is hosted on **GitLab** at `https://labs.gauntletai.com/alangarber/maestro` (already initialized)
- [ ] Clone it: `git clone https://labs.gauntletai.com/alangarber/maestro.git && cd maestro`
  *(The `OldEphraim/maestro` GitHub handle mentioned earlier was incorrect — GitLab is the primary remote)*
- [ ] Verify local environment: Go 1.22+, Node 20+, Docker Desktop running, psql client available
- [ ] Copy `CLAUDE.md` and `STEPS.md` into the repo root and push

### 0.2 — Install and verify Goose CLI

- [ ] Install Goose: `brew install block-goose-cli` (macOS) or follow the curl installer from block.github.io/goose
- [ ] Verify installation: `goose --version`
- [ ] Configure Anthropic provider: `goose configure` → select Anthropic → enter API key
- [ ] **Critical test — run this and inspect raw output:**
  ```bash
  ANTHROPIC_API_KEY=your-key goose run \
    --no-session \
    --provider anthropic \
    --model claude-sonnet-4-5 \
    --output-format json \
    -t "Reply with exactly the word: PONG"
  ```
- [ ] Save raw stdout to `goose-test-output.json` in the repo root
- [ ] From the JSON output, identify:
  - [ ] The field name that contains the model's text response (update `GooseOutput.Response` tag if needed)
  - [ ] Whether a `usage` field with `input_tokens` / `output_tokens` is present
  - [ ] The exact model string that was accepted (confirm `claude-sonnet-4-5` works, or find the right string)
  - [ ] That exit code is 0 on success
- [ ] Commit `goose-test-output.json` as documentation

**Decision gate:** If Goose CLI works cleanly (parseable JSON, zero exit code, response field identifiable):
→ Proceed with `MAESTRO_RUNTIME=goose` as primary runtime.

If Goose CLI is flaky or output format can't be reliably parsed:
→ Set `MAESTRO_RUNTIME=anthropic_direct`. No model string migration needed — `agents.model` always stores the canonical Anthropic string (`claude-sonnet-4-5-20250929`); `AnthropicDirectRunner` uses it as-is. Proceed — don't burn Saturday morning debugging subprocess plumbing.

### 0.3 — Twilio WhatsApp Sandbox

- [ ] Create Twilio account at twilio.com (free tier is fine)
- [ ] Navigate to: Messaging → Try it Out → WhatsApp Sandbox
- [ ] Note the sandbox number and join phrase (e.g. "join bright-forest")
- [ ] Send the join phrase from your personal WhatsApp to the sandbox number — confirm the sandbox responds
- [ ] Install ngrok: `brew install ngrok` (or download from ngrok.com)
- [ ] Start ngrok: `ngrok http 8080` — copy the HTTPS URL
- [ ] In Twilio console: set the sandbox webhook URL to `https://<ngrok-url>/api/webhooks/whatsapp`
- [ ] Note: the join phrase must be re-sent if the sandbox expires (it doesn't expire during a demo)

### 0.4 — Project scaffold

- [ ] Create monorepo structure:
  ```
  mkdir -p backend/cmd/server backend/internal/{agent,workflow,runtime,scheduler,channels,sse,db,api}
  mkdir -p backend/migrations backend/templates
  mkdir frontend
  ```
- [ ] Create `.env` from `.env.example`, fill in `ANTHROPIC_API_KEY`, Twilio creds, ngrok URL
- [ ] Create `docker-compose.yml` with PostgreSQL health check (see CLAUDE.md for full config)
  - **Important:** The Docker Compose `backend` service uses `MAESTRO_RUNTIME=anthropic_direct` by default.
    Goose CLI is a local development tool only — it is not installed inside the Docker container.
    Set this in `docker-compose.yml` as an environment variable on the backend service.
    Local development (running `go run ./cmd/server` outside Docker) can use either runtime.
    The Dockerfile does not need to install Goose.
- [ ] Start PostgreSQL: `docker-compose up postgres`
- [ ] Verify connection: `psql postgres://maestro:maestro@localhost:5432/maestro`

---

## Phase 1 — Backend Foundation (Saturday AM, ~4 hours)

### 1.1 — Go module + dependencies

- [ ] `cd backend && go mod init github.com/oldephraim/maestro/backend`
- [ ] Add dependencies:
  ```bash
  go get github.com/go-chi/chi/v5
  go get github.com/jackc/pgx/v5
  go get github.com/golang-migrate/migrate/v4
  go get github.com/golang-migrate/migrate/v4/database/postgres
  go get github.com/golang-migrate/migrate/v4/source/file
  go get github.com/robfig/cron/v3
  go get github.com/twilio/twilio-go
  go get github.com/google/uuid
  go get github.com/go-chi/cors
  ```

### 1.2 — Database migrations

Create numbered SQL files in `backend/migrations/`:

- [ ] `000001_create_agents.up.sql` — agents, agent_memory, agent_skills, agent_schedules
- [ ] `000002_create_workflows.up.sql` — workflows, workflow_nodes (with `is_entry` bool), workflow_edges (with `priority` int). Edge FKs must include `ON DELETE CASCADE` on both `source_node_id` and `target_node_id` so deleting a node cleans up its edges rather than leaving orphaned rows or triggering FK violations.
- [ ] `000003_create_executions.up.sql` — workflow_executions (with `workflow_id` nullable, `execution_type`, `iteration_count`), agent_messages, execution_logs, execution_costs (with `source` field)
- [ ] Matching `.down.sql` files for each migration
- [ ] Install `migrate` CLI: `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`
- [ ] Run: `migrate -path migrations -database $DATABASE_URL up`
- [ ] Verify schema: `psql $DATABASE_URL -c '\dt'`

### 1.3 — Agent domain

- [ ] `internal/agent/agent.go`:
  - `Agent` struct: ID, Name, Role, SystemPrompt, Model, Tools ([]string from JSONB), Channels ([]string), Guardrails (struct), CreatedAt, UpdatedAt
  - `AgentWithMemory` struct: embeds Agent, adds `Memory map[string]string`
  - `Guardrails` struct: MaxTokensPerRun int, MaxRunsPerHour int, BlockedActions []string
  - `HasChannel(name string) bool` method on both `Agent` and `AgentWithMemory` — linear scan of `Channels []string`: `for _, c := range a.Channels { if c == name { return true } }; return false`. Called in `runNode` to gate the WhatsApp action handler. **Do not forget this — it is not auto-generated and the engine won't compile without it.**
- [ ] `internal/agent/store.go`: raw SQL CRUD using `pgx/v5`
  - `Create(ctx, agent) (Agent, error)`
  - `GetByID(ctx, id) (Agent, error)`
  - `GetWithMemory(ctx, id) (AgentWithMemory, error)` — JOINs agent_memory
  - `List(ctx) ([]Agent, error)`
  - `Update(ctx, agent) (Agent, error)`
  - `Delete(ctx, id) error`
- [ ] `internal/agent/memory.go`:
  - `SetMemory(ctx, agentID, key, value) error` — upsert
  - `GetMemory(ctx, agentID) (map[string]string, error)`
  - `DeleteMemoryKey(ctx, agentID, key) error`
- [ ] `internal/agent/skill.go`:
  - `AddSkill(ctx, agentID, name, description, steps) (Skill, error)`
  - `GetSkills(ctx, agentID) ([]Skill, error)`
  - `DeleteSkill(ctx, skillID) error`
- [ ] `internal/agent/schedule.go`:
  - `UpsertSchedule(ctx, agentID, cronExpr, taskPrompt) (Schedule, error)`
  - `GetSchedules(ctx, agentID) ([]Schedule, error)`
  - `SetEnabled(ctx, scheduleID, enabled) error`
  - `UpdateLastRun(ctx, scheduleID, lastRun, nextRun) error`

### 1.4 — Workflow domain

- [ ] `internal/workflow/workflow.go`:
  - `Workflow` struct: ID, Name, Description, TemplateID, Status
  - `WorkflowNode` struct: ID, WorkflowID, AgentID, Label, PositionX, PositionY, IsEntry
  - `WorkflowEdge` struct: ID, WorkflowID, SourceNodeID, TargetNodeID, Condition, Priority
  - `FullWorkflow` struct: Workflow + Nodes []WorkflowNode + Edges []WorkflowEdge
    - `EntryNode() *WorkflowNode` — finds node where IsEntry = true
    - `OutgoingEdges(nodeID) []WorkflowEdge` — returns edges sorted by Priority ASC
    - `Node(nodeID) *WorkflowNode`
- [ ] `internal/workflow/store.go`:
  - CRUD for workflows, nodes, edges
  - `GetFull(ctx, workflowID) (FullWorkflow, error)` — the SQL query for edges must include `ORDER BY priority ASC`. `OutgoingEdges(nodeID)` on the in-memory struct is then a pure filter, not a sort. Pinning the sort to SQL prevents non-deterministic edge ordering if rows are inserted out of sequence.
  - `CheckGuardrails(ctx, agentID uuid.UUID, g agent.Guardrails) error` — **lives here, not in `internal/runtime`** (see §1.5 note). Returns `ErrCostLimitExceeded` or `ErrRateLimitExceeded`.
  - `CreateExecution(ctx, exec) error`
  - `IncrementIterationCount(ctx, execID) (int, error)` — atomic increment, returns new count
  - `SetStatus(ctx, execID, status) error`
  - `SetCompletedAt(ctx, execID, time) error`
  - `CreateMessage(ctx, execID, fromAgentID, toAgentID *uuid.UUID, content, channel) error`
  - `RecordCost(ctx, execID, agentID, usage) error`
  - `LogEvent(ctx, execID, agentID, level, message string, metadata map[string]any) error`
  - `GetMessages(ctx, execID) ([]Message, error)`
  - `GetLogs(ctx, execID) ([]Log, error)`

### 1.5 — Runtime layer

- [ ] `internal/runtime/runner.go`: define `Runner` interface and `Usage`, `ErrStepTimeout` sentinel
- [ ] `internal/runtime/goose.go`: `GooseRunner` implementing `Runner` (see CLAUDE.md for full implementation)
  - `buildFullPrompt(agent, task)`: system prompt + memory key-value block + skills block + task
  - `parseOutput(raw)`: JSON unmarshal with fallback to raw text; estimate usage if `usage` field absent
- [ ] `internal/runtime/anthropic_direct.go`: `AnthropicDirectRunner` implementing `Runner` (see CLAUDE.md)
- [ ] ~~`internal/runtime/guardrails.go`~~ — **do not create this file**.
  `CheckGuardrails` lives on the **workflow store** (`internal/workflow/store.go`), not the runtime package.
  Add this method to `workflow.Store` in §1.4:
  - `CheckGuardrails(ctx context.Context, agentID uuid.UUID, g agent.Guardrails) error`
  - Queries `execution_costs` for tokens used in the current hour vs `g.MaxTokensPerRun`
  - Queries run count for the agent in the last hour vs `g.MaxRunsPerHour`
  - Returns `ErrCostLimitExceeded` or `ErrRateLimitExceeded`
  - Called as `e.workflows.CheckGuardrails(ctx, ag.ID, ag.Guardrails)` at the top of `runNode`
  - This avoids a separate `costStore` field on `Engine` and keeps all DB access through the workflow store

### 1.6 — SSE broadcaster

- [ ] `internal/sse/broadcaster.go`:
  - `Broadcaster` struct: `clients map[string]chan Event`, protected by `sync.RWMutex`
  - `Event` struct: `Type string`, `ExecutionID string`, `AgentID string`, `Payload any`
  - `Subscribe(clientID) <-chan Event`
  - `Unsubscribe(clientID)` — close the channel, remove from map
  - `Publish(event)` — fan out to all subscribers, non-blocking (use `select` with `default` to skip slow consumers)
- [ ] `internal/api/sse_handler.go`:
  - `GET /api/events?executionId=<id>`
  - Set headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`
  - Register client, stream events, clean up on disconnect via `r.Context().Done()`
  - **Filter server-side**: before writing each event to the response, compare `event.ExecutionID` against the `executionId` query param. Skip events that don't match. Do not fan out all events to all clients and filter client-side — that wastes bandwidth and leaks other executions' data to the wrong client.

### 1.7 — HTTP router + stub handlers

- [ ] Add `github.com/go-chi/cors` dependency: `go get github.com/go-chi/cors`
- [ ] `internal/api/router.go`: wire Chi router with CORS middleware first, then all routes
  ```go
  r := chi.NewRouter()
  r.Use(cors.Handler(cors.Options{
      AllowedOrigins:   []string{"http://localhost:3000"},
      AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
      AllowedHeaders:   []string{"Content-Type", "Authorization"},
      AllowCredentials: false,
  }))
  // ... routes
  ```
  Without this, the browser blocks every API call and SSE connection from the frontend. Two minutes to add; 30 minutes to debug if forgotten.
- [ ] Stub all routes returning `200 OK`
- [ ] `cmd/server/main.go`:
  - Connect pgx pool
  - Run migrations on startup (or separately — your call, document the decision)
  - Instantiate stores, runtime (based on `MAESTRO_RUNTIME` env), SSE broadcaster, scheduler
  - Wire engine, channels client
  - Start HTTP server + scheduler

**Checkpoint:** `go run ./cmd/server` starts cleanly, all stub routes return 200, `/api/events` keeps the connection open and streams nothing. `docker-compose up` brings up Postgres then backend without race condition.

---

## Phase 2 — Workflow Engine (Saturday PM + evening, ~6 hours)

> **This is the riskiest and most time-consuming phase. Budget 6 hours, not 4. It is the core of the demo.**

### 2.1 — Event-driven execution engine

- [ ] `internal/workflow/engine.go`: implement `Engine.Execute` and `Engine.runNode` (see CLAUDE.md for full implementation)
  - Entry: find `is_entry = true` node, dispatch via goroutine
  - Per step: `e.workflows.CheckGuardrails(ctx, ag.ID, ag.Guardrails)` (fail fast if cost/rate limit exceeded — routes cost queries through the workflow store, no separate costStore field on Engine) → run runtime (with 60s timeout ctx) → handle WhatsApp action if `ag.HasChannel("whatsapp")` → persist message → evaluate edges (first-match, priority ASC)
  - Iteration guard: `IncrementIterationCount` → fail if > MAX_ITERATIONS
  - Step timeout: `context.WithTimeout(ctx, 60s)` — map `ErrStepTimeout` → `timed_out` status
  - Terminal detection: no matching outgoing edge → `completed`
  - All SSE events published at each transition: `ExecutionStarted`, `AgentStarted`, `AgentCompleted`, `MessageDispatched`, `WhatsAppSent`, `ExecutionCompleted`, `ExecutionFailed`, `StepTimedOut`

- [ ] `internal/workflow/conditions.go`: `evaluateCondition(output, condition string) bool`
  - "always" / "" → true
  - "approved" → case-insensitive contains "APPROVED"
  - "rejected" → case-insensitive contains "REJECTED"
  - default → case-sensitive substring match

- [ ] `internal/workflow/whatsapp_action.go`: `parseWhatsAppAction(line string) (to, message string, ok bool)`
  - Parses lines matching `ACTION:WHATSAPP: +1234567890 | message text`
  - The `ACTION:` namespace prefix prevents false positives when agents like Connector Scout describe a PSP's webhook format and happen to mention "WHATSAPP:"
  - Called per-line in `handleWhatsAppAction`, which is itself only invoked when `agent.HasChannel("whatsapp")` is true

### 2.2 — Fill in API handlers

- [ ] `internal/api/agent_handler.go`: CRUD + memory + skills endpoints (JSON in/out, proper error codes)
- [ ] `internal/api/workflow_handler.go`:
  - CRUD for workflows/nodes/edges
  - `POST /api/workflows/{id}/execute` → calls `engine.Execute` in a goroutine, returns `{"executionId": "..."}` immediately (fire-and-forget; status comes via SSE)
- [ ] `internal/api/execution_handler.go`:
  - `GET /api/executions/{id}` — full execution status
  - `GET /api/executions/{id}/messages` — agent message timeline
  - `GET /api/executions/{id}/logs` — execution log stream
  - `DELETE /api/executions/{id}` — cancel running execution (set status to "failed", cancel context)
- [ ] `internal/api/template_handler.go`:
  - `GET /api/templates` — reads JSON files from `backend/templates/` directory
  - `POST /api/templates/{id}/load` — reads template JSON, creates workflow + agents + nodes + edges in DB, returns new workflow ID

### 2.3 — Mock payments API

- [ ] `internal/api/mock_payments_handler.go`:
  - `GET /api/mock/failed-transactions` — returns a hardcoded JSON array of 3-5 failed transaction objects:
    ```json
    [
      // Use time.Now().Add(-1*time.Hour) and time.Now().Add(-55*time.Minute) for failed_at
      // so timestamps always read as "recent" regardless of when the demo runs.
      {"id": "txn_001", "amount": 99.99, "currency": "USD", "customer_phone": "+14155551234", "failure_reason": "insufficient_funds", "provider": "stripe", "failed_at": "<1 hour ago>"},
      {"id": "txn_002", "amount": 249.00, "currency": "USD", "customer_phone": "+14155555678", "failure_reason": "card_declined", "provider": "adyen", "failed_at": "<55 min ago>"}
    ]
    ```
  - No auth, no state — pure in-memory fixture. The Transaction Monitor agent hits this endpoint.

### 2.4 — Template seed data

- [ ] `backend/templates/nova-recovery.json`: full workflow definition with 3 nodes, 2 edges (Monitor→Orchestrator and Orchestrator→Reporter, both "always"), agent configs with system prompts. The Reporter is a terminal node with no outgoing edge.
  - Recovery Orchestrator system prompt: *"When contacting a customer via WhatsApp, output a line in this exact format: 'ACTION:WHATSAPP: {phone_number} | {message}'"*
  - Transaction Monitor system prompt must handle both runtimes gracefully:
    *"If you have HTTP tools available, call GET http://localhost:8080/api/mock/failed-transactions and summarize the response as a JSON array. Otherwise, reason about 2-3 plausible failed transactions (card_declined, insufficient_funds, expired_card) with realistic amounts and phone numbers, and produce the same structured output."*
    This ensures the template works with AnthropicDirectRunner (Docker default, no HTTP tools) and GooseRunner with developer extension (can actually hit the endpoint).
- [ ] `backend/templates/connector-integration.json`: 3 nodes, **3 edges** (Scout→Builder always/p0, Builder→Reviewer always/p0, Reviewer→Builder rejected/p0). **No "approved" edge.** When the Reviewer outputs "APPROVED", the "rejected" condition does not match, no edge fires, and the engine's `no matching edge = terminal` rule marks execution completed. This avoids a phantom terminal node or a nullable `target_node_id` FK. Reviewer system prompt ends with: *"Your final line MUST be exactly 'APPROVED' or 'REJECTED: {reason}' — no other format is accepted."*

**Checkpoint:** `POST /api/templates/nova-recovery/load` creates workflow + agents in DB. `POST /api/workflows/{id}/execute` triggers the engine. Backend logs show Goose (or direct API) being called per agent step. `agent_messages` rows appear in DB. SSE stream sends events (verify via `curl -N http://localhost:8080/api/events`).

---

## Phase 3 — External Channel + Scheduler (Sunday AM, ~3 hours)

### 3.1 — Twilio WhatsApp integration

- [ ] `internal/channels/whatsapp.go`:
  ```go
  type WhatsAppClient interface {
      Send(ctx context.Context, to, message string) error
  }

  type TwilioClient struct { /* twilio-go client */ }
  func (c *TwilioClient) Send(ctx context.Context, to, message string) error { ... }

  type NoopClient struct{} // for testing
  func (c *NoopClient) Send(ctx context.Context, to, message string) error { return nil }
  ```
- [ ] `internal/channels/parse.go`:
  - `ParseTwilioWebhook(r *http.Request) (from, body string, err error)` — parses form-encoded body
  - **Skip HMAC validation** — add `// TODO: validate X-Twilio-Signature in production` comment

- [ ] `internal/api/webhook_handler.go` — `POST /api/webhooks/whatsapp`:
  1. Parse inbound message (from, body)
  2. Find agent with "whatsapp" in channels (query agents table, scan JSONB)
  3. Create ad-hoc execution: `workflow_id=NULL, execution_type='adhoc', triggered_by='whatsapp'`
  4. Run agent: `engine.runtime.Run(ctx, agent, body)`
  5. Reply: `twilio.Send("whatsapp:"+from, response)`
  6. Publish SSE: `ExternalMessageReceived`
  7. Return a 200 response. If returning TwiML (`<Response/>`), set `Content-Type: text/xml` — if you accidentally return `Content-Type: application/json`, Twilio will log warnings. An empty body with status 200 and no Content-Type also works for the sandbox.

- [ ] End-to-end WhatsApp test (manual):
  - Send "hello" from your phone to the Twilio sandbox number
  - Confirm backend receives it, runs the Recovery Orchestrator, replies
  - Confirm SSE event appears in browser console

### 3.2 — Scheduler

- [ ] `internal/scheduler/scheduler.go`:
  - Wraps `robfig/cron` v3
  - `New(engine Engine, agentStore agent.Store, workflowStore workflow.Store) *Scheduler`
  - `Start(ctx context.Context)`:
    - Load all `agent_schedules WHERE enabled = true`
    - For each: register cron job that calls `engine.Execute` with the schedule's `task_prompt` as an ad-hoc single-agent run
    - Store job IDs in a map for later removal/reload
  - `Reload(ctx)` — called by the API when a schedule is created or updated; re-registers all jobs
  - `UpdateLastRun(ctx, scheduleID)` — called after each tick; writes `last_run` and computes `next_run`

- [ ] Wire scheduler into `main.go`: `scheduler.Start(ctx)` in a goroutine after server starts

### 3.3 — Validate full end-to-end

- [ ] Trigger Template 2 manually via API
- [ ] Confirm Monitor → Orchestrator → Reporter message chain in `agent_messages` DB table
- [ ] Confirm WhatsApp outbound message arrives on phone
- [ ] Reply from phone → confirm backend receives via webhook, responds
- [ ] Confirm all SSE events appear in browser console (`curl -N http://localhost:8080/api/events`)
- [ ] Confirm execution reaches `completed` status in DB
- [ ] Trigger Template 1 with "Stripe" — confirm Reviewer rejects once, Builder revises, Reviewer approves within 5 iterations

---

## Phase 4 — Frontend (Sunday AM/PM, ~4 hours)

### 4.1 — Scaffold Next.js

- [ ] `cd frontend && npx create-next-app@14 . --typescript --tailwind --app --no-src-dir`
- [ ] Install dependencies:
  ```bash
  npm install reactflow @radix-ui/react-dialog @radix-ui/react-select @radix-ui/react-tabs
  npm install lucide-react clsx
  ```
- [ ] `lib/api.ts`: typed fetch wrappers for all backend endpoints. All requests to `process.env.NEXT_PUBLIC_API_URL`.
- [ ] `lib/sse.ts`: `useSSE(executionID?: string)` hook
  - Connects to `GET /api/events?executionId=<id>`
  - Dispatches typed events to state
  - Cleans up on unmount (calls `EventSource.close()`)
  - TypeScript discriminated union for all event types

### 4.2 — Agent management UI

- [ ] `app/agents/page.tsx`: card grid of agents. "New Agent" opens a modal.
- [ ] `components/agents/AgentModal.tsx` — modal with tabs:
  - **Basic**: Name, Role, System Prompt (textarea), Model (text input)
  - **Memory**: key-value editor (add/remove rows, "Save Memory" button)
  - **Guardrails**: max tokens per run (number input), max runs per hour (number input)
  - *(Skills and Schedules tabs: render the tab but mark as "coming soon" if time is short — see Time Budget section)*
- [ ] `app/agents/[id]/page.tsx`: agent detail — config summary + recent message history

### 4.3 — Workflow builder (React Flow)

- [ ] `app/workflows/page.tsx`: list of workflows, "New Workflow" button, "Browse Templates" link
- [ ] `app/workflows/[id]/page.tsx`:
  - Full-page React Flow canvas
  - **"Add Agent Node" button** (top-left of canvas): opens a dropdown of existing agents → clicking one adds a node at a default position (x: 100 + offset, y: 200). User drags to reposition.
    *Rationale: Custom drag-source-to-canvas implementation is non-trivial in React Flow. A button+dropdown achieves the same functional result in a fraction of the time.*
  - Each node displays: agent name, role badge, WhatsApp icon if "whatsapp" in channels
  - Click an edge → inline condition selector (dropdown: "always" / "approved" / "rejected" / custom text input)
  - Priority is set automatically: edges from the same source are numbered 0, 1, 2... in the order they were added. UI shows a small priority number badge on each edge.
  - Top bar: workflow name (editable inline), "Save", "Run Workflow" button
  - "Run Workflow" → `POST /api/workflows/{id}/execute` → navigate to `/monitor/{executionId}`

### 4.4 — Live monitoring dashboard

**Simplified from original design to a two-panel layout** (cuts significant frontend time while keeping all information visible):

- [ ] `app/monitor/[executionId]/page.tsx`:
  - **Left panel (1/3 width)**: agent status cards (one per node in the workflow)
    - Each card: agent name, status badge (idle / running / completed / error / timed_out), updated in real time via SSE
    - Token/cost tracker at the bottom: total tokens in + out, estimated cost, source ("goose_json" or "estimated")
  - **Right panel (2/3 width)**: unified event timeline — a single scrolling log that interleaves:
    - Inter-agent messages (with sender → receiver header)
    - External channel messages (WhatsApp icon + phone number)
    - Execution log entries (info / warn / error with color coding)
    - All driven by SSE events, newest at bottom, auto-scroll
  - Top bar: execution status badge, elapsed time (updated every second), "Stop" button → `DELETE /api/executions/{id}`

### 4.5 — Templates browser

- [ ] `app/templates/page.tsx`: two cards:
  - **"Payment Connector Integration Pipeline"** — description: "Mirrors Yuno's PSP connector onboarding workflow: a Scout researches the API, a Builder generates the Go adapter, a Compliance Reviewer checks for PCI DSS gaps. The Reviewer's feedback loops back to the Builder until the adapter passes review."
  - **"Failed Transaction Recovery Pipeline (NOVA)"** — description: "A miniaturized version of Yuno's NOVA product. A Transaction Monitor polls for failed transactions, a Recovery Orchestrator contacts customers via WhatsApp, and a Reconciliation Reporter summarizes outcomes."
  - Each card: brief description, "Load Template" button → `POST /api/templates/{id}/load` → redirect to new workflow

**Checkpoint:** Full happy-path demo works end-to-end in the browser. Agent CRUD, template load, workflow builder, execution trigger, WhatsApp message round-trip, monitoring dashboard live-updating.

---

## Phase 5 — Tests (Sunday PM, ~1.5 hours)

### Backend tests

Run tests against the **Docker Compose PostgreSQL instance** (already running on `localhost:5432` during development). Create a separate test database inside that same container:

```bash
# While docker-compose up is running:
docker exec -it maestro-postgres-1 createdb -U maestro maestro_test
# Or, if you have psql installed locally and Docker Compose exposes port 5432:
createdb -h localhost -p 5432 -U maestro maestro_test
```

Set `DATABASE_URL_TEST=postgres://maestro:maestro@localhost:5432/maestro_test` in your shell or `.env.test`. Do **not** create a separate local PostgreSQL installation — that will conflict with the Docker Compose port binding.

- [ ] `internal/agent/store_test.go`: agent CRUD round-trip (create, get, update, delete)
- [ ] `internal/workflow/conditions_test.go`:
  - "always" matches anything
  - "approved" matches output containing "APPROVED" (case-insensitive)
  - "rejected" matches "REJECTED" but not "APPROVED"
  - Output containing both → first-match wins (test with priority-ordered edge slice)
  - Arbitrary substring
- [ ] `internal/workflow/engine_test.go`:
  - Mock `Runner` that returns canned outputs
  - Linear 2-node workflow: entry runs, output dispatched to second node, execution completes
  - Cyclic workflow: Reviewer returns "REJECTED" 6 times → execution fails with "max iterations exceeded" after 5 steps (the Reviewer→Builder "rejected" p0 edge fires repeatedly)
  - Approval path: Reviewer returns "APPROVED" → no edge matches → execution status set to "completed" (not failed) — verify this is the terminal/success path
  - Step timeout: mock runner sleeps 2s, timeout set to 1s → execution status `timed_out`
- [ ] `internal/channels/parse_test.go`: Twilio webhook body parsing

### Frontend tests

- [ ] `lib/sse.test.ts`: hook connects, receives typed events, closes on unmount
- [ ] `components/agents/AgentModal.test.tsx`: renders, form submission calls correct API endpoint

---

## Phase 6 — Documentation + Demo Recording (Sunday PM, ~1.5 hours)

### DECISION_LOG.md — write one entry per decision

- [ ] Why Goose over OpenClaw/OpenCode (most mature extension API; Block's fintech credibility)
- [ ] Why Go backend (Yuno's primary language; goroutines for concurrent agent execution; explicit control)
- [ ] Why event-driven executor over topological sort (handles cycles in Template 1; mirrors Yuno's event-driven payment routing)
- [ ] Why SSE over WebSocket (unidirectional monitoring stream; simpler implementation; no handshake overhead)
- [ ] Why Twilio Sandbox over Meta API direct / unofficial libraries (no approval delays; legitimate; re-used existing Twilio experience)
- [ ] Why `Runner` interface with two implementations (GooseRunner + AnthropicDirectRunner) rather than coupling to one (resilience; testability with mocks; explicit fallback path)
- [ ] Why Chi over Gin/Echo (idiomatic Go; close to stdlib; no magic; easier to reason about in financial contexts)
- [ ] Why raw SQL over GORM (explicit control; easier debugging; no ORM magic hiding query behavior — consistent with how you'd want to audit financial data access)
- [ ] Why first-match edge semantics over all-match (deterministic; avoids fan-out on ambiguous output; easier to reason about in complex cyclic workflows)
- [ ] Why skip Twilio HMAC validation in demo (time budget; not a correctness or functionality concern; production concern clearly labeled with TODO)
- [ ] Why two-panel monitoring layout over three-panel (cut significant frontend time; all information still visible; single timeline easier to follow during a live demo)

### README.md

- [ ] Project name and one-sentence description
- [ ] **Yuno context section** (first paragraph after description): "Built as a hiring assessment for Yuno. Both workflow templates are domain-specific: Template 2 is a miniaturized version of Yuno's NOVA AI payment recovery product; Template 1 mirrors Yuno's PSP connector onboarding workflow. Architecture choices (Go, PostgreSQL, event-driven execution) deliberately mirror Yuno's stack."
- [ ] Architecture diagram (Mermaid):
  ```mermaid
  graph LR
    WA[WhatsApp] -->|inbound message| TW[Twilio Sandbox]
    TW -->|webhook POST| BE[Go Backend :8080]
    FE[Next.js Frontend :3000] -->|REST API| BE
    FE -->|SSE stream| BE
    BE -->|spawns subprocess| GS[Goose CLI]
    GS -->|LLM calls| AN[Anthropic API]
    BE -->|direct fallback| AN
    BE -->|reads/writes| PG[(PostgreSQL :5432)]
    BE -->|outbound message| TW
    TW -->|WhatsApp reply| WA
  ```
- [ ] Setup instructions:
  1. `cp .env.example .env` and fill in keys
  2. Install Goose CLI (link to docs)
  3. `ngrok http 8080` → copy URL → set in Twilio webhook console
  4. `docker-compose up`
  5. Open http://localhost:3000
- [ ] Runtime choice justification: 2-3 paragraphs on Goose (Block's fintech credibility, extension API, provider-agnostic) and the fallback strategy
- [ ] How to add a new workflow template: create a JSON file in `backend/templates/`, follow the schema of existing templates
- [ ] How to add a new messaging channel: implement the `WhatsAppClient` interface in `internal/channels/`, register in the webhook router

### Demo recording

- [ ] Clean environment: `docker-compose down -v && docker-compose up` from scratch
- [ ] Open OBS or QuickTime (screen + mic)
- [ ] Follow demo script from CLAUDE.md exactly — it's structured for maximum impact with a Yuno interviewer
- [ ] Cover both templates in under 2 minutes
- [ ] Upload as MP4 or GIF (< 100MB); link in README

---

## Phase 7 — Polish + Ship (Sunday evening, ~1 hour)

- [ ] Final README pass: spell-check, all links work, setup instructions verified from scratch in a fresh terminal session
- [ ] Confirm `docker-compose up` works from a clean state (no pre-existing volumes): `docker-compose down -v && docker-compose up`
- [ ] Verify `.env.example` has placeholder values only — no real API keys
- [ ] Check `goose-test-output.json` is committed (documents Phase 0 verification)
- [ ] Tag release: `git tag v1.0.0 && git push --tags`
- [ ] Verify repo is accessible at the GitLab URL
- [ ] Send to Yuno contact with a brief note: "Both workflow templates are themed around Yuno's engineering challenges — Template 2 is a miniaturized NOVA, Template 1 mirrors your PSP connector onboarding flow."

---

## Time Budget Summary

| Phase | Original Estimate | Revised Estimate | Notes |
|---|---|---|---|
| 0 — Pre-flight + Goose verification | 1h | 2h | Goose verification is critical path; don't rush it |
| 1 — Backend foundation | 4h | 4h | Solid estimate |
| 2 — Workflow engine | 4h (original) | 6h | Most complex phase; event-driven executor + SSE is the core |
| 3 — External channel + scheduler | 3h | 3h | Solid estimate; skip HMAC validation |
| 4 — Frontend | 4h | 4h | Two-panel monitor layout (simplified from three-panel) |
| 5 — Tests | 1.5h | 1.5h | Local Postgres (no testcontainers); mock Runner |
| 6 — Docs + demo | 1.5h | 1.5h | Demo script pre-written in CLAUDE.md |
| 7 — Polish + ship | 1h | 1h | |
| **Total** | **20h** | **23h** | |

## If Time Gets Tight — Cut in This Order

1. **Skills tab in AgentModal** — keep the DB model and API, just don't render the tab in the UI. "Coming soon" placeholder is fine.
2. **Schedules tab in AgentModal** — same: keep backend, hide frontend tab.
3. **Template 1 (Connector Integration)** — ensure it loads and displays correctly in the UI, but demo only Template 2 (NOVA Recovery) during the recording. Template 1 can be listed as available.
4. **Frontend tests** — keep backend tests; drop `AgentModal.test.tsx`.
5. **Stop button** in monitoring dashboard — omit `DELETE /api/executions/{id}`.

**Never cut:**
- The Template 2 end-to-end demo (WhatsApp + monitoring dashboard) — this is the 40% weight demo criterion AND the Yuno differentiator
- The PostgreSQL health check in docker-compose — this will bite you in the demo
- Phase 0 Goose verification — discovering a broken CLI integration on Saturday afternoon is catastrophic