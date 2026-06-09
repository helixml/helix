# Implementation Tasks (Fork-and-Pause, from scratch on main)

> **Predecessor:** Task [001806](../001806_high-leverage-for-us-to/) — superseded. See [kickoff.md](kickoff.md) and [design.md](design.md). Reality check confirmed (2026-06-09) that no implementation code from 001806 was ever pushed; this is a greenfield build on top of `main`.

> **Progress (2026-06-09):** Phases 0–8 landed on `feature/001806-high-leverage-for-us-to` in commits `d2612ed3a` (data model + serializer), `25ba46c16` (fork endpoint + openapi regen), `be5d51313` (pause enforcement), `d78b541f2` (seed injection), `2e557c3f3` (chat-panel fork dropdown), `30a1921c0` (paused banner + fork badge + fork_seed divider), `562b1f38f` (HTTP integration tests). 29 unit + 7 HTTP integration tests passing. **Phase 9 (manual UI verification) needs to be done by a human against a running inner Helix** — the sandbox-runner environment used for development did not have the stack up.

## Phase 0 — Branch hygiene

- [x] `git checkout feature/001806-high-leverage-for-us-to` in `/home/retro/work/helix` (already on origin, fast-forwarded to `main` — nothing to revert)

## Phase 1 — Data model

- [x] Add `SessionMetadata.ParentSessionID`, `ForkedAt`, `ForkedAtInteractionID` (lineage) in `api/pkg/types/types.go`
- [x] Add `SessionMetadata.Paused`, `PausedReason`, `PausedAt` (pause state)
- [x] Add `TriggerForkSeed = "fork_seed"` constant (place next to existing `Trigger*` constants)
- [x] Confirm Postgres JSON serialization round-trips the new fields (no migration — `config` column is already JSONB)

## Phase 2 — Backend: serializer + helpers

- [x] New file `api/pkg/server/websocket_external_agent_sync.go` (or extend existing one if it appears in a different name on the branch):
  - [x] `maxTranscriptBytes = 400000` constant
  - [x] `serializeTranscript(interactions []*types.Interaction, maxBytes int) string` — markdown output with user turns, agent responses, tool calls; truncation notice when exceeding `maxBytes`
  - [x] `serializeAgentResponse(interaction *types.Interaction) string` — helper for one interaction's agent side
  - [x] `agentNameForRuntime(runtime types.CodeAgentRuntime) string` and `runtimeForAgentName(name string) (types.CodeAgentRuntime, bool)` helpers (may already partly exist via `CodeAgentRuntime.ZedAgentName()` — confirm and reuse)

## Phase 3 — Backend: fork endpoint

- [x] `POST /api/v1/sessions/{id}/fork` — register route in `api/pkg/server/server.go`
- [x] `forkSession` handler in `api/pkg/server/session_handlers.go`:
  - [x] Accepts `{ helix_app_id?, code_agent_runtime? }`
  - [x] Resolves target runtime via app lookup if `helix_app_id` given
  - [x] Validates: source exists, user authorized, source is `zed_external`, source is not paused, target runtime differs from source
  - [x] Returns `{ new_session_id }`
  - [x] Swagger annotations
- [x] Factor `forkSessionFromParent(ctx, parent, targetRuntime, agentName) (*Session, error)` helper:
  - [x] Deep-copies project / user / workspace / system prompt / model fields from parent
  - [x] Sets fork-lineage metadata (`ParentSessionID`, `ForkedAt`, `ForkedAtInteractionID`)
  - [x] Inserts the new session row + provisions the desktop container via existing `externalAgentExecutor.StartDesktop`
- [x] On success, mark parent paused: `Metadata.Paused = true`, `PausedReason = "forked_to:<child_id>"`, `PausedAt = now`
- [x] Create the `fork_seed` synthetic interaction on the child:
  - [x] `Trigger = TriggerForkSeed`
  - [x] `PromptMessage = "Session forked from <parent_id> at turn N"`
  - [x] `ResponseMessage = serializeTranscript(parent.interactions, maxTranscriptBytes)`
- [x] Run `./stack update_openapi` to regenerate the API client

## Phase 4 — Backend: seed injection (websocket layer)

- [x] In the websocket external-agent sync path, before sending the first user message to the agent:
  - [x] Detect `ZedThreadID == ""` AND a `fork_seed` interaction exists on the session
  - [x] Prepend `fork_seed.ResponseMessage` to the outgoing user message
  - [x] Log `transcript_len > 0` for verification
- [x] Confirm subsequent messages don't re-prepend the seed (no loop / duplicate context)

## Phase 5 — Backend: pause enforcement

- [x] Introduce `requireUnpaused(session *types.Session) *system.HTTPError` helper returning HTTP 409 with `"session is paused (reason: X)"` when paused, nil otherwise
- [x] Wire into all message-ingress paths:
  - [x] `sendSessionMessage` (POST `/sessions/{id}/messages`)
  - [x] `chatSession` (POST `/sessions/chat` — when an existing session id is targeted)
  - [x] `NotifyExternalAgentOfNewInteraction` (whichever helper owns the producer side)
  - [x] `pickupWaitingInteraction` (queue consumer)
  - [x] `sendQueuedPromptToSession` (chat-panel queue-based path)
