# Kickoff: Mid-session agent switching via fork-and-pause

> **Purpose of this document:** Paste this as the original prompt for a new spec task in meta.helix.ml that supersedes task 001806 (`high-leverage-for-us-to`). It explains *why* we are restarting that work with a different approach and gives the new task enough context to design and implement cleanly.

## The problem

A Helix session today is bound to one agentic framework (Zed built-in, Claude Code, Qwen Code, Codex, Gemini). If a user has been working with one framework and wants to switch — for example, they started a task with Claude, hit a tricky bit and want Qwen's tool-use, or they want to try the same task on a cheaper model — they have to start a new session by hand and re-explain the entire context.

## The decision: fork-and-pause, not hot-swap

The earlier attempt (task 001806) tried to **mutate** the running session: swap `CodeAgentRuntime`, swap `ZedAgentName`, juggle the `ZedThreadID`, push the old thread into a "draining" set so late events get dropped, inject a partial transcript on switch-back. It worked end-to-end (we have it running and validated against ~20 scenarios) but every step had to land atomically or state quietly diverged. Late events routed wrong, contextMappings drifted out of sync, the frontend dropdown could advance before the backend committed, and so on.

The new approach: **switching the agent forks the session**. Concretely:

1. `POST /api/v1/sessions/{A}/fork { helix_app_id: ... }` creates a new session `B` with the target framework + model. Same project, same user, same workspace, **new session id**. `B.Metadata.ParentSessionID = A`.
2. `B` is **seeded** with `A`'s full event stream — the existing `serializeTranscript` function turns `A`'s interactions into a markdown transcript that gets prepended to `B`'s first user message. The new agent starts having "already heard" everything the previous agent did.
3. `A` is **paused** — `Metadata.Paused = true`, `PausedReason = "forked_to:<B>"`. It stops accepting new input but the row stays intact as a frozen checkpoint, browsable, forkable again.
4. The frontend navigates the chat panel to `/sessions/B`.

Each session is therefore monomorphic in its agent: one row, one framework, one thread, one timeline. The list of sessions becomes a fork tree, not a flat list of polymorphic mutations.

## Why this is the right shape

**It eliminates a whole class of bugs.** No in-place mutation of live state means no atomic-update problems, no late-event reconciliation, no cross-thread routing. The old session keeps its thread, its mapping, its queue — untouched. The new session starts clean.

**It is a better product.** Sessions become a navigable lineage: "I tried this with Claude, then forked to Qwen at turn 12, then forked back to Claude at turn 18 for the implementation." Pausing is a useful primitive in its own right — manual pause, branching from an old conversation, A/B comparisons. The same fork primitive enables a future "duplicate session" feature for trying a different prompt on the same context.

**It composes with what already works.** Initial session creation already handles the "spin up a desktop container with a chosen agent" path. Forking re-uses it. The existing settings-sync-daemon change that pre-configures all agent backends in `settings.json` means the new session's container has the target agent ready instantly — no settings rewrite, no process restart. Lazy ACP spawn means unused agents never consume resources.

## What survives from the earlier work

Roughly half the earlier (001806) implementation carries over:

| Piece | Status |
|---|---|
| `serializeTranscript` / `serializeAgentResponse` in `websocket_external_agent_sync.go` | **Keep** — this is the seed mechanism |
| `maybePrependTranscript` (first-message prepend) | **Keep, simplify** — single trigger (new session with `fork_seed`) instead of two |
| `generateAgentServerConfig` pre-configures all agents in `settings.json` | **Keep** — new session's container needs the target agent ready |
| `AgentDropdown` placement in chat panel header | **Keep** — UI affordance is right; the `onChange` handler changes |
| `maxTranscriptBytes = 400000` + truncation notice | **Keep** — same context-window concern |
| `agentNameForRuntime` / `runtimeForAgentName` helpers | **Keep** |
| `agent_switch` system interaction renderer | **Keep, rename to `fork_seed`** — same divider styling |
| `AgentThreadHistory` + `AgentThreadEntry` + `PendingTranscriptSince` | **Remove** — no thread reuse |
| `drainingThreadIDs` map + draining helpers | **Remove** — no cross-session thread routing |
| Mid-flight `waiting`-state rejection | **Remove** — forking is always safe |
| `switchAgentSession` HTTP handler + `/switch-agent` route | **Replace** with `forkSession` + `/fork` |
| `SwitchAgent` test helper | **Replace** with `ForkSession` |
| E2E phases 13/14 (switch + switch-back) | **Rewrite** as fork + lineage chain |

## New data model additions

`types.SessionMetadata` gains six fields (all JSON-serialized into the existing `sessions.config` JSONB column — no schema migration):

