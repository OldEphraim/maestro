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

```go
// internal/runtime/goose.go

type GooseRunner struct {
    binaryPath string
    apiKey     string
    lastPrompt string // stored for token estimation fallback
}

// GooseOutput reflects the actual JSON schema from --output-format json.
// Field names verified in Phase 0 — update if they differ.
type GooseOutput struct {
    Response string `json:"response"` // UPDATE THIS if the actual field name differs
    Usage    *struct {
        InputTokens  int `json:"input_tokens"`
        OutputTokens int `json:"output_tokens"`
    } `json:"usage,omitempty"` // may be absent depending on Goose version
}

func (r *GooseRunner) Run(ctx context.Context, ag AgentWithMemory, task string) (string, Usage, error) {
    // buildFullPrompt uses ag.Memory (map[string]string) to inject memory into the system prompt.
    // This is why the interface accepts AgentWithMemory, not Agent.
    fullPrompt := buildFullPrompt(ag, task) // system prompt + memory key-values + skills + task
    r.lastPrompt = fullPrompt

    // Per-step timeout: 60 seconds. Analogous to Yuno's PSP transaction timeouts —
    // don't wait forever for a hung provider; fail fast and surface the error.
    ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx,
        r.binaryPath,
        "run",
        "--no-session",
        "--provider", "anthropic",
        "--model", gooseModelName(ag.Model), // strips date suffix: "claude-sonnet-4-5-20250929" -> "claude-sonnet-4-5"
        "--output-format", "json",
        "-t", fullPrompt,
    )
    cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+r.apiKey)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return "", Usage{}, ErrStepTimeout
        }
        return "", Usage{}, fmt.Errorf("goose run: %w\nstderr: %s", err, stderr.String())
    }

    return r.parseOutput(stdout.Bytes(), ag.Model)
}

func (r *GooseRunner) parseOutput(raw []byte, model string) (string, Usage, error) {
    var out GooseOutput

    if err := json.Unmarshal(raw, &out); err != nil {
        // Goose may emit non-JSON preamble — try to find the first '{' and parse from there
        if start := bytes.IndexByte(raw, '{'); start >= 0 {
            if err2 := json.Unmarshal(raw[start:], &out); err2 != nil {
                // Give up on JSON parsing; treat entire stdout as raw response text
                return string(raw), r.estimateUsage(string(raw), model), nil
            }
        } else {
            return string(raw), r.estimateUsage(string(raw), model), nil
        }
    }

    usage := Usage{Source: "estimated"}
    if out.Usage != nil {
        usage.TokensIn = out.Usage.InputTokens
        usage.TokensOut = out.Usage.OutputTokens
        usage.Source = "goose_json"
    } else {
        // Approximate: ~4 chars per token, rough Claude pricing
        usage = r.estimateUsage(out.Response, model)
    }
    usage.EstimatedCostUSD = estimateCost(usage.TokensIn, usage.TokensOut, model)

    return out.Response, usage, nil
}

func (r *GooseRunner) estimateUsage(response, model string) Usage {
    tokensIn  := len(r.lastPrompt) / 4
    tokensOut := len(response) / 4
    return Usage{
        TokensIn:         tokensIn,
        TokensOut:        tokensOut,
        Source:           "estimated",
        EstimatedCostUSD: estimateCost(tokensIn, tokensOut, model),
        // Note: the monitoring dashboard will show this as an approximate cost
        // (marked 'estimated') rather than $0.00 when Goose doesn't emit token counts.
    }
}
```

### Fallback: direct Anthropic API

If Goose CLI invocation proves unreliable (wrong JSON schema, flaky exit codes, auth issues), **switch to calling the Anthropic API directly from Go**. This eliminates the subprocess dependency entirely and is arguably simpler.

