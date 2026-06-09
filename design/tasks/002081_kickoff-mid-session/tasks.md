# Implementation Tasks (Fork-and-Pause, from scratch on main)

> **Predecessor:** Task [001806](../001806_high-leverage-for-us-to/) — superseded. See [kickoff.md](kickoff.md) and [design.md](design.md). Reality check confirmed (2026-06-09) that no implementation code from 001806 was ever pushed; this is a greenfield build on top of `main`.

## Phase 0 — Branch hygiene

- [ ] `git checkout feature/001806-high-leverage-for-us-to` in `/home/retro/work/helix` (already on origin, fast-forwarded to `main` — nothing to revert)

## Phase 1 — Data model

- [ ] Add `SessionMetadata.ParentSessionID`, `ForkedAt`, `ForkedAtInteractionID` (lineage) in `api/pkg/types/types.go`
- [ ] Add `SessionMetadata.Paused`, `PausedReason`, `PausedAt` (pause state)
- [ ] Add `TriggerForkSeed = "fork_seed"` constant (place next to existing `Trigger*` constants)
- [ ] Confirm Postgres JSON serialization round-trips the new fields (no migration — `config` column is already JSONB)

## Phase 2 — Backend: serializer + helpers

- [ ] New file `api/pkg/server/websocket_external_agent_sync.go` (or extend existing one if it appears in a different name on the branch):
  - [ ] `maxTranscriptBytes = 400000` constant
  - [ ] `serializeTranscript(interactions []*types.Interaction, maxBytes int) string` — markdown output with user turns, agent responses, tool calls; truncation notice when exceeding `maxBytes`
  - [ ] `serializeAgentResponse(interaction *types.Interaction) string` — helper for one interaction's agent side
  - [ ] `agentNameForRuntime(runtime types.CodeAgentRuntime) string` and `runtimeForAgentName(name string) (types.CodeAgentRuntime, bool)` helpers (may already partly exist via `CodeAgentRuntime.ZedAgentName()` — confirm and reuse)

## Phase 3 — Backend: fork endpoint

- [ ] `POST /api/v1/sessions/{id}/fork` — register route in `api/pkg/server/server.go`
- [ ] `forkSession` handler in `api/pkg/server/session_handlers.go`:
  - [ ] Accepts `{ helix_app_id?, code_agent_runtime? }`
  - [ ] Resolves target runtime via app lookup if `helix_app_id` given
  - [ ] Validates: source exists, user authorized, source is `zed_external`, source is not paused, target runtime differs from source
  - [ ] Returns `{ new_session_id }`
  - [ ] Swagger annotations
- [ ] Factor `forkSessionFromParent(ctx, parent, targetRuntime, agentName) (*Session, error)` helper:
  - [ ] Deep-copies project / user / workspace / system prompt / model fields from parent
  - [ ] Sets fork-lineage metadata (`ParentSessionID`, `ForkedAt`, `ForkedAtInteractionID`)
  - [ ] Inserts the new session row + provisions the desktop container via existing `externalAgentExecutor.StartDesktop`
- [ ] On success, mark parent paused: `Metadata.Paused = true`, `PausedReason = "forked_to:<child_id>"`, `PausedAt = now`
- [ ] Create the `fork_seed` synthetic interaction on the child:
  - [ ] `Trigger = TriggerForkSeed`
  - [ ] `PromptMessage = "Session forked from <parent_id> at turn N"`
  - [ ] `ResponseMessage = serializeTranscript(parent.interactions, maxTranscriptBytes)`
- [ ] Run `./stack update_openapi` to regenerate the API client

## Phase 4 — Backend: seed injection (websocket layer)

- [ ] In the websocket external-agent sync path, before sending the first user message to the agent:
  - [ ] Detect `ZedThreadID == ""` AND a `fork_seed` interaction exists on the session
  - [ ] Prepend `fork_seed.ResponseMessage` to the outgoing user message
  - [ ] Log `transcript_len > 0` for verification
- [ ] Confirm subsequent messages don't re-prepend the seed (no loop / duplicate context)

## Phase 5 — Backend: pause enforcement

