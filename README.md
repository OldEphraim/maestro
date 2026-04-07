# Maestro

AI Agent Orchestration Platform. Create AI agents, configure their behavior, and connect them into visual multi-agent workflows that execute in real time.

## Yuno Context

Built as a hiring assessment for [Yuno](https://y.uno). Both workflow templates are domain-specific: **Template 2** is a miniaturized version of Yuno's **NOVA AI payment recovery product**; **Template 1** mirrors Yuno's **PSP connector onboarding workflow**. Architecture choices (Go, PostgreSQL, event-driven execution) deliberately mirror Yuno's stack.

## Architecture

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

## Stack

| Layer | Technology | Rationale |
|---|---|---|
| Backend | Go | Matches Yuno's primary language; goroutines for concurrent agent execution |
| Database | PostgreSQL | ACID guarantees for workflow state and message history |
| Router | Chi | Lightweight, idiomatic Go, no framework magic |
| Agent Runtime | Goose (Block) + Anthropic Direct fallback | Best extension API; Block's fintech credibility |
| External Channel | WhatsApp via Twilio Sandbox | Legitimate API, 5-minute setup |
| Frontend | Next.js 16 + TypeScript | App Router, fast iteration |
| Workflow Visualization | React Flow (@xyflow/react) | Standard for node-based workflow UIs |
| Real-time | SSE (Server-Sent Events) | Simpler than WebSocket for unidirectional monitoring |
| Orchestration | Docker Compose | Single `docker-compose up` runs everything |

## Setup

### Prerequisites

- Docker Desktop running
- Anthropic API key
- (Optional) Goose CLI for local `goose` runtime: `brew install block-goose-cli`
- (Optional) Twilio account for WhatsApp integration
- (Optional) ngrok for WhatsApp webhook development

### Quick Start

1. Clone and configure:
   ```bash
   git clone https://labs.gauntletai.com/alangarber/maestro.git
   cd maestro
   cp .env.example .env
   # Fill in ANTHROPIC_API_KEY (required)
   # Fill in TWILIO_* credentials (optional, for WhatsApp)
   ```

2. (Optional) Set up WhatsApp:
   ```bash
   ngrok http 8080
   # Copy the HTTPS URL and set it in the Twilio sandbox webhook configuration:
   # https://<ngrok-url>/api/webhooks/whatsapp
   # Update NGROK_URL in .env
   ```

3. Start everything:
   ```bash
   docker compose up
   ```

4. Open [http://localhost:3000](http://localhost:3000)

5. Load a template from the Templates page and run it.

### Running Locally (without Docker)

```bash
# Terminal 1: PostgreSQL
docker compose up postgres

# Terminal 2: Backend
cd backend
export $(grep -v '^#' ../.env | grep -v '^\s*$' | sed 's/ *#.*//' | sed 's/[[:space:]]*$//' | xargs)
export DATABASE_URL="postgres://maestro:maestro@localhost:5432/maestro?sslmode=disable"
export MAESTRO_RUNTIME=anthropic_direct
go run ./cmd/server

# Terminal 3: Frontend
cd frontend
NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev
```

## Runtime Choice

Maestro supports two agent runtimes, selected via the `MAESTRO_RUNTIME` environment variable:

**`goose` (default locally)** — Uses [Goose](https://block.github.io/goose/) by Block (the company behind Square and Cash App). Goose has the most mature extension API of any open-source agent framework, is provider-agnostic, and its fintech pedigree is directly relevant to Yuno's domain. The agent runs as a subprocess via `goose run --no-session --provider anthropic --output-format json`.

**`anthropic_direct` (Docker default)** — Calls the Anthropic Messages API directly from Go. This eliminates the subprocess dependency, provides exact token counts (vs. Goose's estimated totals), and works in Docker containers where Goose CLI isn't installed.

Both runtimes share the same prompt-building helpers (`buildSystemPrompt`, `buildFullPrompt`) and the same `Runner` interface, making them interchangeable. The `agents.model` column always stores the canonical Anthropic model string (e.g., `claude-sonnet-4-5-20250929`); `GooseRunner` strips the date suffix at runtime.

## Workflow Templates

### Template 1: Payment Connector Integration Pipeline

Mirrors Yuno's Core Payments PSP onboarding workflow. Three agents in a cycle:

- **Connector Scout** — Researches a PSP's API and produces a structured specification
- **Connector Builder** — Generates a Go adapter stub implementing the PaymentConnector interface
- **Compliance Reviewer** — Reviews for PCI DSS compliance, rate limiting, idempotency. Outputs "APPROVED" or "REJECTED: {reason}"

The Reviewer's rejection loops back to the Builder (cycle), capped at 5 iterations.

### Template 2: Failed Transaction Recovery Pipeline (NOVA)

A miniaturized version of Yuno's NOVA product. Linear pipeline:

- **Transaction Monitor** — Identifies failed transactions (simulated or from mock API)
- **Recovery Orchestrator** — Decides retry/WhatsApp contact/escalate per transaction; sends WhatsApp messages via `ACTION:WHATSAPP:` lines
- **Reconciliation Reporter** — Produces a summary report with recovery rates

## Extending

### Adding a New Workflow Template

Create a JSON file in `backend/templates/` following the schema of existing templates:

```json
{
  "id": "my-template",
  "name": "My Template",
  "description": "...",
  "agents": [{ "temp_id": "agent-1", "name": "...", "role": "...", "system_prompt": "...", "model": "claude-sonnet-4-5-20250929", "tools": [], "channels": [] }],
  "nodes": [{ "temp_id": "node-1", "agent_ref": "agent-1", "label": "...", "position_x": 100, "position_y": 200, "is_entry": true }],
  "edges": [{ "source_ref": "node-1", "target_ref": "node-2", "condition": "always", "priority": 0 }]
}
```

### Adding a New Messaging Channel

1. Implement the `WhatsAppClient` interface in `internal/channels/`:
   ```go
   type MyChannelClient interface {
       Send(ctx context.Context, to, message string) error
   }
   ```
2. Add a webhook handler in `internal/api/` for inbound messages
3. Register the handler in `internal/api/router.go`

## Testing

```bash
# Backend (requires PostgreSQL running)
cd backend
go test ./internal/... -v -p 1

# Frontend
cd frontend
npm test
```

## Environment Variables

See `.env.example` for the full list. Key variables:

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | Yes | Anthropic API key for LLM calls |
| `MAESTRO_RUNTIME` | No | `goose` (default) or `anthropic_direct` |
| `TWILIO_ACCOUNT_SID` | No | Twilio account SID for WhatsApp |
| `TWILIO_AUTH_TOKEN` | No | Twilio auth token |
| `TWILIO_WHATSAPP_FROM` | No | Twilio sandbox WhatsApp number |
| `MAX_ITERATIONS` | No | Max agent steps per execution (default: 5) |
| `AGENT_STEP_TIMEOUT_SECS` | No | Per-step timeout in seconds (default: 60) |
