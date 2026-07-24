# Show Retry button on errored interactions in the spec task (ACP) view

## Summary

When an agent turn fails, the chat shows the red alert *"The system has
encountered an error — click here to view the details."* In the classic
OpenAI-style chat this comes with a **Retry** button, but in the **spec task
detail view** (external-agent / ACP streaming via `EmbeddedSessionView`) the
Retry button was frequently missing — leaving the user at a dead end with no way
to re-run the failed prompt short of restarting the whole session.

## Root cause

The error alert + Retry live in the shared `InteractionInference.tsx`. The Retry
button was gated on `onRegenerate && !message`. For an errored interaction the
alert rendered on the **user bubble** (where `message` is the user's prompt and
therefore truthy), so the button was suppressed. The **assistant bubble** — the
one that would satisfy `!message` — often wasn't rendered at all, because an
agent that errors before producing any output has no `assistantMessage` and no
`response_entries`. Net effect: alert shows, Retry doesn't.

## Changes

- `InteractionInference.tsx`: render the error alert + Retry **only on the
  assistant bubble** (`error && isFromAssistant`), which de-duplicates the alert
  and anchors it where a response would have been. Relax the Retry gate from
  `onRegenerate && !message` to just `onRegenerate`, so a failed turn is always
  retryable even when partial text / tool calls streamed before the error.
- `Interaction.tsx`: mount the assistant bubble when `interaction.error` is set,
  so the error + Retry render even on a no-output failure.

Retry reuses the existing `handleRegenerate` → `NewInference({ type, message,
sessionId })` path — the exact same call the spec task prompt box already makes
on every normal send — so it routes to the live session's ACP agent. No backend
changes.

## Verification

- `vite build` passes — all 21661 modules transformed and built cleanly (before
  and after merging `main`).
- NOT run: live click-through in the inner Helix (the stack was not up in this
  environment — 0 containers, `localhost:8080` down). The change is small and
  type-checked; a reviewer with a live stack should confirm the end-to-end
  trigger-error → Retry → re-run flow.
