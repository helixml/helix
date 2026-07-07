# Requirements: Add Retry Button to Errored Interactions in Spec Task Detail (ACP) View

## Background

When an agent turn fails, the chat shows the red alert:

> "The system has encountered an error — click here to view the details."

In the classic OpenAI-style chat this alert is accompanied by a **Retry**
button. In the **spec task detail view** — which renders external-agent (ACP /
Zed) sessions via `EmbeddedSessionView` — the same error alert appears but the
Retry button is frequently **missing**, leaving the user with a dead-end error
and no way to re-run the failed prompt short of restarting the whole session.

The error UI lives in `frontend/src/components/session/InteractionInference.tsx`.
The Retry button is gated on `onRegenerate && !message`. For an errored
interaction the alert renders on the **user bubble** (where `message` is the
user's prompt and therefore truthy), so the button is suppressed; the assistant
bubble that would satisfy `!message` often isn't rendered at all when the agent
errored before producing any output. The live-stream path
(`InteractionLiveStream.tsx`) renders no error/retry UI whatsoever.

## User Stories

### US1 — Retry a failed agent turn from the spec task detail view
As a user viewing a spec task, when an agent turn errors, I want a Retry button
next to the error message so I can re-run the failed prompt without restarting
the session or losing context.

**Acceptance criteria**
- [ ] When an interaction in the spec task detail (ACP) view has an error, the
  error alert is shown **with** a visible Retry button.
- [ ] This holds regardless of whether the agent produced partial output before
  failing (partial text / tool calls) or failed before producing any output.
- [ ] Clicking Retry re-sends the failed prompt to the external agent and the
  turn begins streaming again in place.
- [ ] The error alert is not duplicated (it must not render once on the user
  bubble and again on the assistant bubble).

### US2 — Consistent behaviour with OpenAI-style chat
As a user, I want the retry experience in the spec task detail view to match the
existing OpenAI-style chat so the product feels consistent.

**Acceptance criteria**
- [ ] The Retry button uses the same label, icon (`ReplayIcon`), and placement
  pattern already used in `InteractionInference.tsx`.
- [ ] The "click here to view the details" error terminal still works
  unchanged.

### US3 — No regression to the existing OpenAI-style retry
As a user of the classic chat, I want retry to keep working exactly as it does
today.

**Acceptance criteria**
- [ ] The OpenAI-style chat error + Retry behaviour is unchanged.
- [ ] Non-error, in-progress, and completed interactions render unchanged.

## Out of Scope
- Automatic/silent retries (this is a manual, user-initiated button only).
- The existing session-level "Restart agent" flow (`restart-agent` endpoint) —
  that stays as-is; this task adds a lighter per-turn retry.
- Any backend changes, unless investigation shows the frontend `NewInference`
  retry path does not correctly reach the external ACP agent.

## Open Questions
1. **Retry mechanism**: Per-turn retry should go through the existing
   `handleRegenerate` → `NewInference` path (re-sends the prompt to the live
   session). Is that acceptable, or should Retry instead offer the heavier
   session `restart-agent` when the agent/desktop has crashed (not just the
   turn)? Assumption: use `NewInference` for the turn-level Retry.
2. **Errors during streaming**: Should the Retry also be surfaced while the
   interaction is still in the `waiting`/live state (i.e. add error handling to
   `InteractionLiveStream`), or is it sufficient to show it once the interaction
   has settled into an error state? Assumption: post-settle is sufficient for
   this task.
3. **Agent context on retry**: `handleRegenerate` currently calls `NewInference`
   without an explicit `agentType`/`external_agent_config` (relying on the
   backend to use the existing session's agent). Should we pass the session's
   agent type explicitly to be safe? Assumption: rely on existing session
   routing unless testing shows the retry lands on the wrong agent.