```go
// internal/runtime/anthropic_direct.go

type AnthropicDirectRunner struct {
    apiKey string
    client *http.Client
}

func (r *AnthropicDirectRunner) Run(ctx context.Context, ag AgentWithMemory, task string) (string, Usage, error) {
    payload := map[string]any{
        "model":      ag.Model, // canonical Anthropic string stored in agents.model
        "max_tokens": 4096,
        "system":     buildSystemPrompt(ag), // separate helper: system prompt + memory + skills, no task appended
        // Note: GooseRunner bakes task into one string via buildFullPrompt(ag, task) because
        // it passes everything through a single -t flag. AnthropicDirectRunner uses the
        // Anthropic API's native system/user split, so the task travels as the user message
        // below — not appended to the system prompt. Two distinct functions:
        //   buildSystemPrompt(ag AgentWithMemory) string  — system + memory + skills
        //   buildFullPrompt(ag AgentWithMemory, task string) string  — above + "\n\nTask: " + task
        // Both live in internal/runtime/prompt.go. AnthropicDirectRunner calls buildSystemPrompt;
        // GooseRunner calls buildFullPrompt. Neither appends a dangling "\n\nTask: ".
        "messages":   []map[string]any{{"role": "user", "content": task}},
    }
    body, _ := json.Marshal(payload)

    req, _ := http.NewRequestWithContext(ctx, "POST",
        "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
    req.Header.Set("x-api-key", r.apiKey)
    req.Header.Set("anthropic-version", "2023-06-01")
    req.Header.Set("Content-Type", "application/json")

    resp, err := r.client.Do(req)
    if err != nil {
        return "", Usage{}, fmt.Errorf("anthropic API: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", Usage{}, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, body)
    }

    var result struct {
        Content []struct { Text string `json:"text"` } `json:"content"`
        Usage   struct {
            InputTokens  int `json:"input_tokens"`
            OutputTokens int `json:"output_tokens"`
        } `json:"usage"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", Usage{}, fmt.Errorf("anthropic decode: %w", err)
    }

    text := ""
    if len(result.Content) > 0 {
        text = result.Content[0].Text
    }
    usage := Usage{
        TokensIn:         result.Usage.InputTokens,
        TokensOut:        result.Usage.OutputTokens,
        Source:           "anthropic_api",
        EstimatedCostUSD: estimateCost(result.Usage.InputTokens, result.Usage.OutputTokens, ag.Model),
    }
    return text, usage, nil
}
```

**The `Runner` interface** (both implementations satisfy this):
```go
type Runner interface {
    // AgentWithMemory carries both the agent config and the pre-loaded memory map.
    // buildFullPrompt uses ag.Memory to inject key-value pairs into the system prompt,
    // so the base Agent type is insufficient here.
    Run(ctx context.Context, ag AgentWithMemory, task string) (string, Usage, error)
}
```

**Selecting the implementation** via env var:
```go
// in main.go
var runner runtime.Runner
switch os.Getenv("MAESTRO_RUNTIME") {
case "anthropic_direct":
    runner = runtime.NewAnthropicDirectRunner(os.Getenv("ANTHROPIC_API_KEY"))
default:
    runner = runtime.NewGooseRunner(
        os.Getenv("GOOSE_BINARY_PATH"),
        os.Getenv("ANTHROPIC_API_KEY"),
    )
}
```
Both runtimes read `agent.Model` from the database. The column always stores the canonical Anthropic string (e.g. `claude-sonnet-4-5-20250929`). `GooseRunner` strips the date suffix via `gooseModelName()` before passing `--model` to the CLI. `AnthropicDirectRunner` uses the stored string as-is. No per-runtime configuration or data migration is needed when switching runtimes.

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

```go
// internal/workflow/engine.go

const DefaultMaxIterations = 5

type Engine struct {
    agents    agent.Store
    workflows workflow.Store
    // workflow.Store must implement:
    //   CheckGuardrails(ctx context.Context, agentID uuid.UUID, g agent.Guardrails) error
    //     Queries execution_costs for tokens used this run vs g.MaxTokensPerRun.
    //     Queries run count in the last hour vs g.MaxRunsPerHour.
    //     Returns ErrCostLimitExceeded or ErrRateLimitExceeded.
    //     Lives in the workflow package (not runtime) to avoid a costStore field on Engine.
    runtime   runtime.Runner
    sse       *sse.Broadcaster
    whatsapp  channels.WhatsAppClient
    // Note: no separate costStore field. Guardrails enforcement routes through
    // e.workflows.CheckGuardrails(ctx, agentID, guardrails), which queries
    // execution_costs internally. This avoids a second store dependency on Engine.
}

func (e *Engine) Execute(ctx context.Context, workflowID uuid.UUID, trigger string) (uuid.UUID, error) {
    wf, err := e.workflows.GetFull(ctx, workflowID) // loads nodes + edges
    if err != nil {
        return uuid.Nil, err
    }

    exec := &Execution{
        ID:            uuid.New(),
        WorkflowID:    &workflowID,
        ExecutionType: "workflow",
        Status:        "running",
        TriggeredBy:   trigger,
    }
    e.workflows.CreateExecution(ctx, exec)
    e.sse.Publish(Event{Type: "ExecutionStarted", ExecutionID: exec.ID})

    entry := wf.EntryNode()
    if entry == nil {
        return uuid.Nil, errors.New("no entry node found")
    }

    go e.runNode(ctx, exec, wf, entry, "Start workflow")
    return exec.ID, nil
}

