# Decision Log — Maestro

> Any decision made during the build that isn't explicitly prescribed in STEPS.md is logged here.
> See CLAUDE.md §Decision Log for the required format.

---

## Why Goose over OpenClaw/OpenCode

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Use Goose (by Block) as the agent runtime.
**Alternatives considered:** OpenClaw, OpenCode, direct API calls only.
**Rationale:** Goose has the most mature extension API, is provider-agnostic, and Block (Square/Cash App) has strong fintech credibility — directly relevant to a Yuno audience.
**Consequences:** Adds a subprocess dependency; mitigated by the AnthropicDirectRunner fallback.

---

## Why Go backend

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Go as the backend language.
**Alternatives considered:** Node.js/TypeScript, Python, Rust.
**Rationale:** Matches Yuno's primary backend language. Goroutines are ideal for concurrent agent execution and the SSE broadcaster. Explicit control over concurrency and error handling.
**Consequences:** Yuno engineers will immediately recognize the idioms and patterns.

---

## Why event-driven executor over topological sort

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Event-driven workflow executor with first-match edge semantics.
**Alternatives considered:** Topological sort, BFS/DFS pre-ordering.
**Rationale:** Topological sort is undefined for graphs with cycles. Template 1 has a deliberate cycle (Reviewer → Builder on rejection). Event-driven execution mirrors Yuno's payment routing model: events arrive, conditions are evaluated, next hop dispatched.
**Consequences:** No static pre-ordering; graph traversed dynamically. Requires iteration guard to prevent infinite loops.

---

## Why SSE over WebSocket

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Server-Sent Events for real-time monitoring.
**Alternatives considered:** WebSocket, long-polling.
**Rationale:** Monitoring is unidirectional (server→client). SSE is simpler, no handshake overhead, built-in reconnection. Sufficient for log tailing and status updates.
**Consequences:** Cannot send client→server messages over the same connection (not needed for monitoring).

---

## Why Twilio Sandbox over Meta API direct

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Twilio WhatsApp Sandbox for the external channel demo.
**Alternatives considered:** Meta Business API direct, unofficial WhatsApp libraries.
**Rationale:** No approval delays, legitimate API (not a scraper), 5-minute setup. Twilio is a respected platform in fintech.
**Consequences:** Limited to sandbox participants; sufficient for demo purposes.

---

## Why Runner interface with two implementations

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** `Runner` interface with `GooseRunner` and `AnthropicDirectRunner`.
**Alternatives considered:** Coupling directly to Goose CLI only.
**Rationale:** Resilience (fallback if Goose is flaky), testability (mock Runner in tests), Docker compatibility (Goose not installed in container).
**Consequences:** Must maintain two implementations, but they share prompt-building helpers.

---

## Why Chi over Gin/Echo

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Chi as the HTTP router.
**Alternatives considered:** Gin, Echo, stdlib only.
**Rationale:** Idiomatic Go, close to stdlib, no framework magic. Easier to reason about in financial contexts where transparency matters.
**Consequences:** Slightly more boilerplate than Gin, but code is more explicit.

---

## Why raw SQL over GORM

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Raw SQL with pgx/v5, no ORM.
**Alternatives considered:** GORM, sqlx, sqlc.
**Rationale:** Explicit control, easier debugging, no ORM magic hiding query behavior. Consistent with how you'd want to audit financial data access at Yuno.
**Consequences:** More verbose CRUD code, but every query is visible and auditable.

---

## Why first-match edge semantics over all-match

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Edges evaluated in priority ASC order; first match wins, no fan-out.
**Alternatives considered:** All-match (fan-out to every matching edge).
**Rationale:** Deterministic routing. Avoids fan-out on ambiguous output. Easier to reason about in cyclic workflows. Mirrors how payment routing typically works — one route selected, not broadcast.
**Consequences:** Only one path taken per step; complex fan-out patterns not supported (not needed for either template).