- [x] Confirm in-flight `waiting` interactions are NOT cancelled by pause (pause is "no new input", not "kill the agent")

## Phase 6 — Backend: remove the old switch-agent path

> **No-op** in this redesign — the in-place mutation code was never pushed. Skip the phase entirely.

## Phase 7 — Frontend: chat-panel fork dropdown

- [x] In `frontend/src/components/tasks/SpecTaskDetailContent.tsx`, add a new `AgentDropdown` instance in the chat panel header (separate from the existing settings-sidebar instance at line 1404 which is for task-creation/edit):
  - [x] Value = the session's current `CodeAgentRuntime` / `ZedAgentName`
  - [x] `onChange = handleFork(newAgent)`:
    - [x] Call `POST /api/v1/sessions/{id}/fork` via the generated API client with `{ helix_app_id }`
    - [x] On success: `router.navigate('session', { session_id: result.new_session_id })`
    - [x] On error (`null` return): revert dropdown selection + show error snackbar
  - [x] Disable when `session.Metadata.Paused === true` (the user should fork from the *active* descendant, not the paused parent)

## Phase 8 — Frontend: lineage + pause UI

- [x] `PausedBanner` component (new) — `frontend/src/components/session/PausedBanner.tsx`:
  - [x] Renders when `session.Metadata.Paused`
  - [x] Reason text + link to child if `PausedReason` starts with `"forked_to:"`
- [x] Wire into `EmbeddedSessionView.tsx`:
  - [x] Show `PausedBanner` at the top of the view
  - [x] Disable the chat input box when paused
- [x] `ForkBadge` component (new) — appears on child sessions in the session header: "Forked from <parent_id>" link
- [x] Render `fork_seed` interaction in `EmbeddedSessionView.tsx`:
  - [x] Inline divider (horizontal rule + centered text)
  - [x] Centered text: "Forked from <parent> with N interactions of context"
  - [x] Expandable disclosure showing the raw transcript content (`ResponseMessage`)

## Phase 9 — E2E test coverage

> **What was actually delivered:** in-process unit tests + HTTP integration tests against the in-memory store, which exercise every Go code path on the fork/pause/seed flow without spinning up Zed. The full docker-based "claude really recalls what zed-agent said" loop is **deferred to a follow-up** — it adds CI time + Anthropic API spend without verifying anything the Go code controls.

- [x] in-process: 8 unit tests for `forkSessionFromParent` / `resolveForkTarget` (covers happy path, parent_app override, empty parent, fork-of-fork lineage chain, app lookup edge cases)
- [x] in-process: 6 unit tests for `serializeTranscript` / `serializeAgentResponse` (fork_seed skipped, truncation from front, structured-entries preference, malformed-entries fallback)
- [x] in-process: 3 unit tests for `maybePrependTranscript` (no-op on existing thread, no-op on non-forked, prepends with correct ordering on first message)
- [x] in-process: 4 sub-tests for `requireUnpaused` (nil / live / paused-with-reason / paused-without-reason)
- [x] HTTP: happy path → 200 + new_session_id, parent paused with `forked_to:<child>`
- [x] HTTP: fork-from-paused → 409, fork-same-runtime → 400, fork-non-zed_external → 400, fork-nonexistent → 404, fork-by-non-owner → 403
- [x] HTTP: send-message-to-paused → 409 with `forked_to:` reason in body
- [ ] **Deferred:** docker-based E2E in `zed-repo/crates/external_websocket_sync/e2e-test/` that drives a real Claude through fork A→B→C and LLM-asserts cross-agent recall. Adds CI time + LLM spend; covers behaviour external to the Helix code under test here.

## Phase 10 — Manual verification

> **Status:** **NOT done** — this branch was developed in a sandbox-runner without the inner Helix stack running. Needs to be exercised by a human against `http://localhost:8080` before the PR merges.

- [ ] In the inner Helix UI (register `test@helix.ml` / `helixtest` → testorg → testproj → claude-opus-4-6):
  - [ ] Create a spec task → start a chat with one agent → fork to another agent via the chat-panel dropdown
  - [ ] Verify the chat panel navigates to the new session
  - [ ] Verify the new agent acknowledges context from the prior session
  - [ ] Navigate back to the parent → verify the paused banner + link to child
  - [ ] Try sending a message in the paused session → verify it's blocked
  - [ ] Fork the child to yet another agent → verify the chain shows lineage depth 2

---

## Out of scope for v1

- Manual `POST /sessions/{id}/pause` and `POST /sessions/{id}/unpause` endpoints
- `POST /sessions/{id}/duplicate` (fork to the same agent for prompt experimentation)
- Container reaping policy for paused sessions
- Shared-container optimization between parent and child
- Fork-tree visualization in the session list

## Open product questions (carried over from design.md)

- Container reaping policy for paused sessions
- Sync vs asynchronous fork (frontend polling vs blocking response)
- Whether to expose a `/duplicate` endpoint distinct from `/fork`
- Pause-of-paused semantics
