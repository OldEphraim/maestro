# CLAUDE.md — Maestro

## Project Context

Maestro is an AI agent orchestration platform built as a hiring assessment for **Yuno** (y.uno), a payment orchestration company that routes transactions across 450+ providers in 195 countries through a single API. Yuno's hardest engineering problems — normalizing heterogeneous provider APIs, routing decisions under latency pressure, recovering failed transactions — are the inspiration for Maestro's two built-in workflow templates.

The platform lets users create AI agents, configure their behavior (personality, tools, schedules, memory, guardrails), and connect them into visual multi-agent workflows. Agents run on a real runtime (Goose), communicate asynchronously via a persistent message bus, and at least one agent is reachable through WhatsApp (Twilio Sandbox) so a human can converse with it in real time.

**All architectural decisions should be made with Yuno's stack as the reference point:** Go microservices, PostgreSQL, event-driven async communication, AWS. When in doubt, choose the option that a Yuno backend engineer would recognize and respect.

---

## Repository Structure

```
maestro/
├── backend/                  # Go backend
│   ├── cmd/
│   │   └── server/
│   │       └── main.go       # Entry point
│   ├── internal/
│   │   ├── agent/            # Agent domain: CRUD, config, memory
│   │   ├── workflow/         # Workflow engine: event-driven executor, message passing
│   │   ├── runtime/          # Goose integration layer + Anthropic direct fallback
│   │   ├── scheduler/        # Cron job runner for scheduled agents
│   │   ├── channels/         # External channel adapters (WhatsApp/Twilio)
│   │   ├── sse/              # SSE event broadcaster
│   │   ├── db/               # PostgreSQL connection, migrations
│   │   └── api/              # HTTP handlers, router (Chi)
│   ├── migrations/           # SQL migration files (golang-migrate)
│   ├── templates/            # Seed data for built-in workflow templates (JSON)
│   └── Dockerfile
├── frontend/                 # Next.js 14 + TypeScript
│   ├── app/                  # App Router pages
│   │   ├── agents/           # Agent list + CRUD UI
│   │   ├── workflows/        # Workflow builder (React Flow) + execution
│   │   ├── monitor/          # Live monitoring dashboard (SSE consumer)
│   │   └── templates/        # Template browser and loader
│   ├── components/
│   │   ├── flow/             # React Flow node/edge components
│   │   ├── agents/           # Agent form, memory editor, guardrail config
│   │   └── monitor/          # Log stream, message timeline, token tracker
│   └── lib/
│       ├── api.ts            # Backend API client
│       └── sse.ts            # SSE hook
├── docker-compose.yml        # PostgreSQL + backend + frontend, single command
├── .env.example
├── CLAUDE.md                 # This file
├── STEPS.md                  # Build log
├── DECISION_LOG.md           # Architectural decisions with rationale
└── README.md                 # Architecture diagram, setup, runtime justification
```

---

## Stack

| Layer | Technology | Rationale |
|---|---|---|
| Backend language | Go | Matches Yuno's primary backend language. Goroutines are ideal for concurrent agent execution and the SSE broadcaster. |
| Database | PostgreSQL | Yuno's system-of-record database. ACID guarantees matter for workflow state and agent message history. |
| HTTP router | Chi | Lightweight, idiomatic Go. No framework magic. |
| Migrations | golang-migrate | SQL-first migrations, no ORM magic. |
| Agent runtime | Goose (Block) | Best-documented extension API, provider-agnostic, session management. Block built Square and Cash App — respected in fintech. |
| External channel | WhatsApp via Twilio Sandbox | Legitimate API (not a scraper), 5-minute setup, demonstrates real-world integration. |
| Frontend | Next.js 14 + TypeScript | App Router, fast iteration. |
| Workflow visualization | React Flow | The standard for node-based workflow UIs. Handles graph state, drag-and-drop, and edge conditions cleanly. |
| Real-time | SSE (Server-Sent Events) | Simpler than WebSocket for unidirectional server→client streams (monitoring, log tailing, agent status). |
| Local orchestration | Docker Compose | Single `docker-compose up` runs everything. PostgreSQL, backend, frontend. |