---

## Why skip Twilio HMAC validation in demo

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Skip HMAC signature validation for inbound Twilio webhooks.
**Alternatives considered:** Full HMAC validation.
**Rationale:** HMAC validation with ngrok URLs is a time sink to debug. Not a correctness concern for a demo. Clearly labeled with TODO comment for production.
**Consequences:** Webhook endpoint is unauthenticated in demo; acceptable tradeoff.

---

## Why two-panel monitoring layout over three-panel

**Date:** 2026-04-04
**Phase:** Planning
**Decision:** Two-panel monitoring dashboard (workflow view + timeline).
**Alternatives considered:** Three-panel layout (workflow + messages + logs separately).
**Rationale:** Cuts significant frontend time. All information still visible in a single timeline. Easier to follow during a live demo.
**Consequences:** Less granular filtering, but simpler UX.

---

## Run migrations on startup

**Date:** 2026-04-04
**Phase:** Phase 1 — Backend Foundation
**Decision:** Run golang-migrate on server startup in main.go, not as a separate CLI step.
**Alternatives considered:** Separate `migrate up` CLI step before starting the server.
**Rationale:** Simpler for Docker Compose — the backend container handles its own schema setup. Uses `migrate.ErrNoChange` to no-op when already applied. One fewer manual step for `docker-compose up`.
**Consequences:** Schema changes apply automatically on deploy. In production you'd want a separate migration step, but for a demo this is cleaner.

---

## Use context.Background() for engine execution from HTTP handlers

**Date:** 2026-04-04
**Phase:** Phase 2 — Workflow Engine
**Decision:** Pass `context.Background()` to `engine.Execute()` from the HTTP handler, not `r.Context()`.
**Alternatives considered:** Using `r.Context()` (the HTTP request context).
**Rationale:** The engine runs asynchronously in goroutines after the HTTP response is sent. Using `r.Context()` causes the context to be canceled when the HTTP handler returns, killing the engine goroutine. `context.Background()` allows the engine to run independently of the request lifecycle.
**Consequences:** Engine execution is not tied to HTTP request cancellation. A separate cancel mechanism (per-execution cancel map) is needed for the stretch-goal cancel endpoint.

---

## WhatsApp webhook polls for execution completion before replying

**Date:** 2026-04-04
**Phase:** Phase 3 — External Channel + Scheduler
**Decision:** The inbound WhatsApp webhook handler returns 200 immediately to Twilio, then spawns a goroutine that polls `GetExecution` every second (up to 120s) until the ad-hoc execution completes. Once done, it reads the last message and sends it as the WhatsApp reply.
**Alternatives considered:** (1) Blocking the HTTP handler until execution completes (Twilio would timeout). (2) Using a channel/callback from the engine (requires engine refactoring). (3) Subscribing to SSE events internally.
**Rationale:** Twilio requires a fast 200 response. Polling is simple and reliable. The engine already sets execution status to "completed"/"failed"/"timed_out", so polling is a natural fit. The 120s cap prevents leaked goroutines.
**Consequences:** There's a 1-second latency granularity on the reply. Acceptable for a demo.

---

## FindByChannel uses PostgreSQL JSONB containment operator

**Date:** 2026-04-04
**Phase:** Phase 3 — External Channel + Scheduler
**Decision:** `agent.Store.FindByChannel` queries `WHERE channels @> '["whatsapp"]'::jsonb` using PostgreSQL's JSONB containment operator.
**Alternatives considered:** Loading all agents and filtering in Go.
**Rationale:** Pushes the filter to the database. JSONB `@>` is indexable and idiomatic PostgreSQL. Returns the first matching agent (`LIMIT 1`).
**Consequences:** Only one agent per channel is supported for inbound routing. Sufficient for demo.

---

## @xyflow/react instead of reactflow

