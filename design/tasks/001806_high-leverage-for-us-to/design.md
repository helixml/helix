# Design: Mid-Session Agent Switching via Fork-and-Pause

## Investigation Summary

Investigated three systems: ACP (Agent Communication Protocol) between Zed and external agents, Zed's thread/agent architecture, and the Helix session database. The goal is letting a user pick a different agentic framework (Claude Code, Qwen Code, Codex, Gemini, Zed built-in) for a running session without losing the context they've built up.

## Core Design Decision: Fork-and-Pause, Not Hot-Swap

A "session" in Helix is conceptually one (agent framework, model, conversation thread) running together. Switching the framework on a running session is treated as a **fork**, not a mutation:

1. Create a **new session** with the target framework + model. Same project, same user, same workspace; new session id; `Metadata.ParentSessionID` points back to the source.
2. **Seed** the new session with the parent's full event stream so the new agent starts having "already heard" everything the previous agent did.
3. **Pause** the parent session — it stops accepting new input but stays intact as a frozen checkpoint. The user can resume it, fork from it again, or compare against it.
4. **Navigate** the chat panel to the new session id.

Each session is therefore **monomorphic** in its agent framework: one row, one framework, one thread, one timeline. The list of sessions a user sees becomes a fork tree, not a flat list of in-place mutations.

### Why fork-and-pause, not in-place mutation?

Earlier iterations of this work mutated the live session row: swap `ZedAgentName`, swap `CodeAgentRuntime`, clear or restore `ZedThreadID`, swap an in-memory `contextMappings` entry, push the old thread into a `drainingThreadIDs` set so late events get dropped, and inject a partial transcript on the first message after switch-back. The end-to-end behaviour worked, but every one of those steps had to land atomically or state quietly diverged:

- A late event arriving 200ms after the switch routed to the new thread.
- A thread restored from `AgentThreadHistory` but `contextMappings` not re-added → events dropped.
- The frontend dropdown advanced before the backend update committed → next message went to the wrong agent.
- `PendingTranscriptSince` cleared before the message actually shipped → catch-up lost.

Forking sidesteps all of it. The old session keeps its threads, its mapping, its queue — untouched. The new session starts clean and gets its context through one well-defined seed step. No cross-session mutable state, no atomic-update problems, no late-event reconciliation.

It also makes a richer product:

- Sessions become a navigable lineage. Users can see "I tried this with Claude, then forked to Qwen at turn 12, then forked back to Claude at turn 18."
- Side-by-side comparison becomes natural: pick any two points on the lineage and diff their outputs.
- Pausing is a useful primitive in its own right (manual pause, branching from an old conversation, A/B testing prompts).

The earlier branch's `AgentThreadHistory`, `PendingTranscriptSince`, `drainingThreadIDs`, switch-back partial-transcript filter, and waiting-state rejection all become unnecessary in this model.

## How fork works end-to-end

```
User picks "Claude Code" from the AgentDropdown on session A (currently running zed_agent)
  ↓
POST /api/v1/sessions/{A}/fork  { helix_app_id: "app_xxx" }
  ↓
Backend:
  1. Validate: source session exists, user authorized, target app is zed_external with a valid runtime
  2. Create session B: same project, user, workspace, system prompt;
     Metadata.AgentType = "zed_external"
     Metadata.CodeAgentRuntime = target
     Metadata.ZedAgentName = mapped
     Metadata.ParentSessionID = A
     Metadata.ForkedAt = now
     Metadata.ForkedAtInteractionID = last completed interaction on A
  3. Mark session A paused: Metadata.Paused = true, Metadata.PausedReason = "forked_to:<B>"
  4. Snapshot A's interactions for the seed (deep copy of the slice — not a DB pointer; A may continue to receive late events for its own thread, but B's seed is fixed)
  5. Create one synthetic system interaction on B with Trigger="fork_seed" containing the serialized transcript of A — visible in the UI as a divider, used by the websocket layer as the seed message
  6. Provision B's desktop container (same path as session start) — settings already pre-configure all agents, so no settings rewrite
  7. Return { new_session_id: B } to the frontend
  ↓
Frontend navigates to /sessions/{B}; chat panel re-mounts on B
  ↓
On B's first real user message, the websocket layer detects ZedThreadID=="" and prepends the seed transcript before sending to the agent. After that, B runs as a normal session for the rest of its life.
```

