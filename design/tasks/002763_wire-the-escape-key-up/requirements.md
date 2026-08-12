# Requirements: Escape Cancels the Current Turn in the Spec Task Chat

## Background

On the spec task detail page (`SpecTaskDetailPage` → `SpecTaskDetailContent` → `AgentChat` →
`RobustPromptInput`) the only way to stop a running agent turn is to click the red square
"Stop generation" button, which appears in the composer toolbar while the agent is busy.
Claude Code users expect `Escape` to do the same thing while the chat box is focused.

A second expectation carried over from Claude Code: after cancelling, any messages the user
already queued should start running. Investigation shows this does **not** happen on its own
today — see [Open Questions](#open-questions) and `design.md` §3.

## User Stories

### US-1: Cancel the current turn with Escape
**As a** user watching an agent turn I no longer want,
**I want** to press `Escape` while the chat box is focused,
**so that** the turn is cancelled without reaching for the mouse.

**Acceptance Criteria**
- [ ] With the composer focused and the agent busy (a red Stop button is visible), pressing
      `Escape` cancels the current turn — identical effect to clicking Stop.
- [ ] The button switches to its "Stopping generation…" spinner state while the cancellation
      is awaiting acknowledgement, exactly as it does for a click.
- [ ] `Escape` does not clear, submit, or otherwise modify the draft text or attachments.
- [ ] Holding or repeatedly hitting `Escape` sends at most one cancel request per turn; the
      in-flight guard (`isCancelling`) suppresses the rest.
- [ ] When the agent is **not** busy, `Escape` in the composer does nothing to the session —
      existing page-level `Escape` behaviour (closing the slide-in "New Task" panel) is
      unchanged.
- [ ] When the `@`/`$` sandbox-completion popup is open, `Escape` closes the popup and does
      **not** cancel the turn. A second `Escape` then cancels.
- [ ] Escape works for both composer variants: the plain textarea and the
      `SandboxPromptEditor` (`enableSandboxCompletions`).
- [ ] Cancel failures surface the same snackbar as the button path
      (`Failed to interrupt current turn`); a `noop` response still shows
      "The agent is no longer running a turn".

### US-2: Queued messages run after a cancel
**As a** user who queued follow-up messages while the agent was working,
**I want** the next queued message to be dispatched promptly once I cancel the current turn,
**so that** cancelling advances the queue instead of stalling it.

**Acceptance Criteria**
- [ ] After a successful cancel (Escape *or* the Stop button *or* `POST /sessions/{id}/cancel`
      from the CLI), the oldest pending queue-mode prompt for that session is dispatched
      without further user action.
- [ ] Dispatch happens within a couple of seconds, not on the next incidental
      `/prompt-history/sync` from the UI.
- [ ] If there are no pending prompts, cancelling leaves the session idle — no spurious
      dispatch, no error.
- [ ] Existing ordering guarantees hold: the drain goes through the normal
      `processPendingPromptsForSession` path, so the per-session drain lock, the
      thread-establishment barrier, and oldest-first ordering are all respected.
- [ ] Interrupt-mode pending prompts continue to be handled by the existing interrupt path.

### US-3: Discoverability
**As a** user,
**I want** the Stop control to tell me about the shortcut,
**so that** I learn it without reading docs.

**Acceptance Criteria**
- [ ] The Stop button tooltip reads `Stop generation (Esc)` in its idle state.
      (`Stopping generation…` is unchanged.)
- [ ] The `aria-label` stays `Stop generation` / `Stopping generation` so existing tests and
      screen-reader semantics are untouched.

## Non-Goals

- No global (window-level) Escape-to-cancel — only when the composer has focus.
- No double-Escape-to-clear-draft, no Escape-to-blur, no Escape-to-close-chat.
- No change to the cancel API contract or to Zed's `cancel_current_turn` handling.
- No change to the desktop viewer's own Escape handling (`DesktopStreamViewer` forwards
  Escape to the remote desktop; it is a different focus target).

## Open Questions

1. **Queue drain after cancel — confirmed missing.** `cancelActiveTurn`
   (`api/pkg/server/prompt_history_handlers.go:547`) and `handleTurnCancelled`
   (`api/pkg/server/websocket_external_agent_sync.go:2420`) both mark the interaction
   `Interrupted` but neither calls `processPromptQueue` / `nudgeSessionQueue` — unlike the
   normal `message_completed` path, which does. So a queued message currently waits for an
   unrelated nudge. The spec assumes we fix this **server-side** in the cancel path (one fix,
   benefits Escape, the Stop button and the CLI alike). Confirm you want the backend change
   rather than a frontend-only "call cancel then nudge" in `AgentChat`.
2. **Should Escape cancel in every `RobustPromptInput`, or only the spec task chat?** The
   handler lives in the shared component, so the shortcut will also apply wherever `onCancel`
   is passed — currently `AgentChat` (spec task detail, org chat, tabs view) and
   `ExternalAgentDesktopViewer`. Assumed desirable for consistency; say so if you want it
   gated to the spec task page only.
3. **Escape while a queued-message edit is open.** The queue-item edit textarea already binds
   Escape to "cancel edit" and is a separate focus target, so it keeps that meaning. Assumed
   correct.