**Date:** 2026-04-04
**Phase:** Phase 4 — Frontend
**Decision:** Use `@xyflow/react` (v12+) instead of the `reactflow` package listed in STEPS.md.
**Alternatives considered:** Installing legacy `reactflow` package.
**Rationale:** `reactflow` was renamed to `@xyflow/react` in v12. The old package name is deprecated. The API is nearly identical but imports come from `@xyflow/react`.
**Consequences:** Import paths differ from STEPS.md sketches but functionality is the same.

---

## Client components with useParams instead of async params

**Date:** 2026-04-04
**Phase:** Phase 4 — Frontend
**Decision:** Use `'use client'` + `useParams()` for dynamic route pages instead of server component async `params`.
**Alternatives considered:** Server components with `await params` (Next.js 16 pattern).
**Rationale:** All dynamic pages (agent detail, workflow editor, monitor) need client-side interactivity (React Flow, SSE, state). Using `useParams()` in client components is simpler than splitting into server/client layers.
**Consequences:** Pages are fully client-rendered. Initial load fetches data client-side. Acceptable for a dashboard-style app.

---

## Goose CLI output format differs from STEPS.md assumptions

**Date:** 2026-04-04
**Phase:** Phase 0 — Goose Verification
**Decision:** Update GooseOutput struct to match actual Goose v1.29.1 JSON format. Response text is at `messages[last].content[0].text`, not a flat `response` field. Token info is `metadata.total_tokens` (single number), not split into `input_tokens`/`output_tokens`. Banner text precedes JSON and must be skipped.
**Alternatives considered:** Switching entirely to anthropic_direct.
**Rationale:** Goose works (exit code 0, correct response "PONG"), but the output format requires finding the last assistant message and extracting text. Token split will be estimated (~50/50 of total_tokens, or char-count fallback). Both runtimes remain supported as planned.
**Consequences:** GooseRunner.parseOutput needs to handle the messages array format and skip banner text. Token cost tracking from Goose will be approximate.

---

## Inline edge condition editor over modal dialog

**Date:** 2026-04-07
**Phase:** Phase 4 — Frontend Improvements
**Decision:** Edge conditions are edited via an inline panel anchored to the bottom-center of the React Flow canvas, triggered by clicking an edge. Condition is a dropdown (always/approved/rejected/custom) with an optional free-text input for custom substring matches. Priority is a number input.
**Alternatives considered:** Full modal dialog, right-click context menu, sidebar panel.
**Rationale:** An inline panel keeps the user's attention on the workflow graph and avoids the heavy context switch of a modal. The dropdown presets cover the most common conditions while custom allows arbitrary substring matching consistent with the engine's `evaluateCondition` logic.
**Consequences:** Edge updates require a new `PUT /api/workflows/{id}/edges/{edgeId}` endpoint (added). The panel closes on save or manual dismiss.

---

## Execution GET response extended with aggregated cost summary

**Date:** 2026-04-07
**Phase:** Phase 4 — Frontend Improvements
**Decision:** The `GET /api/executions/{id}` response now includes a `cost_summary` field with `total_tokens_in`, `total_tokens_out`, `total_cost_usd`, and per-agent breakdown. Aggregation is done via a new `GetCostSummary` store method that queries `execution_costs`.
**Alternatives considered:** (1) Separate `GET /api/executions/{id}/costs` endpoint. (2) Client-side aggregation from raw cost rows.
**Rationale:** Embedding the summary in the existing execution response avoids an extra HTTP round-trip. The monitoring dashboard already polls `getExecution()` — no new fetch needed. Non-fatal: if cost query fails, the execution is still returned without costs.
**Consequences:** The Execution response type is slightly larger. Frontend `Execution` interface extended with optional `cost_summary`.

---

## Renamed Skills tab to Schedules with full CRUD