---

## Database Schema

```sql
-- Core agent configuration
-- The model field always stores the canonical Anthropic API model string
-- (e.g. 'claude-sonnet-4-5-20250929'), NOT Goose's format.
-- GooseRunner strips the date suffix at runtime: 'claude-sonnet-4-5-20250929' → 'claude-sonnet-4-5'.
-- AnthropicDirectRunner uses the stored string as-is.
-- This way switching MAESTRO_RUNTIME never requires a data migration.
agents (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  system_prompt TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT 'claude-sonnet-4-5-20250929',
  tools JSONB DEFAULT '[]',          -- list of enabled tool names
  channels JSONB DEFAULT '[]',       -- e.g. ["whatsapp", "internal"]
  guardrails JSONB DEFAULT '{}',     -- { max_tokens_per_run, max_runs_per_hour, blocked_actions[] }
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
)

-- Persistent memory per agent (key-value, injected into system prompt at runtime)
agent_memory (
  id UUID PRIMARY KEY,
  agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(agent_id, key)
)

-- Reusable skills (step-by-step procedures injected as tool instructions)
agent_skills (
  id UUID PRIMARY KEY,
  agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT,
  steps JSONB NOT NULL              -- ordered list of step strings
)

-- Cron schedules per agent
agent_schedules (
  id UUID PRIMARY KEY,
  agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
  cron_expr TEXT NOT NULL,
  task_prompt TEXT NOT NULL,        -- what to tell the agent when it wakes
  enabled BOOLEAN DEFAULT TRUE,
  last_run TIMESTAMPTZ,
  next_run TIMESTAMPTZ
)

-- Workflow definitions (the graph)
workflows (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  template_id TEXT,                 -- e.g. "connector-integration", "nova-recovery"
  status TEXT DEFAULT 'draft',      -- draft | active | archived
  created_at TIMESTAMPTZ DEFAULT NOW()
)

-- Nodes in a workflow (each node = one agent instance)
workflow_nodes (
  id UUID PRIMARY KEY,
  workflow_id UUID REFERENCES workflows(id) ON DELETE CASCADE,
  agent_id UUID REFERENCES agents(id),
  label TEXT,
  position_x FLOAT,
  position_y FLOAT,
  is_entry BOOLEAN DEFAULT FALSE    -- exactly one node per workflow should be true
)

-- Directed edges between nodes (with conditions and priority for first-match evaluation)
workflow_edges (
  id UUID PRIMARY KEY,
  workflow_id UUID REFERENCES workflows(id) ON DELETE CASCADE,
  source_node_id UUID REFERENCES workflow_nodes(id) ON DELETE CASCADE,
  target_node_id UUID REFERENCES workflow_nodes(id) ON DELETE CASCADE,  -- cascade so deleting a node cleans up its edges
  condition TEXT,                   -- "always" | "approved" | "rejected" | arbitrary substring
  priority INT DEFAULT 0            -- edges evaluated in ASC order; first match wins
)

-- A single run of a workflow OR an ad-hoc single-agent conversation.
-- workflow_id is nullable: NULL means this was triggered ad-hoc (e.g. inbound WhatsApp message
-- with no running workflow). execution_type distinguishes the two cases.
workflow_executions (
  id UUID PRIMARY KEY,
  workflow_id UUID REFERENCES workflows(id),   -- nullable
  execution_type TEXT DEFAULT 'workflow',       -- "workflow" | "adhoc"
  status TEXT DEFAULT 'running',               -- running | completed | failed | timed_out
  triggered_by TEXT,                           -- "manual" | "schedule" | "whatsapp"
  iteration_count INT DEFAULT 0,               -- total agent steps fired; guards against cycles
  started_at TIMESTAMPTZ DEFAULT NOW(),
  completed_at TIMESTAMPTZ
)

-- Messages passed between agents (the async message bus).
-- execution_id is always set in practice (webhook handler creates execution first),
-- but declared nullable as a safety measure.
agent_messages (
  id UUID PRIMARY KEY,
  execution_id UUID REFERENCES workflow_executions(id),
  from_agent_id UUID REFERENCES agents(id),
  to_agent_id UUID REFERENCES agents(id) NULL,  -- NULL when the recipient is the implicit next node
  content TEXT NOT NULL,
  channel TEXT DEFAULT 'internal',  -- "internal" | "whatsapp"
  status TEXT DEFAULT 'pending',    -- pending | delivered | processed
  created_at TIMESTAMPTZ DEFAULT NOW()
)

-- Structured execution logs per agent per run
execution_logs (
  id UUID PRIMARY KEY,
  execution_id UUID REFERENCES workflow_executions(id),
  agent_id UUID REFERENCES agents(id),
  level TEXT DEFAULT 'info',        -- info | warn | error
  message TEXT NOT NULL,
  metadata JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW()
)

-- Token/cost tracking per execution.
-- Populated on a best-effort basis: "goose_json" if parsed from Goose's JSON output,
-- "estimated" if approximated from character count (~4 chars per token).
execution_costs (
  id UUID PRIMARY KEY,
  execution_id UUID REFERENCES workflow_executions(id),
  agent_id UUID REFERENCES agents(id),
  tokens_in INT DEFAULT 0,
  tokens_out INT DEFAULT 0,
  estimated_cost_usd NUMERIC(10, 6) DEFAULT 0,
  source TEXT DEFAULT 'estimated',  -- "goose_json" (Goose JSON output) | "estimated" (char-count approximation) | "anthropic_api" (AnthropicDirectRunner, exact token counts)
  created_at TIMESTAMPTZ DEFAULT NOW()
)
```

