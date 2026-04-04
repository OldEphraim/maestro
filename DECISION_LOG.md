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

## Goose CLI output format differs from STEPS.md assumptions

**Date:** 2026-04-04
**Phase:** Phase 0 — Goose Verification
**Decision:** Update GooseOutput struct to match actual Goose v1.29.1 JSON format. Response text is at `messages[last].content[0].text`, not a flat `response` field. Token info is `metadata.total_tokens` (single number), not split into `input_tokens`/`output_tokens`. Banner text precedes JSON and must be skipped.
**Alternatives considered:** Switching entirely to anthropic_direct.
**Rationale:** Goose works (exit code 0, correct response "PONG"), but the output format requires finding the last assistant message and extracting text. Token split will be estimated (~50/50 of total_tokens, or char-count fallback). Both runtimes remain supported as planned.
**Consequences:** GooseRunner.parseOutput needs to handle the messages array format and skip banner text. Token cost tracking from Goose will be approximate.
