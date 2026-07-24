# Design: Add Retry Button to Errored Interactions in Spec Task Detail (ACP) View

## Where this renders

Spec task detail view render chain (all in `frontend/src/`):

```
components/tasks/SpecTaskDetailContent.tsx
  └─ components/session/EmbeddedSessionView.tsx   (owns handleRegenerate → NewInference)
       └─ components/session/Interaction.tsx      (renders user bubble + assistant bubble)
            ├─ components/session/InteractionInference.tsx     (error alert + Retry button)
            └─ components/session/InteractionLiveStream.tsx    (live ACP stream; no error UI)
```

- `EmbeddedSessionView.tsx:475` `handleRegenerate(interactionID, message)` →
  `NewInference({ message, sessionId, type })`. Already passed down as
  `onRegenerate` to every `Interaction` (`EmbeddedSessionView.tsx:713`).
- `Interaction.tsx:386` passes `error={interaction?.error}` and `onRegenerate`
  to **both** the user-bubble and assistant-bubble `InteractionInference`.
- `InteractionInference.tsx:539-585` renders the error `Alert` and the Retry
  button. Retry is gated on `onRegenerate && !message`.

## Root cause (leading hypothesis — confirm during implementation)

For an errored ACP interaction:

- The error `Alert` renders on the **user bubble**, where
  `message` = the user's prompt (truthy) → the `!message` guard suppresses the
  Retry button.
- The **assistant bubble** (`Interaction.tsx:440`) only renders when
  `assistantMessage || response_entries?.length > 0 || isLive`. When the agent
  errors before emitting anything, none of these hold, so the assistant-side
  `InteractionInference` (which would pass `!message`) never mounts.
- `InteractionLiveStream.tsx` has no error/retry branch at all.

Net effect: the alert shows, the Retry button does not — matching the report.

There is also a latent **double-render** risk: because `error` is passed to
both bubbles, an interaction that renders both bubbles could show the alert
twice. The fix must ensure the alert (and Retry) render exactly once.

## Approach

Keep the change small and localised to the shared `InteractionInference`
error block (fixing it there fixes every surface, not just the spec task view).

1. **Show the alert + Retry once, on the assistant side.**
   Render the `error` block only when `isFromAssistant` is true. This
   de-duplicates the alert and guarantees the block lives on the bubble whose
   `message` is the assistant response (empty on a clean failure).

2. **Guarantee the assistant bubble mounts on error.**
   In `Interaction.tsx:440`, extend the assistant-bubble render condition to
   also mount when `interaction.error` is set:
   `assistantMessage || response_entries?.length > 0 || isLive || interaction.error`.
   This makes the assistant-side `InteractionInference` (and thus the error +
   Retry) render even when the agent produced no output.

3. **Make the Retry button robust to partial output.**
   Change the Retry gate from `onRegenerate && !message` to simply
   `onRegenerate` (within the now assistant-only error block). A failed turn is
   always retryable regardless of whether partial text/tool-calls streamed
   first. Retry re-sends `interaction.prompt_message` via the existing
   `onRegenerate` handler.

4. **Verify the retry actually reaches the ACP agent.**
   Confirm `handleRegenerate` → `NewInference` re-runs the prompt against the
   live external-agent session. If it lands on the wrong agent (defaults to
   `helix_agent`), pass the session's `agent_type` explicitly from
   `EmbeddedSessionView`.

## Key decisions

- **Fix in the shared component, not a spec-task-only branch.** The error UI is
  already shared between OpenAI-style and ACP views; correcting the gating there
  is the minimal, DRY fix and avoids a divergent code path.
- **Assistant-side, single render.** Anchoring the error block to the assistant
  bubble both de-dupes it and aligns it with where a response would have been.
- **Reuse `NewInference`, no new endpoint.** `handleRegenerate` already exists
  and is wired for external-agent sessions; per-turn retry is a re-send, not a
  new backend concept. The heavier `restart-agent` flow stays separate.

## Implementation Notes

- **Confirmed retry routing.** `handleRegenerate` (`EmbeddedSessionView.tsx:475`)
  calls `NewInference({ type, message, sessionId })` with no `agentType` /
  `external_agent_config`. This is the *exact same* call the spec task detail
  prompt box makes on every normal send (`SpecTaskDetailContent.tsx:2007`), so
  the backend routes it to the existing session's external (ACP) agent. Open
  question #3 resolved: no explicit agent type needed.
- **`InteractionInference` default export has only one consumer** — `Interaction.tsx`,
  which mounts it twice (`isFromAssistant={false}` user bubble,
  `isFromAssistant={true}` assistant bubble). Other imports pull the named
  `MessageWithToolCalls` / `ResponseEntry` exports only. So gating the error
  block to `isFromAssistant` is safe and self-contained.
- **Three-line change, no backend edits:**
  1. `InteractionInference.tsx` — error block now `error && isFromAssistant`,
     Retry gate now just `onRegenerate` (was `onRegenerate && !message`).
  2. `Interaction.tsx` — assistant-bubble render condition gains
     `|| interaction.error` so it mounts even on a no-output failure.

## Testing

Per CLAUDE.md, test end-to-end in the inner Helix (`localhost:8080`):

1. Register/login, complete onboarding, create a spec task (provisions the ACP
   sandbox so Zed connects — a live external-agent session).
2. Induce a turn error (e.g. bad model/provider config, or a prompt that trips
   the agent) and confirm the error alert now shows a Retry button in the spec
   task detail view.
3. Click Retry; confirm the prompt re-runs and streams in place.
4. Regression: confirm OpenAI-style chat still shows Retry and that normal
   interactions render unchanged.
5. `cd frontend && yarn build` before committing.

### Verification status (this environment)

- **Build: PASSED.** `vite build` transformed all 21661 modules and built
  cleanly. (The default `frontend/dist` is a root-owned read-only bind mount
  here, so the final copy step hits `EACCES`; building to a writable `--outDir`
  completes with `✓ built`. The compile/transform is the meaningful check and it
  is green.)
- **E2E in inner Helix: NOT RUN.** The inner Helix stack is not running in this
  environment — `docker ps` shows zero containers and `localhost:8080` returns
  `000`. The three-line change is low-risk and type-checked, but the live
  click-through (trigger error → Retry appears → click → prompt re-runs) has NOT
  been exercised here and should be confirmed by a reviewer with a live stack.