func (e *Engine) runNode(ctx context.Context, exec *Execution, wf *Workflow, node *Node, input string) {
    // Cycle guard
    count := e.workflows.IncrementIterationCount(ctx, exec.ID)
    maxIter := defaultInt(os.Getenv("MAX_ITERATIONS"), DefaultMaxIterations)
    if count > maxIter {
        e.failExecution(ctx, exec, "max iterations exceeded — possible infinite loop detected")
        return
    }

    // Load agent config + memory first — needed by both guardrails check and runtime
    ag, err := e.agents.GetWithMemory(ctx, node.AgentID)
    if err != nil {
        e.failExecution(ctx, exec, fmt.Sprintf("load agent %s: %v", node.AgentID, err))
        return
    }

    // Guardrails check — before spawning any LLM call
    if err := e.workflows.CheckGuardrails(ctx, ag.ID, ag.Guardrails); err != nil {
        e.failExecution(ctx, exec, fmt.Sprintf("guardrails: %v", err))
        return
    }

    // Per-step timeout
    stepCtx, cancel := context.WithTimeout(ctx, stepTimeout())
    defer cancel()

    e.sse.Publish(Event{Type: "AgentStarted", ExecutionID: exec.ID, AgentID: node.AgentID})

    output, usage, err := e.runtime.Run(stepCtx, ag, input)

    if errors.Is(err, runtime.ErrStepTimeout) {
        e.workflows.SetStatus(ctx, exec.ID, "timed_out")
        e.sse.Publish(Event{Type: "StepTimedOut", ExecutionID: exec.ID, AgentID: node.AgentID})
        return
    }
    if err != nil {
        e.failExecution(ctx, exec, err.Error())
        return
    }

    // Check for outbound WhatsApp action — only for agents with "whatsapp" in their channels.
    // Gating on the channel list prevents accidental triggers when agents like the Connector Scout
    // produce output that happens to mention "WHATSAPP:" while describing a PSP's webhook format.
    if ag.HasChannel("whatsapp") {
        e.handleWhatsAppAction(ctx, exec, ag, output)
    }

    // Persist
    e.workflows.CreateMessage(ctx, exec.ID, node.AgentID, nil, output, "internal")
    e.workflows.RecordCost(ctx, exec.ID, node.AgentID, usage)
    e.sse.Publish(Event{Type: "AgentCompleted", ExecutionID: exec.ID, AgentID: node.AgentID})

    // First-match edge evaluation
    edges := wf.OutgoingEdges(node.ID) // pre-sorted by priority ASC from SQL (ORDER BY priority ASC in GetFull query)
    for _, edge := range edges {
        if evaluateCondition(output, edge.Condition) {
            target := wf.Node(edge.TargetNodeID)
            e.sse.Publish(Event{Type: "MessageDispatched", From: node.AgentID, To: target.AgentID})
            go e.runNode(ctx, exec, wf, target, output)
            return // first-match: don't evaluate further edges
        }
    }

    // No matching edge = terminal node
    e.workflows.SetStatus(ctx, exec.ID, "completed")
    e.workflows.SetCompletedAt(ctx, exec.ID, time.Now())
    e.sse.Publish(Event{Type: "ExecutionCompleted", ExecutionID: exec.ID})
}

