# Implementation Tasks: Add Retry Button to Errored Interactions in Spec Task Detail (ACP) View

- [ ] Reproduce the bug: create a spec task in the inner Helix, induce a turn error, and confirm the error alert shows with NO Retry button in the detail view.
- [~] In `InteractionInference.tsx` (error block ~L539-585): render the error alert + Retry only when `isFromAssistant` (de-duplicate across bubbles).
- [ ] In `InteractionInference.tsx`: change the Retry gate from `onRegenerate && !message` to `onRegenerate` so retry shows even when partial output streamed.
- [ ] In `Interaction.tsx` (~L440): extend the assistant-bubble render condition to also mount when `interaction.error` is set, so the error block renders on clean (no-output) failures.
- [ ] Confirm `EmbeddedSessionView.handleRegenerate` → `NewInference` re-sends the prompt to the live external-agent session; if it lands on the wrong agent, pass the session's `agent_type` explicitly.
- [ ] Verify the "click here to view the details" error terminal still opens correctly.
- [ ] Regression check: OpenAI-style chat retry unchanged; normal in-progress/completed interactions render unchanged.
- [ ] Test end-to-end in the inner Helix: trigger error → Retry appears → click Retry → prompt re-runs and streams in place.
- [ ] `cd frontend && yarn build` and confirm it passes before committing.