**Date:** 2026-04-07
**Phase:** Phase 4 — Frontend Improvements
**Decision:** Replaced the "Skills — coming soon" placeholder tab in AgentModal with a fully functional Schedules tab. Users can add cron schedules (expression + task prompt), toggle enabled/disabled, and delete schedules. Added `DELETE /api/agents/{id}/schedules/{scheduleId}` and `PUT /api/agents/{id}/schedules/{scheduleId}` (toggle enabled) backend endpoints.
**Alternatives considered:** Adding schedules as a separate page section on the agent detail view.
**Rationale:** Keeping schedules inside the AgentModal tab structure is consistent with how memory and guardrails are configured. The backend already had schedule CRUD — only the UI and delete/toggle endpoints were missing.
**Consequences:** The Skills concept is no longer exposed in the UI. Skills backend CRUD remains available via API but has no frontend surface. This is acceptable since no template uses skills.

---

## Split handlers.go into domain-specific files

**Date:** 2026-04-07
**Phase:** Phase 5 — Code Quality
**Decision:** Split the 899-line `internal/api/handlers.go` into 6 files: `helpers.go` (shared utilities), `agent_handler.go`, `workflow_handler.go`, `execution_handler.go`, `template_handler.go`, `webhook_handler.go`. No behavior changes.
**Alternatives considered:** (1) Keep as-is (functional but hard to navigate). (2) Split into sub-packages per domain.
**Rationale:** A single 900-line file with 30+ handler functions is hard to navigate and review. Domain-specific files match the route groups in `router.go`. Sub-packages would be over-engineering — all handlers share the same helper functions and belong in the same Chi router registration.
**Consequences:** Each file has clear, focused responsibility. Import lists are smaller per file. No change to router.go or any external behavior.

---

## Integration tests use httptest with real DB and mock Runner

**Date:** 2026-04-07
**Phase:** Phase 5 — Code Quality
**Decision:** Integration tests for template loading and WhatsApp webhook use `httptest.NewRecorder` + the real router, real PostgreSQL (`maestro_test`), and a mock `Runner`. The tests verify end-to-end behavior: HTTP request → handler → store → DB → response.
**Alternatives considered:** (1) Unit tests mocking the store layer. (2) External test process hitting a running server.
**Rationale:** The handler tests need to verify that the router correctly dispatches requests AND that the handler logic correctly creates the expected DB records. Mocking the store would not catch SQL or routing bugs. An external test process adds deployment complexity. `httptest` + real DB is the sweet spot for handler integration tests, consistent with the existing `engine_test.go` pattern.
**Consequences:** Tests require a running PostgreSQL with `maestro_test` database. The cleanup block in `testPool` truncates all tables after each test to prevent cross-test contamination.

---

## Automatic ngrok tunnel and Twilio webhook configuration on startup

**Date:** 2026-04-07
**Phase:** Phase 3 — External Channel
**Decision:** When `NGROK_AUTH_TOKEN` is set, the backend automatically starts an ngrok tunnel using the official Go SDK (`golang.ngrok.com/ngrok`). The tunnel URL is logged at startup so the user can paste it into the Twilio sandbox console (one-time setup with a reserved domain). The Twilio sandbox webhook cannot be updated programmatically — there is no REST API for it, and the Messaging v2 Senders API returns an empty list for sandbox numbers.
**Alternatives considered:** (1) Keep manual ngrok + Twilio console setup. (2) Use ngrok CLI subprocess instead of the Go SDK. (3) Auto-update the Twilio webhook via REST API (investigated; not possible for sandbox).
**Rationale:** The manual setup required running `ngrok http 8080` in a separate terminal, copying the URL, and pasting it. The Go SDK eliminates the separate process — `ngrok.Listen` returns a `net.Listener` that `http.Serve` accepts directly, and the tunnel lifecycle is tied to the server process. With a reserved domain (`NGROK_DOMAIN`), the URL is stable across restarts, so the Twilio console only needs to be configured once.
**Consequences:** Added `golang.ngrok.com/ngrok` dependency. `NGROK_URL` env var removed (URL is determined at runtime). When ngrok is active, the server listens on both the ngrok tunnel (for external Twilio webhooks) and localhost (for the frontend and internal requests). When `NGROK_AUTH_TOKEN` is not set, behavior is unchanged. If the tunnel fails to connect (e.g. stale session on free plan), the server falls back to local-only mode gracefully.