### Seeding mechanism

The serializer that already exists (`serializeTranscript` in `websocket_external_agent_sync.go`) turns a list of `Interaction` rows into a markdown transcript with user turns, agent responses, and tool calls. That same function is the seed.

Two equivalent ways to deliver the seed; we pick the simpler one:

- **(chosen) On-first-message injection.** The seed is stored as a `fork_seed` interaction on B but is *also* prepended to B's first real user message at the websocket layer — same code path the earlier branch already used for new-thread transcript injection. The `fork_seed` interaction is the UI-visible record; the prepend is what the agent actually receives.
- **(rejected) Synthetic first turn.** Send the transcript as a no-op first user turn to the agent. Cleaner conceptually but produces a wasted agent response and pollutes the timeline.

### Pausing semantics

Pausing a session means:

- `Metadata.Paused = true`, `Metadata.PausedReason` set (`"forked_to:<id>"` or `"user_paused"`)
- The chat panel disables the input box on paused sessions, shows a "paused — forked to X" banner with a link to the child
- API rejects new `POST /sessions/{id}/messages` and `POST /sessions/chat` for paused sessions (HTTP 409 with explanation)
- Any in-flight `waiting` interaction on A is allowed to complete naturally — pausing is "no new input", not "kill the agent"
- The desktop container for A may be torn down on a configurable idle timer (out of scope for v1; v1 just keeps it alive until normal session lifecycle reaps it)

Unpausing is also a primitive (`POST /sessions/{id}/unpause`) but v1 only exposes it via "duplicate session" from the UI — i.e. forking off a paused session into a fresh one. We do not allow resuming an already-paused session into the in-place active state in v1, because that re-introduces the dual-active problem.

### Container handling

The existing `generateAgentServerConfig` change (pre-configuring all agents in `settings.json`) means session B's container has all agent backends ready from boot. There is no settings rewrite or process restart when B is provisioned — the container picks the configured agent based on B's `CodeAgentRuntime`, and the other agent processes never spawn (lazy ACP startup).

Session A keeps its existing container until normal lifecycle reaps it. This is the simplest path; an optimization in a later phase is to share the container between paused parent and active child (same workspace mount), but that requires changes to `wolf_executor` ownership and we don't need it for v1.

### Lineage in the UI

The session detail page surfaces lineage:

- Header on B shows "Forked from <A> at turn N" with a clickable parent link.
- Header on A shows "Paused — forked to <B>" with a clickable child link.
- A future "fork tree" view can render the whole lineage as a directed graph, but v1 ships with just the parent/child link badges.

The session list groups forks under their root for default sorting (chronological by root, then by fork depth). Users can flatten the view if they want.

### Frontend changes