---

## Goose Integration — Verified CLI Contract

> **This section documents the actual Goose CLI interface as verified against the official docs at block.github.io/goose/docs/guides/running-tasks.** The original assumption that `--non-interactive` and a `--model` flag existed was incorrect. Use only the flags documented here.

### Correct invocation pattern

```bash
goose run \
  --no-session \
  --provider anthropic \
  --model claude-sonnet-4-5 \
  --output-format json \
  -t "your full prompt here"
```

**Verified flags:**
- `-t "text"` — pass the task prompt as a string. Preferred for programmatic invocation.
- `--no-session` — discard session after the run; prevents session files from accumulating in automation.
- `--provider anthropic` — selects the Anthropic provider; reads `ANTHROPIC_API_KEY` from environment.
- `--model <n>` — overrides the configured model. Goose uses its own model name format (no date suffix). The `agents.model` column always stores the canonical Anthropic string (e.g. `claude-sonnet-4-5-20250929`). `GooseRunner` calls `gooseModelName(m string) string` to strip the date suffix before passing this flag: `claude-sonnet-4-5-20250929` → `claude-sonnet-4-5`. `AnthropicDirectRunner` uses `agents.model` as-is. Switching `MAESTRO_RUNTIME` never requires a data migration.
- `--output-format json` — emits structured JSON to stdout. Required for reliable parsing. Alternative: `--output-format stream-json` for streaming events.
- `--with-builtin developer` — enables the developer extension (file I/O, shell execution). Add for agents that need tool access.
- `--debug` — verbose tool output. Useful during development.

**Stdin alternative** (when the prompt is too long for a `-t` flag):
```bash
echo "your prompt" | goose run --no-session --provider anthropic \
  --model claude-sonnet-4-5 --output-format json -i -
```

### ⚠️ Phase 0 mandatory verification (do this before writing any other code)

```bash
ANTHROPIC_API_KEY=your-key goose run \
  --no-session \
  --provider anthropic \
  --model claude-sonnet-4-5 \
  --output-format json \
  -t "Reply with exactly the word: PONG"
```