---

## Reserved ngrok domain via NGROK_DOMAIN env var

**Date:** 2026-04-07
**Phase:** Phase 3 — External Channel
**Decision:** Added `NGROK_DOMAIN` env var. When set, the ngrok tunnel uses a fixed reserved domain (e.g. `my-app.ngrok-free.app`) via `ngrokconfig.WithDomain()` instead of a random URL. This means the Twilio webhook URL is stable across server restarts.
**Alternatives considered:** (1) Always use random domains and re-configure Twilio on every startup. (2) Store the last-used URL and skip Twilio update if unchanged.
**Rationale:** With a Hobby+ plan, ngrok provides a free reserved domain. A fixed domain eliminates the need to update the Twilio webhook on every restart — configure it once and forget. The implementation is a single optional config option passed to `ngrokconfig.HTTPEndpoint()`.
**Consequences:** Users on the free ngrok plan still get random domains (NGROK_DOMAIN left empty). Hobby+ users set it once. The Twilio auto-update still runs on every startup as a safety net, but it's a no-op if the URL hasn't changed.

---

## Enable ngrok endpoint pooling to prevent ERR_NGROK_334

**Date:** 2026-04-07
**Phase:** Phase 3 — External Channel
**Decision:** Added `ngrokconfig.WithPoolingEnabled(true)` to the ngrok listener options. This allows multiple instances (or a new instance after a crash) to share the same reserved domain without `ERR_NGROK_334` ("endpoint is already online") errors.
**Alternatives considered:** (1) Leave pooling off and rely on the stale session expiring before restarting. (2) Use the ngrok API to force-disconnect stale sessions (requires a separate API key).
**Rationale:** Without pooling, if the server is killed without a graceful shutdown (e.g. `kill -9`, crash, power loss), the stale ngrok session blocks new connections to the same domain until it expires server-side (can take several minutes). With pooling enabled, ngrok treats the new connection as a pool member and routes traffic to it immediately. This is the intended solution per ngrok's own error message.
**Consequences:** Traffic to the reserved domain is load-balanced if multiple instances connect simultaneously. For a single-instance demo this is a no-op — the behavior is identical to non-pooled mode.

---

## Stale ngrok endpoint cleanup on startup via API

**Date:** 2026-04-07
**Phase:** Phase 3 — External Channel
**Decision:** On startup, if `NGROK_API_KEY` is set, the server lists endpoints via `GET https://api.ngrok.com/endpoints`, finds any matching the reserved domain, and sends `POST /tunnel_sessions/{id}/stop` to tear them down. The server also registers `ngrok.WithStopHandler` so that future sessions support remote stop commands. This is a best-effort safety net alongside pooling.
**Alternatives considered:** (1) `DELETE /endpoints/{id}` (returns 404 for ephemeral endpoints — not supported). (2) Rotating the authtoken programmatically (API only supports PATCH description, not token rotation). (3) Relying solely on pooling.
**Rationale:** Pooling handles the common case (new session coexists with stale one), but the stop command can actively tear down a stale session if it was started with `WithStopHandler`. Together they cover both the "stale session supports remote stop" and "stale session doesn't support remote stop" cases. The `context.Background()` fix for `ngrok.Listen` ensures the tunnel stays alive after connection (previously the timeout context's cancel was closing the listener immediately).
**Consequences:** Requires `NGROK_API_KEY` env var (from dashboard.ngrok.com → API). Without it, cleanup is skipped and pooling alone handles restarts. Added 2-second sleep after successful cleanup to let ngrok's servers process the teardown.
