# Implementation Tasks (Fork-and-Pause Redesign)

> **Status note (2026-06-09):** The original implementation (in-place `switch-agent` with thread reuse) is in `feature/001806-high-leverage-for-us-to`, merged from main. We are pivoting to a **fork-and-pause** design (see `design.md`). The tasks below reflect the redesign. Pieces of the prior work that survive are marked **[reuse]**; pieces that get removed are listed at the bottom.

## Phase 0 — Branch hygiene

- [ ] Decide: keep working on the same feature branch (revert the in-place mutation pieces in a single commit, then build fork-and-pause on top) **or** start a fresh branch off main with only the survivor pieces cherry-picked
- [ ] Drop the backup stash `stash@{0}` (`pre-main-merge: thread-reuse + UI`) once the surviving pieces are extracted
- [ ] Rebase / fast-forward the working branch to current `origin/main`

## Phase 1 — Data model

- [ ] Add `SessionMetadata.ParentSessionID`, `ForkedAt`, `ForkedAtInteractionID` (lineage)
- [ ] Add `SessionMetadata.Paused`, `PausedReason`, `PausedAt` (pause state)
- [ ] Remove `AgentThreadHistory`, `AgentThreadEntry`, `PendingTranscriptSince` from `SessionMetadata` (dead in the new model)
- [ ] Confirm Postgres JSON serialization round-trips the new fields (no migration — `config` column is already JSONB)

## Phase 2 — Backend: fork endpoint

- [ ] `POST /api/v1/sessions/{id}/fork` — handler `forkSession` in `api/pkg/server/session_handlers.go`
  - [ ] Accepts `{ helix_app_id?, code_agent_runtime? }`; resolves target runtime via app lookup if `helix_app_id` given (same logic as the earlier branch)
  - [ ] Validates: source exists, user authorized, source is `zed_external`, source is not paused, target runtime differs from source
  - [ ] Returns `{ new_session_id }`
- [ ] Factor `forkSessionFromParent(ctx, parent, targetRuntime, agentName) (*Session, error)` helper
  - [ ] Deep-copies project / user / workspace / system prompt / model fields from parent
  - [ ] Sets fork-lineage metadata
  - [ ] Inserts the new session row + provisions the desktop container via existing `externalAgentExecutor.StartDesktop`
- [ ] On success, mark parent paused: `Metadata.Paused = true`, `PausedReason = "forked_to:<child_id>"`, `PausedAt = now`
- [ ] Create the `fork_seed` synthetic interaction on the child:
  - [ ] `Trigger = "fork_seed"`
  - [ ] `PromptMessage = "Session forked from <parent_id> at turn N"`
  - [ ] `ResponseMessage = serializeTranscript(parent.interactions, maxTranscriptBytes)`

## Phase 3 — Backend: seed injection (websocket layer)

- [ ] Simplify `maybePrependTranscript` in `api/pkg/server/websocket_external_agent_sync.go`:
  - [ ] Single trigger: `ZedThreadID == ""` AND a `fork_seed` interaction exists on the session
  - [ ] Read transcript from `fork_seed.ResponseMessage` rather than re-serializing the parent's interactions on every call (the seed is captured at fork time and is immutable)
  - [ ] Remove `PendingTranscriptSince` handling (no thread-reuse path anymore)
- [ ] Keep `serializeTranscript` / `serializeAgentResponse` unchanged — they're used by the fork handler to populate `fork_seed.ResponseMessage` once
- [ ] Keep `maxTranscriptBytes = 400000` + truncation notice

## Phase 4 — Backend: pause enforcement

- [ ] Introduce `requireUnpaused(session) *system.HTTPError` helper returning HTTP 409 with `"session is paused (reason: X)"` when paused, nil otherwise
- [ ] Wire into all message-ingress paths:
  - [ ] `sendSessionMessage` (POST /sessions/{id}/messages)
  - [ ] `chatSession` (POST /sessions/chat — when an existing session id is targeted)
  - [ ] `NotifyExternalAgentOfNewInteraction`
  - [ ] `pickupWaitingInteraction`
  - [ ] `sendQueuedPromptToSession` (the queue-based path used by the chat panel — easy to miss)
- [ ] Stretch: `POST /sessions/{id}/pause` and `POST /sessions/{id}/unpause` manual endpoints

## Phase 5 — Backend: remove the old switch-agent path

- [ ] Delete `switchAgentSession` handler from `session_handlers.go`
- [ ] Delete the `/sessions/{id}/switch-agent` route registration in `server.go`
- [ ] Remove `drainingThreadIDs` field + `addDrainingThread` / `removeDrainingThread` / `isThreadDraining` helpers from `server.go` and `websocket_external_agent_sync.go` (no more cross-session thread routing)
- [ ] Remove the late-event drop guard at the top of `processExternalAgentSyncMessage`
- [ ] Delete `SwitchAgent` test helper from `test_helpers.go`
- [ ] Remove the in-place thread-reuse code from the earlier branch (`AgentThreadHistory` map manipulation, `PendingTranscriptSince` clearing)

## Phase 6 — Frontend: chat panel rewiring

- [ ] In `SpecTaskDetailContent.tsx`, rename `handleAgentSwitch` → `handleFork`:
  - [ ] Call `POST /api/v1/sessions/{id}/fork` with `{ helix_app_id }`
  - [ ] On success: `router.navigate('session', { session_id: result.new_session_id })` (or equivalent — confirm route name)
  - [ ] On error (`null` return): revert dropdown selection + show error snackbar
- [ ] Disable the `AgentDropdown` when the current session has `Metadata.Paused === true` (the user should fork from the *active* descendant, not the paused parent)