- [ ] Introduce `requireUnpaused(session *types.Session) *system.HTTPError` helper returning HTTP 409 with `"session is paused (reason: X)"` when paused, nil otherwise
- [ ] Wire into all message-ingress paths:
  - [ ] `sendSessionMessage` (POST `/sessions/{id}/messages`)
  - [ ] `chatSession` (POST `/sessions/chat` — when an existing session id is targeted)
  - [ ] `NotifyExternalAgentOfNewInteraction` (whichever helper owns the producer side)
  - [ ] `pickupWaitingInteraction` (queue consumer)
  - [ ] `sendQueuedPromptToSession` (chat-panel queue-based path)
- [ ] Confirm in-flight `waiting` interactions are NOT cancelled by pause (pause is "no new input", not "kill the agent")

## Phase 6 — Backend: remove the old switch-agent path

> **No-op** in this redesign — the in-place mutation code was never pushed. Skip the phase entirely.

## Phase 7 — Frontend: chat-panel fork dropdown

- [ ] In `frontend/src/components/tasks/SpecTaskDetailContent.tsx`, add a new `AgentDropdown` instance in the chat panel header (separate from the existing settings-sidebar instance at line 1404 which is for task-creation/edit):
  - [ ] Value = the session's current `CodeAgentRuntime` / `ZedAgentName`
  - [ ] `onChange = handleFork(newAgent)`:
    - [ ] Call `POST /api/v1/sessions/{id}/fork` via the generated API client with `{ helix_app_id }`
    - [ ] On success: `router.navigate('session', { session_id: result.new_session_id })`
    - [ ] On error (`null` return): revert dropdown selection + show error snackbar
  - [ ] Disable when `session.Metadata.Paused === true` (the user should fork from the *active* descendant, not the paused parent)

## Phase 8 — Frontend: lineage + pause UI

- [ ] `PausedBanner` component (new) — `frontend/src/components/session/PausedBanner.tsx`:
  - [ ] Renders when `session.Metadata.Paused`
  - [ ] Reason text + link to child if `PausedReason` starts with `"forked_to:"`
- [ ] Wire into `EmbeddedSessionView.tsx`:
  - [ ] Show `PausedBanner` at the top of the view
  - [ ] Disable the chat input box when paused
- [ ] `ForkBadge` component (new) — appears on child sessions in the session header: "Forked from <parent_id>" link
- [ ] Render `fork_seed` interaction in `EmbeddedSessionView.tsx`:
  - [ ] Inline divider (horizontal rule + centered text)
  - [ ] Centered text: "Forked from <parent> with N interactions of context"
  - [ ] Expandable disclosure showing the raw transcript content (`ResponseMessage`)

## Phase 9 — E2E test coverage

- [ ] Add `ForkSession` test helper to `api/pkg/server/test_helpers.go` (mirror the in-process style used by `WebSocketSyncSuite`)
- [ ] Phase 13: fork from session A (zed-agent) to session B (claude); verify:
  - [ ] B has different session id from A
  - [ ] B has `Metadata.ParentSessionID == A.id`
  - [ ] A has `Metadata.Paused == true`, `PausedReason == "forked_to:<B>"`
  - [ ] B has exactly one `fork_seed` interaction
  - [ ] First user message on B → API logs show `transcript_len > 0` (seed injection happened)
  - [ ] Claude on B can recall what zed-agent said on A (LLM-asserted)
- [ ] Phase 14: fork from B (claude) to a new session C (zed-agent); verify:
  - [ ] C has `Metadata.ParentSessionID == B.id` (chain depth 2)
  - [ ] B has `Metadata.Paused == true`, `PausedReason == "forked_to:<C>"`
  - [ ] C's seed includes A's transcript + B's transcript (the full lineage is in B's `fork_seed` + B's interactions, which `serializeTranscript` walks at fork time)
  - [ ] zed-agent on C can recall content from both A and B
- [ ] Phase 15 (new): send a message to a paused session → HTTP 409
- [ ] Content-validator: skip empty-`ResponseEntries` check for `fork_seed` markers

## Phase 10 — Manual verification

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