1. Capture the full raw stdout and save it to `goose-test-output.json`
2. Identify the field that contains the model's text response (likely `"response"` or `"content"` — confirm)
3. Check whether a `"usage"` field with token counts is present
4. Verify the exit code is 0 on success
5. Update the `GooseOutput` struct in `internal/runtime/goose.go` to match the actual field names
6. Commit `goose-test-output.json` to the repo as documentation

If this test fails (wrong model string, auth error, unexpected output format), **immediately switch to the direct Anthropic fallback** (see below) rather than debugging Goose CLI for hours.

### Go subprocess implementation

*Full implementation sketch (`GooseRunner`, `GooseOutput`, `parseOutput`, `estimateUsage`) in STEPS.md §Phase 1.5.*

### Fallback: direct Anthropic API

If Goose CLI invocation proves unreliable, switch to `MAESTRO_RUNTIME=anthropic_direct`. This calls the Anthropic API directly from Go, eliminating the subprocess dependency.

Key design note: `GooseRunner` calls `buildFullPrompt(ag, task)` (task baked into one `-t` string). `AnthropicDirectRunner` calls `buildSystemPrompt(ag)` for the `system` field and passes `task` as the user message — using the API's native system/user split. Both helpers live in `internal/runtime/prompt.go`.

Both runtimes read `agent.Model` from the database (canonical Anthropic string, e.g. `claude-sonnet-4-5-20250929`). `GooseRunner` strips the date suffix via `gooseModelName()` before passing `--model`. `AnthropicDirectRunner` uses it as-is. Switching runtimes never requires a data migration.

*Full implementation sketches (`AnthropicDirectRunner`, `Runner` interface, `main.go` runtime selection) in STEPS.md §Phase 1.5.*

---

## Workflow Execution Model — Event-Driven, Not Topological Sort

**Do not use topological sort.** Topological sort is defined only for directed acyclic graphs (DAGs). Template 1 contains a deliberate cycle (Reviewer → Builder on rejection), which would cause `TopologicalSort` to return an error and prevent the template from ever executing.

**Use an event-driven executor** instead — this is also architecturally correct for Maestro's domain. It mirrors Yuno's payment routing model: events arrive, conditions are evaluated, and the next hop is dispatched based on the output. No static pre-ordering; the graph is traversed dynamically.

### Execution model

```
1. Create a workflow_executions record (status: running).
2. Find the entry node (workflow_nodes.is_entry = true).
3. Run the entry node with the initial task prompt.
4. After each node completes:
   a. Persist output as agent_message.
   b. Increment execution.iteration_count.
   c. If iteration_count > MAX_ITERATIONS: mark execution failed ("max iterations exceeded").
   d. Load outgoing edges for this node, sorted by priority ASC.
   e. Evaluate each edge's condition against the output (first-match semantics — stop at first match).
   f. If a match is found: dispatch to the target node asynchronously (goroutine).
   g. If no match: this is a terminal node — mark execution completed.
5. Each node runs in its own goroutine. The SSE broadcaster publishes events after each step.
```

### Go implementation sketch

*Full implementation (`Engine`, `Execute`, `runNode`, `handleWhatsAppAction`) in STEPS.md §Phase 2.1.*

### Edge condition evaluation — first-match semantics

Outgoing edges are sorted by `priority ASC`. The **first** edge whose condition matches is followed; subsequent edges are not evaluated. This prevents ambiguous routing when output contains multiple keywords.

`evaluateCondition` matches: `"always"`/`""` → true; `"approved"` → case-insensitive contains "APPROVED"; `"rejected"` → case-insensitive contains "REJECTED"; default → case-sensitive substring. *Full function in STEPS.md §Phase 2.1.*

**Template 1 edge priorities:**
- Scout → Builder: condition "always", priority 0
- Builder → Reviewer: condition "always", priority 0
- Reviewer → Builder: condition "rejected", priority 0   ← only edge out of Reviewer

There is **no** "approved" edge. When the Reviewer outputs "APPROVED", the "rejected" condition does not match, no outgoing edge fires, and the engine's `no matching edge = terminal` rule marks the execution completed. This avoids the schema problem of a `target_node_id` FK pointing to a phantom "terminal" node. The Reviewer's system prompt must still end with exactly "APPROVED" or "REJECTED: {reason}" so the condition check is deterministic.

