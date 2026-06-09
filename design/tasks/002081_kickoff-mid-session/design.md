# Design: Mid-Session Agent Switching via Fork-and-Pause

> **Predecessor:** Task [001806](../001806_high-leverage-for-us-to/) — superseded. Its `design.md` is preserved as the record of the in-place mutation approach we rejected. See [kickoff.md](kickoff.md) for the architectural-pivot summary.

## Reality check on starting state

Audit of helix git history (2026-06-09) confirms that **no code from the in-place mutation attempt was ever pushed to the helix repo**. Cross-branch `git log -S` searches for `serializeTranscript`, `maybePrependTranscript`, `switchAgentSession`, `AgentThreadHistory`, `drainingThreadIDs`, `fork_seed` all return only `helix-specs` markdown commits. `feature/001806-high-leverage-for-us-to` is byte-identical to `main`.

Implications:
- "Phase 0: branch hygiene" collapses to `git checkout feature/001806-high-leverage-for-us-to` (no revert needed).
- "Phase 5: remove old switch-agent path" is a no-op (nothing to delete).
- Every component listed as "Keep" or "Keep, simplify" in the 001806 design's survivors table — `serializeTranscript`, `serializeAgentResponse`, `maybePrependTranscript`, `agentNameForRuntime`, `runtimeForAgentName`, `agent_switch` → `fork_seed` renderer, `AgentDropdown` in chat panel header — must be **written from scratch**.
- The `AgentDropdown` component itself exists (in `frontend/src/components/agent/`) and is used in `NewSpecTaskForm.tsx` (line 1107) and `SpecTaskDetailContent.tsx` (line 1404). Both existing uses are in **settings sidebar** contexts (`handleAgentChange` selects the agent at task-creation/edit time). We need a **new** instance in the **chat panel header** for forking.
- `generateAgentServerConfig` in `api/cmd/settings-sync-daemon/main.go` does exist and is already invoked. We need to confirm it pre-configures all agent backends — if not, that's a small extra task.

## Core Design Decision: Fork-and-Pause, Not Hot-Swap

A "session" in Helix is conceptually one (agent framework, model, conversation thread) running together. Switching the framework on a running session is treated as a **fork**, not a mutation:

1. Create a **new session** with the target framework + model. Same project, same user, same workspace; new session id; `Metadata.ParentSessionID` points back to the source.
2. **Seed** the new session with the parent's full event stream so the new agent starts having "already heard" everything the previous agent did.
3. **Pause** the parent session — it stops accepting new input but stays intact as a frozen checkpoint. The user can fork from it again or compare against it.
4. **Navigate** the chat panel to the new session id.

Each session is therefore **monomorphic** in its agent framework: one row, one framework, one thread, one timeline. The list of sessions a user sees becomes a fork tree, not a flat list of in-place mutations.

### Why fork-and-pause, not in-place mutation?

See `../001806_high-leverage-for-us-to/design.md` for the full story; the short version:

- In-place mutation required atomic updates across `ZedAgentName`, `CodeAgentRuntime`, `ZedThreadID`, `contextMappings`, `drainingThreadIDs`, `PendingTranscriptSince`. Any divergence quietly broke state.
- Fork-and-pause has no cross-session mutable state, no atomic-update problem, no late-event reconciliation. The parent's thread/mapping/queue are untouched; the child starts clean.
- As a side benefit: navigable lineage, side-by-side comparison becomes natural, pausing is a useful primitive in its own right.

## How fork works end-to-end

```
User picks "Claude Code" from the chat-panel AgentDropdown on session A (currently running zed_agent)
  ↓
POST /api/v1/sessions/{A}/fork  { helix_app_id: "app_xxx" }
  ↓
Backend:
  1. Validate: source session exists, user authorized, target app is zed_external with a valid runtime
  2. Build child session B (deep-copy fields from A):
     Metadata.AgentType = "zed_external"
     Metadata.CodeAgentRuntime = target
     Metadata.ZedAgentName = mapped (via agentNameForRuntime)
     Metadata.ParentSessionID = A.id
     Metadata.ForkedAt = now
     Metadata.ForkedAtInteractionID = id of last completed interaction on A (or "" if none)
     Project, User, Workspace, SystemPrompt, ModelName copied from A
  3. Snapshot A's interactions into a transcript (serializeTranscript, capped at maxTranscriptBytes)
  4. Create one synthetic system interaction on B with Trigger="fork_seed":
        PromptMessage  = "Session forked from <A> at turn N"
        ResponseMessage = <serialized transcript>
        Created        = Metadata.ForkedAt
  5. Mark A paused: Metadata.Paused = true, PausedReason = "forked_to:<B>", PausedAt = now
  6. Provision B's desktop container (same path as initial session start)
  7. Return { new_session_id: B } to the frontend
  ↓
Frontend navigates to /sessions/{B}; chat panel re-mounts on B
  ↓
On B's first real user message, the websocket layer detects ZedThreadID=="" and the
presence of a fork_seed interaction → prepends the seed transcript to the user message
before sending to the agent. After that, B runs as a normal session for the rest of its life.
```