```go
// Fork lineage
ParentSessionID       string    `json:"parent_session_id,omitempty"`
ForkedAt              time.Time `json:"forked_at,omitempty"`
ForkedAtInteractionID string    `json:"forked_at_interaction_id,omitempty"`

// Pause state — paused sessions cannot accept new messages
Paused       bool      `json:"paused,omitempty"`
PausedReason string    `json:"paused_reason,omitempty"`
PausedAt     time.Time `json:"paused_at,omitempty"`
```

One new synthetic interaction trigger: `Trigger="fork_seed"`, created on the child at fork time, with `ResponseMessage` containing the serialized parent transcript. The websocket layer reads from this field when injecting the seed into the child's first real message.

## Implementation phases

The detailed plan lives in `design/tasks/001806_high-leverage-for-us-to/{design.md,tasks.md}` on the `helix-specs` branch of the project repo at https://meta.helix.ml/orgs/helix/projects/prj_01kg02vqqyg178c1n2ydscn5fb (the new task should copy / adapt those files into its own task directory). Top-level phases:

1. **Branch hygiene** — decide whether to continue on the existing feature branch (revert the in-place mutation pieces, build fork-and-pause on top) or start fresh and cherry-pick the survivors.
2. **Data model** — add lineage + pause fields to `SessionMetadata`; remove `AgentThreadHistory` / `PendingTranscriptSince`.
3. **Backend: fork endpoint** — `POST /sessions/{id}/fork` returning `{ new_session_id }`; helper `forkSessionFromParent` that deep-copies project/user/workspace/system-prompt fields, sets lineage, provisions the desktop container, creates the `fork_seed` interaction.
4. **Backend: seed injection** — simplify `maybePrependTranscript` to one trigger condition (read transcript from the `fork_seed.ResponseMessage` field).
5. **Backend: pause enforcement** — `requireUnpaused(session)` helper wired into all message-ingress paths (`sendSessionMessage`, `chatSession`, `NotifyExternalAgentOfNewInteraction`, `pickupWaitingInteraction`, `sendQueuedPromptToSession`).
6. **Backend: remove old switch-agent path** — delete `switchAgentSession`, `drainingThreadIDs`, the late-event drop guard, `SwitchAgent` test helper, and the thread-reuse branch of `maybePrependTranscript`.
7. **Frontend: chat panel** — rename `handleAgentSwitch` to `handleFork`; navigate to the new session id on success; revert dropdown on error.
8. **Frontend: lineage + pause UI** — `PausedBanner` (shows on paused sessions with link to child); `ForkBadge` (on children, links back to parent); `fork_seed` divider (mirrors the old `agent_switch` divider styling with an expandable disclosure for the raw transcript).
9. **E2E test rewrite** — Phase 13: fork A→B, verify lineage + seed + recall; Phase 14: fork B→C, verify chain depth 2; Phase 15: send to paused session → 409.
10. **Manual verification** in the inner Helix UI.

## Open questions for the new task to settle

1. **Container reaping for paused sessions.** v1 keeps the parent's container alive until the normal idle reaper picks it up. v2 might explicitly release the container on pause, or share a container with the child (same workspace mount, change agent-server selection).
2. **Sync vs async fork.** Lean async — return the child id immediately, frontend handles the "provisioning" state — matching how initial session creation already works.
3. **`/duplicate` endpoint distinct from `/fork`.** Duplicate = fork to the same agent (try a different prompt on the same context). Useful UX, out of scope for v1.
4. **Pause-of-paused semantics.** v1: idempotent; subsequent pauses no-op.

## Acceptance for v1

- A user on a running session can pick a different agent from the chat panel dropdown.
- The system creates a new session with that agent, seeded with the parent's full event stream.
- The chat panel navigates to the new session.
- The new agent can recall what the previous agent did (verified end-to-end with an LLM assertion).
- The parent session is marked paused; its detail page shows a "forked to X" link; its chat input is disabled.
- Sending a message to a paused session returns HTTP 409 with a clear error.
- Forking again from the child produces a chain of depth 2 with correct lineage on both rows.

## What to do about task 001806

Close it as superseded. The existing implementation branch (`feature/001806-high-leverage-for-us-to`) can be kept around as a reference for the survivor pieces, or its survivors cherry-picked into a fresh branch for the new task. The detailed design/tasks files we wrote (under `design/tasks/001806_high-leverage-for-us-to/`) are also worth preserving as a record of the architectural pivot — they explain *why* fork-and-pause was chosen over in-place mutation, with the validation matrix from the old approach showing what we successfully proved end-to-end before deciding it was the wrong shape.