The Reviewer's system prompt must end with: *"Your final line MUST be exactly 'APPROVED' or 'REJECTED: {reason}' — no other format is accepted."*

### Step timeout — the Yuno analogy

Every agent step has a **60-second hard timeout** via `context.WithTimeout`. If Goose doesn't return within 60 seconds, the step is marked `timed_out` and the execution fails. This is directly analogous to how Yuno handles PSP timeouts — don't wait forever for a hung provider; fail fast, surface the error, and let the operator decide whether to retry. Mention this analogy explicitly in the demo.

---

## WhatsApp / Twilio Sandbox — Inbound Message Flow

When an inbound WhatsApp message arrives at `POST /api/webhooks/whatsapp`:

1. Parse the Twilio form-encoded webhook body (fields: `From`, `Body`, `To`, etc.)
2. **Skip HMAC signature validation for the demo** — add `// TODO: validate X-Twilio-Signature in production` comment. HMAC validation requires computing a signature over the full request URL + sorted params, and debugging HMAC mismatches with ngrok URLs is a time sink not worth it for a demo. Yuno engineers will understand this is a production concern, not a scope failure.
3. Find the agent configured with `"whatsapp"` in its `channels` JSONB array.
4. Create an ad-hoc execution: `workflow_id = NULL`, `execution_type = 'adhoc'`, `triggered_by = 'whatsapp'`. This ensures every inbound WhatsApp conversation appears in the monitoring dashboard even without a running workflow.
5. Run the agent with the inbound message text as the task.
6. Send the response: `twilio.Send("whatsapp:"+fromNumber, response)`.
7. Publish SSE event: `ExternalMessageReceived` — the monitoring dashboard highlights this with a WhatsApp icon.

**ngrok requirement:** Twilio needs a public HTTPS URL to POST webhooks. Run `ngrok http 8080`, copy the URL, and set it in the Twilio sandbox webhook configuration as `https://<ngrok-url>/api/webhooks/whatsapp`. Add `NGROK_URL` to `.env` for reference.

---

## Docker Compose — PostgreSQL Health Check

The backend must not start before PostgreSQL is healthy. Use `depends_on: condition: service_healthy` and a `pg_isready` healthcheck. The backend service defaults to `MAESTRO_RUNTIME=anthropic_direct` — Goose CLI is not installed in the container.

*Full `docker-compose.yml` in STEPS.md §Phase 0.4.*

---

## Coding Conventions

- **CORS middleware is required.** The frontend runs on `:3000`, the backend on `:8080`. Without CORS, the browser blocks every API call and SSE connection. Add `github.com/go-chi/cors` to the Chi router in `cmd/server/main.go` — allow origin `http://localhost:3000`, methods GET/POST/PUT/DELETE, headers Content-Type. Two minutes to add, 30 minutes to debug if forgotten.
- **`agent.HasChannel(name string) bool`** is defined on `AgentWithMemory` (and `Agent`) as a linear scan of the `Channels []string` field: `for _, c := range a.Channels { if c == name { return true } }; return false`. It is called in `runNode` to gate the WhatsApp action handler. Claude Code will not generate this automatically — add it to `internal/agent/agent.go`.
- **No ORM.** Raw SQL with `pgx/v5`. Queries live in the domain package next to the handler that uses them.
- **Errors are values.** Wrap with `fmt.Errorf("agent.Create: %w", err)`. No panics except in `main`.
- **Context propagation.** Every function that touches the DB or spawns a subprocess takes a `context.Context` as first argument.
- **Timeouts are contexts.** Use `context.WithTimeout` for agent steps (60s) and HTTP handlers (30s). Never block indefinitely.
- **Per-execution cancel contexts (stretch goal).** Supporting `DELETE /api/executions/{id}` to stop a running execution requires a `map[uuid.UUID]context.CancelFunc` in the Engine (populated in `Execute`, cleaned up in `failExecution`/`completeExecution`). The stop button in the frontend is in the "cut if time is tight" list. If implemented, the handler calls `engine.Cancel(execID)` which invokes the stored cancel func. If not implemented, the button should be hidden rather than wired to a 404.
- **`Runner` is an interface.** Two implementations: `GooseRunner` and `AnthropicDirectRunner`. Selected by `MAESTRO_RUNTIME` env var at startup.
- **SSE events are typed.** Define a Go struct for each event type and serialize to JSON. TypeScript frontend has a matching discriminated union.
- **SSE filtering is server-side.** The `GET /api/events?executionId=<id>` handler compares `event.ExecutionID` against the query param before writing to the response stream. It does not fan out all events to all clients and rely on client-side filtering. This keeps bandwidth proportional to relevant events, not total platform activity.
- **Frontend data fetching.** Server Components for initial loads; Client Components only where interactivity is required (React Flow canvas, SSE subscriber, forms).
- **Environment variables.** All secrets in `.env`. Never hardcoded. `.env.example` documents every variable.