// handleWhatsAppAction scans output for "ACTION:WHATSAPP: +1234567890 | message" lines.
// The ACTION: namespace prefix distinguishes deliberate agent commands from incidental output
// that mentions "WHATSAPP" (e.g. a Connector Scout describing a PSP's webhook format).
// Only called for agents with "whatsapp" in their channels array (checked by caller).
func (e *Engine) handleWhatsAppAction(ctx context.Context, exec *Execution, ag AgentWithMemory, output string) {
    agentID := ag.ID
    const prefix = "ACTION:WHATSAPP:"
    for _, line := range strings.Split(output, "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, prefix) {
            parts := strings.SplitN(strings.TrimPrefix(line, prefix), "|", 2)
            if len(parts) == 2 {
                to := strings.TrimSpace(parts[0])
                msg := strings.TrimSpace(parts[1])
                if err := e.whatsapp.Send(ctx, to, msg); err != nil {
                    e.workflows.LogError(ctx, exec.ID, agentID, "WhatsApp send failed: "+err.Error())
                } else {
                    e.workflows.CreateMessage(ctx, exec.ID, agentID, nil, msg, "whatsapp")
                    e.sse.Publish(Event{Type: "WhatsAppSent", ExecutionID: exec.ID, AgentID: agentID, To: to})
                }
            }
        }
    }
}
```

### Edge condition evaluation — first-match semantics

Outgoing edges are sorted by `priority ASC`. The **first** edge whose condition matches is followed; subsequent edges are not evaluated. This prevents ambiguous routing when output contains multiple keywords.

```go
func evaluateCondition(output, condition string) bool {
    switch strings.ToLower(strings.TrimSpace(condition)) {
    case "always", "":
        return true
    case "approved":
        return strings.Contains(strings.ToUpper(output), "APPROVED")
    case "rejected":
        return strings.Contains(strings.ToUpper(output), "REJECTED")
    default:
        return strings.Contains(output, condition) // arbitrary substring
    }
}
```

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

The backend must not attempt to connect before PostgreSQL is accepting connections. A race condition here will fail the demo if the audience is watching `docker-compose up` from scratch.

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: maestro
      POSTGRES_PASSWORD: maestro
      POSTGRES_DB: maestro
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U maestro -d maestro"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 10s

  backend:
    build: ./backend
    depends_on:
      postgres:
        condition: service_healthy   # waits for healthcheck to pass, not just container start
    environment:
      DATABASE_URL: postgres://maestro:maestro@postgres:5432/maestro
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
      MAESTRO_RUNTIME: ${MAESTRO_RUNTIME:-anthropic_direct}   # Goose CLI not installed in container
      GOOSE_BINARY_PATH: ${GOOSE_BINARY_PATH:-/usr/local/bin/goose}
      TWILIO_ACCOUNT_SID: ${TWILIO_ACCOUNT_SID}
      TWILIO_AUTH_TOKEN: ${TWILIO_AUTH_TOKEN}
      TWILIO_WHATSAPP_FROM: ${TWILIO_WHATSAPP_FROM}
      MAX_ITERATIONS: ${MAX_ITERATIONS:-5}
      AGENT_STEP_TIMEOUT_SECS: ${AGENT_STEP_TIMEOUT_SECS:-60}
    ports:
      - "8080:8080"

  frontend:
    build: ./frontend
    depends_on:
      - backend
    environment:
      NEXT_PUBLIC_API_URL: http://localhost:8080
    ports:
      - "3000:3000"

volumes:
  postgres_data:
```

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

## Key Commands

```bash
# Start everything
docker-compose up

# Backend only (development)
cd backend && go run ./cmd/server

# Frontend only
cd frontend && npm run dev

# Run migrations
cd backend && migrate -path migrations -database $DATABASE_URL up

# PHASE 0: Verify Goose CLI before writing any other code
ANTHROPIC_API_KEY=your-key goose run \
  --no-session --provider anthropic \
  --model claude-sonnet-4-5 \
  --output-format json \
  -t "Reply with exactly the word: PONG"
# Inspect output, save as goose-test-output.json, verify field names

# Switch to direct Anthropic runtime (if Goose proves flaky)
MAESTRO_RUNTIME=anthropic_direct go run ./cmd/server

# Backend tests (requires maestro_test database on local Postgres)
createdb maestro_test
cd backend && go test ./...

# Expose local port for Twilio webhooks
ngrok http 8080
# Copy the HTTPS URL → Twilio console → Sandbox webhook URL
```

---

## Demo Script

1. `docker-compose up` from scratch — show PostgreSQL health check passing before backend connects.
2. Open browser → Templates → load **"Failed Transaction Recovery Pipeline (NOVA)"**.
3. Say verbally: *"This template is a miniaturized version of Yuno's NOVA product — AI agents recovering failed payments by contacting customers via WhatsApp. Template 1 mirrors your PSP connector onboarding workflow."*
4. Show the React Flow canvas: Transaction Monitor → Recovery Orchestrator → Reconciliation Reporter.
5. Click **Run Workflow** → monitoring dashboard opens automatically.
6. Watch agent status cards flip: idle → running → completed, left to right, driven by SSE.
7. Show inter-agent messages appearing in the timeline as each step completes.
8. WhatsApp message arrives on phone — show screen to camera.
9. Reply from phone — show the response appearing in the monitoring dashboard under external channel messages.
10. Show the Reconciliation Reporter's final summary in the log (recovered count, escalated count).
11. Briefly: load **"Payment Connector Integration Pipeline"**, trigger with "Stripe". Show Reviewer rejecting once (`REJECTED: missing idempotency key handling`), Builder revising, Reviewer approving (`APPROVED`).
    - The Connector Scout generates a plausible Stripe API spec from its training knowledge, not live web scraping (no `--with-builtin developer` by default). Frame this in the demo as: "the Scout reasons from its knowledge of Stripe's API." If you want actual live research during the demo, add `--with-builtin developer` to the Scout agent's Goose invocation in `GooseRunner.Run` and note it in the README.
12. Close: *"Both templates are themed around Yuno's actual engineering challenges. I built them after studying your NOVA product and your Core Payments integration workflow."*