- `AgentDropdown` in the chat panel header: same component as before; `onChange` now calls `POST /sessions/{id}/fork` instead of `/switch-agent`. On success, navigate the chat panel to the returned new session id. On error (e.g. paused source, unauthorized), revert the dropdown and show a snackbar — same null-on-error pattern as `api.post` returns.
- New "Paused" banner component on `EmbeddedSessionView`: appears when `session.Metadata.Paused` is true; renders the reason + link to child / unpause action.
- The `fork_seed` system interaction renders the same way the prior `agent_switch` divider did: horizontal rule + centered "Forked from <parent> with N interactions of context" text. The actual transcript content is hidden behind an expandable disclosure (most users won't need to see it).
- Session list: badge sessions with a fork indicator + parent link; group child sessions visually under their root (collapsible).

### What survives from the earlier implementation

Roughly half the earlier work survives the redesign:

| Piece | Survives? | Why |
|---|---|---|
| `serializeTranscript` / `serializeAgentResponse` in `websocket_external_agent_sync.go` | yes | This is the seed mechanism. Used unchanged. |
| `maybePrependTranscript` (first-message prepend) | yes, simplified | Single trigger condition (new session with seed) instead of two (new thread OR thread reuse). |
| `generateAgentServerConfig` pre-configures all agents | yes | New session's container needs the target agent immediately. |
| `AgentDropdown` placement in chat panel header | yes | UI affordance is correct; what it calls changes. |
| `agent_switch` system interaction → `fork_seed` interaction | yes, renamed | Same visual marker pattern, different trigger string. |
| `maxTranscriptBytes` = 400KB + truncation notice | yes | Same context-window concern applies to the seed. |
| `AgentThreadHistory` + `PendingTranscriptSince` + thread reuse logic | no | Each session is monomorphic; no concept of switching back into the same session. |
| `drainingThreadIDs` + late-event drop | no | Old session keeps its threads; no cross-routing. |
| Mid-flight `waiting`-state rejection | no | Forking is always safe; old session continues its waiting interaction independently. |
| `switchAgentSession` HTTP handler | replaced | New `forkSession` handler; entirely different code path. |
| `runtimeForAgentName` helper | yes | Still useful for the inverse mapping. |
| `agentNameForRuntime` helper | yes | Same. |
| E2E test Phase 13 / 14 (agent switch + switch-back) | rewritten | Phase 13 becomes "fork from session A to session B, verify seed delivery"; Phase 14 becomes "fork from B back to a new C, verify lineage chain". |

## Data model

### `types.SessionMetadata` additions

```go
type SessionMetadata struct {
    // ... existing fields ...

    // Fork lineage — set on a session created by forking from a parent.
    ParentSessionID        string    `json:"parent_session_id,omitempty"`
    ForkedAt               time.Time `json:"forked_at,omitempty"`
    ForkedAtInteractionID  string    `json:"forked_at_interaction_id,omitempty"`

    // Pause state — sessions cannot accept new messages while paused.
    Paused       bool      `json:"paused,omitempty"`
    PausedReason string    `json:"paused_reason,omitempty"` // e.g. "forked_to:<id>", "user_paused"
    PausedAt     time.Time `json:"paused_at,omitempty"`
}
```

No new DB table — lineage lives in `Metadata` (already JSON-serialized on the `sessions.config` column). Children link to parents via `ParentSessionID`; queries that need "all forks of X" do a JSONB lookup. If lineage queries become hot, a `parent_session_id` indexed column can be added later.

### New synthetic interaction trigger

`Trigger="fork_seed"` on a single interaction on each forked session, containing:

- `PromptMessage`: e.g. `"Session forked from ses_xxx at turn 12"`
- `ResponseMessage`: the serialized transcript (used by `maybePrependTranscript` to inject into the first real message, and rendered behind a disclosure in the UI)
- `Created`: same as `Metadata.ForkedAt`

### Endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/sessions/{id}/fork` | Body: `{ helix_app_id?, code_agent_runtime? }`. Returns `{ new_session_id }`. Pauses source. |
| POST | `/api/v1/sessions/{id}/pause` | Manual pause. Body: `{ reason? }`. (Stretch goal for v1.) |
| POST | `/api/v1/sessions/{id}/unpause` | Manual unpause — only allowed if no descendant is currently active on the same workspace. (Stretch goal for v1.) |

The `/switch-agent` endpoint from the earlier branch is removed.

## Validation matrix (replaces the earlier 20-case matrix)

| # | Scenario | Expected |
|---|---|---|
| 1 | Fork to a different runtime, no waiting interaction | HTTP 200, new session id returned, parent paused, child seeded |
| 2 | Fork while parent has a waiting interaction | HTTP 200 — parent's interaction completes on its own thread; child starts clean |
| 3 | Fork to same runtime | HTTP 400 `"source session is already using <runtime>"` (no-op forks are pointless; explicit "duplicate" endpoint for that later) |
| 4 | Fork from a paused session | HTTP 409 `"source session is paused; fork from its active descendant instead"` |
| 5 | Fork to a non-zed_external app | HTTP 400 `"target app has no zed_external assistant"` |
| 6 | Fork to nonexistent app | HTTP 400 `"failed to look up app"` |
| 7 | Fork from nonexistent session | HTTP 404 `"session not found"` |
| 8 | Fork without auth | HTTP 401 |
| 9 | Send message to paused session | HTTP 409 `"session is paused"` |
| 10 | Frontend: dropdown switches → page navigates to new session id | navigation happens on success, dropdown reverts on error |
| 11 | Frontend: parent session detail shows "forked to <child>" link | rendered |
| 12 | Frontend: child session detail shows "forked from <parent>" link + fork_seed disclosure | rendered |
| 13 | Seed transcript: parent had N completed interactions → child's first agent message receives serialized transcript with all N + the user's new message | verified via API logs (`transcript_len` > 0 on first message) |
| 14 | Lazy spawn preserved: forking to a runtime never used in the container does not pre-spawn other agent processes | verified via `ps` in container |
| 15 | Container handling: child gets its own desktop container; parent's container is untouched | verified via `docker ps` |

## Codebase patterns

- **Fork endpoint:** new `forkSession` in `api/pkg/server/session_handlers.go` — replaces `switchAgentSession`.
- **Pause/unpause:** new `pauseSession` / `unpauseSession` in same file (stretch).
- **Session creation:** child session uses existing `WriteSession` + `WriteInteractions` + `externalAgentExecutor.StartDesktop` paths — same as initial session creation. Factor a `forkSessionFromParent(ctx, parent, target)` helper that returns the new session.
- **Seed injection:** `maybePrependTranscript` simplified to one condition (`ZedThreadID == ""` AND `fork_seed` interaction exists) and reads the transcript from the `fork_seed.ResponseMessage` field instead of re-serializing every time.
- **Pause enforcement:** `sendSessionMessage`, `pickupWaitingInteraction`, `sendQueuedPromptToSession`, and chat-message routing all check `Metadata.Paused` and short-circuit with the same HTTP 409 when paused. Centralize via a `requireUnpaused(session)` helper.
- **Settings daemon:** unchanged — already pre-configures all agents.
- **Frontend dropdown:** `SpecTaskDetailContent.tsx` → `handleAgentSwitch` becomes `handleFork`; calls fork endpoint, navigates to new session id on success.
- **EmbeddedSessionView:** adds `PausedBanner` and `ForkBadge` components.

## Migration notes

Existing sessions on `main` that have `AgentThreadHistory` / `PendingTranscriptSince` in their metadata are harmless — the new code path ignores those fields. They can be cleaned up by a background job or just left as dead JSON.

The earlier branch's E2E test phases (13, 14) need rewriting against the fork model. The earlier "switch + thread reuse" assertions become "fork + lineage" assertions.

## Open questions

1. **When does a paused session's desktop container get reaped?** v1: leave it until the normal session-idle reaper picks it up. v2: explicit "release container" action on pause, or shared-container-with-child to avoid duplication.
2. **Should fork happen synchronously (block on container provisioning) or async (return id immediately, frontend polls until ready)?** Lean async to match how initial session creation already works — the child shows up in "provisioning" state, the chat panel handles that state gracefully.
3. **Do we want a `/duplicate` endpoint distinct from `/fork`?** Duplicate = fork to the same agent (clone the session as a sandbox to try a different prompt). Useful UX but out of scope for v1.
4. **Pause-of-paused — can a paused session be force-paused again with a different reason?** v1: no, pause is idempotent; subsequent pauses no-op.