---

## Decision Log

Every architectural or implementation decision that is **not explicitly specified in STEPS.md** must be recorded in `DECISION_LOG.md` before moving on. This applies to Claude Code and any human contributor alike.

**When to add an entry:** Any time you make a choice between two or more plausible options — library selection, schema tweak, API design, error handling strategy, naming convention, shortcut taken — write it down. If you find yourself thinking "I could do X or Y, I'll go with X," that's a decision log entry.

**Format** (one entry per decision):

```markdown
## [Short title]

**Date:** YYYY-MM-DD  
**Phase:** e.g. Phase 1 — Backend Foundation  
**Decision:** What you chose.  
**Alternatives considered:** What else you could have done.  
**Rationale:** Why you chose what you chose.  
**Consequences:** What this makes easier or harder going forward.
```

**Examples of decisions that must be logged:**
- Choosing between two pgx query patterns
- Deciding to run migrations on startup vs. as a separate CLI step
- Using `uuid.New()` vs. a DB-generated UUID
- Any deviation from the schema as written in this file
- Adding a dependency not listed in STEPS.md §1.1
- Simplifying or skipping any checklist item
- Any frontend state management choice (local state vs. context vs. external library)

STEPS.md §6 lists decisions that were made during planning and should be pre-populated in `DECISION_LOG.md` at the start of the project. All subsequent decisions go there as they are made.

---

## Testing Strategy

Every phase should produce tests for the code written in that phase — don't save testing for Phase 5. The goal is confidence at each checkpoint, not 100% coverage.

**Testing philosophy:** Test behavior, not implementation. A test should break when the system stops doing the right thing, not when the internal wiring changes. Prefer integration-style tests (real DB, real store) over mocks where practical; use mocks for the `Runner` interface and external channels (Twilio, Goose subprocess) since those cross a real network or process boundary.

**Per-phase testing expectations:**

| Phase | What to test |
|---|---|
| 1 — Backend foundation | Agent CRUD round-trip against real DB; migration runs cleanly; SSE handler connects and disconnects without leak |
| 2 — Workflow engine | `evaluateCondition` unit tests; engine integration test with mock `Runner`; cycle guard fires at MAX_ITERATIONS; approval path completes without error |
| 3 — External channel | Twilio webhook body parsing; `NoopClient` used in all non-Twilio tests |
| 4 — Frontend | `useSSE` hook lifecycle; `AgentModal` renders and submits |
| 5 — Full pass | Any gaps from earlier phases; end-to-end happy path if time allows |

**Test database:** Use the Docker Compose PostgreSQL instance (`localhost:5432`). Create `maestro_test` inside it:
```bash
docker exec -it maestro-postgres-1 createdb -U maestro maestro_test
```
Set `DATABASE_URL_TEST=postgres://maestro:maestro@localhost:5432/maestro_test`. Do not install a separate local PostgreSQL — it will conflict on the port.

