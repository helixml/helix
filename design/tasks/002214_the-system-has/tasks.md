# Implementation Tasks: Add Retry Button to Errored Interactions in Spec Task Detail (ACP) View

- [x] Root cause identified by code inspection (see design.md): error alert renders on the user bubble where `!message` fails → Retry suppressed; assistant bubble not mounted on no-output failures; live-stream path has no error UI.
- [x] In `InteractionInference.tsx` (error block ~L539-585): render the error alert + Retry only when `isFromAssistant` (de-duplicate across bubbles).
- [x] In `InteractionInference.tsx`: change the Retry gate from `onRegenerate && !message` to `onRegenerate` so retry shows even when partial output streamed.
- [x] In `Interaction.tsx` (~L440): extend the assistant-bubble render condition to also mount when `interaction.error` is set, so the error block renders on clean (no-output) failures.
- [x] Confirm `EmbeddedSessionView.handleRegenerate` → `NewInference` re-sends the prompt to the live external-agent session. VERIFIED by code inspection: the retry uses the exact same `NewInference({ type, message, sessionId })` signature as the spec task detail prompt input (`SpecTaskDetailContent.tsx:2007`), so it routes to the existing session's ACP agent. No explicit `agent_type` needed.
- [x] Verify the "click here to view the details" error terminal still opens correctly — unchanged; `setViewingError`/`TerminalWindow` live in the same assistant-side `InteractionInference`, so it still works.
- [x] Regression check (code inspection): the default `InteractionInference` export has a single consumer (`Interaction.tsx`), so gating the error block to `isFromAssistant` affects only user-vs-assistant placement; OpenAI-style retry and normal interactions render unchanged.
- [~] Test end-to-end in the inner Helix: trigger error → Retry appears → click Retry. NOT RUN: the inner Helix stack is not up in this environment (0 containers, `localhost:8080` returns 000). See note below.
- [x] `yarn build` passes — all 21661 modules transformed and built cleanly (verified with a writable outDir; the default `dist` is a root-owned read-only bind mount in this env).