### Seeding mechanism

`serializeTranscript` is the seed. It turns a list of `Interaction` rows into a markdown transcript with user turns, agent responses, and tool calls. The output is stored once on the `fork_seed.ResponseMessage` field at fork time (so it's immutable from then on) and re-read by `maybePrependTranscript` on the child's first message.

Two equivalent ways to deliver the seed; we pick **(a)** for simplicity:

- **(a) On-first-message injection** — chosen. The seed is stored as a `fork_seed` interaction on B but is also prepended to B's first real user message at the websocket layer. The `fork_seed` interaction is the UI-visible record; the prepend is what the agent actually receives.
- **(b) Synthetic first turn** — rejected. Send the transcript as a no-op first user turn. Cleaner conceptually but produces a wasted agent response and pollutes the timeline.

### Pausing semantics

Pausing a session means:

- `Metadata.Paused = true`, `PausedReason` set (`"forked_to:<id>"` is the only producer in v1)
- The chat panel disables the input box on paused sessions and shows a "paused — forked to X" banner with a link to the child
- API rejects new `POST /sessions/{id}/messages` and `POST /sessions/chat` (when targeting an existing session) with HTTP 409
- Any in-flight `waiting` interaction on A is allowed to complete naturally — pausing is "no new input", not "kill the agent"
- The desktop container for A may be torn down on a configurable idle timer (out of scope for v1)

v1 does **not** expose a manual unpause. The only way to "use the conversation again" is to fork off the paused session — this avoids the dual-active problem.

### Container handling

The existing `generateAgentServerConfig` change (pre-configuring all agents in `settings.json`) means session B's container has all agent backends ready from boot. There is no settings rewrite or process restart when B is provisioned — the container picks the configured agent based on B's `CodeAgentRuntime`, and the other agent processes never spawn (lazy ACP startup).

Session A keeps its existing container until normal lifecycle reaps it.

### Lineage in the UI

- Header on B: "Forked from <A> at turn N" with a clickable parent link.
- Header on A: "Paused — forked to <B>" with a clickable child link.
- Chat-panel input on A: disabled, with the paused banner above it.
- `fork_seed` interaction on B: rendered as a horizontal-rule divider with "Forked from <parent> with N interactions of context" centered. The transcript content is behind an expandable disclosure (most users won't need to see it).

A future "fork tree" view can render the whole lineage as a directed graph, but v1 ships with just the parent/child link badges.

### Frontend changes

- **Chat-panel AgentDropdown** — a new instance separate from the existing settings-sidebar one. Sits in the chat panel header. `onChange` calls `POST /sessions/{id}/fork` via the generated API client. On success, `router.navigate('session', { session_id: result.new_session_id })`. On error, revert the dropdown selection and show an error snackbar.
- **PausedBanner** — appears on `EmbeddedSessionView` when `session.Metadata.Paused` is true. Renders the reason + link to child if `PausedReason` starts with `"forked_to:"`. Disables the chat input.
- **ForkBadge** — appears on child sessions in the session header. "Forked from <parent>" link.
- **fork_seed divider renderer** — horizontal rule + centered "Forked from <parent> with N interactions of context" text. Expandable disclosure with the raw transcript.

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
    PausedReason string    `json:"paused_reason,omitempty"` // e.g. "forked_to:<id>"
    PausedAt     time.Time `json:"paused_at,omitempty"`
}
```

No new DB table — lineage lives in `Metadata` (already JSON-serialized on the `sessions.config` column). Children link to parents via `ParentSessionID`; queries that need "all forks of X" do a JSONB lookup. If lineage queries become hot, a `parent_session_id` indexed column can be added later.

### New synthetic interaction trigger

`Trigger="fork_seed"` on a single interaction on each forked session, containing:

- `PromptMessage`: `"Session forked from <parent_id> at turn <N>"`
- `ResponseMessage`: the serialized transcript (used by `maybePrependTranscript` to inject into the first real message, and rendered behind a disclosure in the UI)
- `Created`: same as `Metadata.ForkedAt`

### Endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/sessions/{id}/fork` | Body: `{ helix_app_id?, code_agent_runtime? }`. Returns `{ new_session_id }`. Pauses source. |

`/pause` and `/unpause` are deferred from v1.

## Validation matrix

| # | Scenario | Expected |
|---|---|---|
| 1 | Fork to a different runtime, no waiting interaction | HTTP 200, new session id returned, parent paused, child seeded |
| 2 | Fork while parent has a waiting interaction | HTTP 200 — parent's interaction completes on its own thread; child starts clean |
| 3 | Fork to same runtime | HTTP 400 `"source session is already using <runtime>"` (no-op forks pointless) |
| 4 | Fork from a paused session | HTTP 409 `"source session is paused; fork from its active descendant instead"` |
| 5 | Fork to a non-zed_external app | HTTP 400 `"target app has no zed_external assistant"` |
| 6 | Fork to nonexistent app | HTTP 400 `"failed to look up app"` |
| 7 | Fork from nonexistent session | HTTP 404 `"session not found"` |
| 8 | Fork without auth | HTTP 401 |
| 9 | Send message to paused session | HTTP 409 `"session is paused"` |
| 10 | Frontend: dropdown switches → page navigates to new session id | navigation happens on success, dropdown reverts on error |
| 11 | Frontend: parent session detail shows "forked to <child>" link | rendered |
| 12 | Frontend: child session detail shows "forked from <parent>" link + fork_seed disclosure | rendered |
| 13 | Seed transcript: parent had N completed interactions → child's first agent message receives serialized transcript with all N + the user's new message | verified via API logs (`transcript_len > 0` on first message) |
| 14 | Lazy spawn preserved: forking to a runtime never used in the container does not pre-spawn other agent processes | verified via `ps` in container |
| 15 | Container handling: child gets its own desktop container; parent's container is untouched | verified via `docker ps` |

## Codebase patterns

- **Fork endpoint:** new `forkSession` in `api/pkg/server/session_handlers.go`. Swagger annotations; `./stack update_openapi` to regenerate the client.
- **Session creation helper:** factor `forkSessionFromParent(ctx, parent, targetRuntime, agentName) (*Session, error)` that returns the new session. Deep-copies project / user / workspace / system-prompt / model fields, sets fork-lineage metadata, inserts the new session row, kicks off `externalAgentExecutor.StartDesktop` (same path as initial session start).
- **Pause helper:** `requireUnpaused(session) *system.HTTPError` returning HTTP 409 when paused, nil otherwise. Wired into `sendSessionMessage`, `chatSession` (when targeting an existing id), `NotifyExternalAgentOfNewInteraction`, `pickupWaitingInteraction`, `sendQueuedPromptToSession`.
- **Serializer:** new file `api/pkg/server/websocket_external_agent_sync.go` (currently doesn't exist) hosting `serializeTranscript`, `serializeAgentResponse`, `maxTranscriptBytes = 400000` constant, and `maybePrependTranscript`.
- **Frontend dropdown:** new `AgentDropdown` instance in the chat panel header of `SpecTaskDetailContent.tsx` (and potentially also `EmbeddedSessionView` if that's the right surface — confirm during impl). Distinct from the existing settings-sidebar dropdown.
- **EmbeddedSessionView:** adds `PausedBanner`, `ForkBadge`, and a renderer branch for `fork_seed` interactions.

## Migration notes

No DB migration needed — all new fields live on the JSONB `config` column. Existing sessions without the new fields decode to zero values (`Paused=false`, empty lineage strings), which is the correct default.

## Open questions

1. **When does a paused session's desktop container get reaped?** v1: leave it until the normal session-idle reaper picks it up. v2: explicit "release container" action on pause, or shared-container-with-child to avoid duplication.
2. **Should fork happen synchronously (block on container provisioning) or async (return id immediately, frontend polls until ready)?** Lean async to match how initial session creation already works — the child shows up in "provisioning" state, the chat panel handles that state gracefully.
3. **Do we want a `/duplicate` endpoint distinct from `/fork`?** Duplicate = fork to the same agent. Useful UX but out of scope for v1.
4. **Pause-of-paused — can a paused session be force-paused again with a different reason?** v1: no, pause is idempotent; subsequent pauses no-op.