## Phase 7 — Frontend: lineage + pause UI

- [ ] `PausedBanner` component in `EmbeddedSessionView.tsx`:
  - [ ] Shows when `session.Metadata.Paused`
  - [ ] Renders reason + link to child if `PausedReason` starts with `"forked_to:"`
  - [ ] Disables the chat input box
- [ ] `ForkBadge` on child sessions: "Forked from <parent_id>" link in the session header
- [ ] Render `fork_seed` interaction:
  - [ ] Inline divider (mirrors the prior `agent_switch` divider styling)
  - [ ] Centered text: "Forked from <parent> with N interactions of context"
  - [ ] Expandable disclosure showing the raw transcript content (for debugging / user verification)
- [ ] Session list: group children under their root by default; flat-view toggle as a stretch goal

## Phase 8 — E2E test coverage (rewrite)

- [ ] Rewrite Phase 13: fork from session A (zed-agent) to session B (claude); verify:
  - [ ] B has different session id from A
  - [ ] B has `Metadata.ParentSessionID == A.id`
  - [ ] A has `Metadata.Paused == true`, `PausedReason == "forked_to:<B>"`
  - [ ] B has exactly one `fork_seed` interaction
  - [ ] First user message on B → API logs show `transcript_len > 0` (seed injection happened)
  - [ ] Claude on B can recall what zed-agent said on A (LLM-asserted)
- [ ] Rewrite Phase 14: fork from B (claude) to a NEW session C (zed-agent); verify:
  - [ ] C has `Metadata.ParentSessionID == B.id` (chain depth 2)
  - [ ] B has `Metadata.Paused == true`, `PausedReason == "forked_to:<C>"`
  - [ ] C's seed includes A's transcript + B's transcript (the full lineage is in the parent's `fork_seed.ResponseMessage`, which is included when C is created)
  - [ ] zed-agent on C can recall content from both A and B
- [ ] Phase 15 (new): send a message to a paused session → HTTP 409
- [ ] Update isolation validator: each session has its own ACP `message_id` sequence (already true by construction in the new model — no resets needed)
- [ ] Update content validator: skip empty-`ResponseEntries` check for `fork_seed` markers (same as the old `agent_switch` skip)

## Phase 9 — Spec updates

- [x] Rewrite `design.md` for fork-and-pause
- [x] Rewrite `tasks.md` (this file)
- [ ] Update `requirements.md` user stories — acceptance criteria item "Helix session ID remains the same" is wrong in the new model; replace with "Helix navigates to a new session id; the parent is preserved as a paused checkpoint"

## Phase 10 — Manual verification

- [ ] In the inner Helix UI, create a spec task → start a chat with one agent → fork to another agent
- [ ] Verify the chat panel navigates to the new session
- [ ] Verify the new agent acknowledges context from the prior session
- [ ] Navigate back to the parent → verify the paused banner + link to child
- [ ] Try sending a message in the paused session → verify it's blocked
- [ ] Fork the child to yet another agent → verify the chain shows lineage depth 2

---

## Earlier work that gets removed in this redesign

The following pieces from the existing branch are **no longer needed** in the fork-and-pause model and should be deleted as part of Phase 5:

| Component | File | Reason removed |
|---|---|---|
| `switchAgentSession` HTTP handler | `api/pkg/server/session_handlers.go` | Replaced by `forkSession` |
| `/sessions/{id}/switch-agent` route | `api/pkg/server/server.go` | Replaced by `/fork` |
| `AgentThreadHistory` map | `api/pkg/types/types.go`, `session_handlers.go`, `test_helpers.go` | Per-runtime thread caching is unneeded; each session is monomorphic |
| `AgentThreadEntry` struct | `api/pkg/types/types.go` | Same |
| `PendingTranscriptSince` field | `api/pkg/types/types.go`, `websocket_external_agent_sync.go` | No thread-reuse / catch-up path in the new model |
| `drainingThreadIDs` map + draining helpers | `api/pkg/server/server.go`, `websocket_external_agent_sync.go` | No cross-session thread routing means no late-event reconciliation needed |
| Waiting-state rejection at top of `switchAgentSession` | `session_handlers.go` | Forking is always safe; old session's waiting interaction is allowed to complete on its own |
| `SwitchAgent` test helper | `api/pkg/server/test_helpers.go` | Replaced by a `ForkSession` helper |
| Thread reuse branch of `maybePrependTranscript` | `websocket_external_agent_sync.go` | Single trigger condition (new session with `fork_seed`) instead of two |

## Earlier work that survives (do NOT remove)

| Component | File | Why kept |
|---|---|---|
| `runtimeForAgentName` helper | `session_handlers.go` | Still used for inverse mapping |
| `agentNameForRuntime` helper | `session_handlers.go` | Still used by fork handler |
| `serializeTranscript`, `serializeAgentResponse` | `websocket_external_agent_sync.go` | These are the seed mechanism |
| Pre-configure-all-agents in `generateAgentServerConfig` | `api/cmd/settings-sync-daemon/main.go` | Child sessions need the target agent ready |
| `AgentDropdown` placement in chat panel header | `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | UI affordance is right; `onChange` handler changes |
| `agent_switch` system interaction renderer (rename to `fork_seed`) | `frontend/src/components/session/EmbeddedSessionView.tsx` | Same divider pattern, different trigger name |
| `maxTranscriptBytes = 400000` + truncation notice | `websocket_external_agent_sync.go` | Same context-window concern applies to the seed |

## Open product questions (carried over from design.md)

- Container reaping policy for paused sessions
- Async vs synchronous fork (frontend polling vs blocking response)
- Whether to expose a `/duplicate` endpoint distinct from `/fork`
- Pause-of-paused semantics