**Never mock the database** for store tests — use the real `maestro_test` database. The store layer is thin SQL; if the SQL is wrong, a mock won't catch it.

## Environment Variables

```bash
# Database
DATABASE_URL=postgres://maestro:maestro@localhost:5432/maestro

# Anthropic (used by both Goose runtime and direct fallback)
ANTHROPIC_API_KEY=

# Runtime selection
# "goose" (default) — uses Goose CLI subprocess
# "anthropic_direct" — calls Anthropic API directly (use if Goose CLI proves flaky)
MAESTRO_RUNTIME=goose

# Goose binary path (only needed when MAESTRO_RUNTIME=goose)
# macOS (brew): /usr/local/bin/goose or /opt/homebrew/bin/goose
# Linux: ~/.local/bin/goose
GOOSE_BINARY_PATH=/usr/local/bin/goose

# Twilio WhatsApp Sandbox
TWILIO_ACCOUNT_SID=
TWILIO_AUTH_TOKEN=
TWILIO_WHATSAPP_FROM=whatsapp:+14155238886   # Twilio's sandbox number — check your console

# ngrok (for local WhatsApp webhook development)
NGROK_URL=https://xxxx.ngrok.io              # run: ngrok http 8080

# Server
PORT=8080
FRONTEND_URL=http://localhost:3000

# Workflow engine
MAX_ITERATIONS=5             # max agent steps per execution (cycle + runaway guard)
AGENT_STEP_TIMEOUT_SECS=60  # per-step hard timeout in seconds
```

---

## Workflow Templates

### Template 1: Payment Connector Integration Pipeline
*Mirrors Yuno's Core Payments PSP onboarding workflow.*

| Node | Agent | Role | Entry? | Edges |
|---|---|---|---|---|
| 1 | Connector Scout | Given a PSP name, researches its API docs, produces structured spec: auth method, endpoint patterns, error codes, idempotency support, rate limits, webhook format | YES | → Builder (always, p0) |
| 2 | Connector Builder | Receives spec, generates a Go adapter stub with Init/Charge/Refund/Webhook interface | NO | → Reviewer (always, p0) |
| 3 | Compliance Reviewer | Reviews adapter for PCI DSS surface area, rate limit handling, idempotency key propagation. Final line must be "APPROVED" or "REJECTED: {reason}" | NO | → Builder (rejected, p0) only. "APPROVED" output matches no edge → engine marks execution completed naturally. |

Max 5 iterations. Reviewer is instructed: *"Your final line MUST be exactly 'APPROVED' or 'REJECTED: {reason}'."*

### Template 2: Failed Transaction Recovery Pipeline (NOVA in miniature)
*Direct analog to Yuno's NOVA product.*

| Node | Agent | Role | Entry? | Edges |
|---|---|---|---|---|
| 1 | Transaction Monitor | Produces a structured summary of failed transactions. **When using `AnthropicDirectRunner`** (the Docker default), the agent has no HTTP tools and will simulate polling from its training knowledge — frame this in the demo as "the Monitor reasons about recent failed transactions." **When using `GooseRunner` with `--with-builtin developer`**, the agent can actually hit `http://localhost:8080/api/mock/failed-transactions`. The system prompt should include both the mock URL and the instruction: *"If you have HTTP tools available, call GET http://localhost:8080/api/mock/failed-transactions. Otherwise, reason about 2-3 plausible failed transactions and produce the same structured output."* | YES | → Orchestrator (always) |
| 2 | Recovery Orchestrator | Per transaction: decides retry / WhatsApp contact / escalate. WhatsApp contact lines: `ACTION:WHATSAPP: +1234567890 | message text` | NO | → Reporter (always) |
| 3 | Reconciliation Reporter | Aggregates outcomes, produces summary (recovered N, escalated M, retry success rate) | NO | terminal |

Linear pipeline — no cycles. Iteration cap won't trigger but is still enforced as a safety net.

---

## Demo Script

*Full step-by-step demo script in STEPS.md §Phase 6 (Demo Recording).*