# Implementation Tasks

- [~] Fix retry button in `frontend/src/components/session/InteractionInference.tsx` (line ~385) to pass `interaction.prompt_message` instead of empty string
- [ ] Remove the `isExternalAgent` conditional that disables `onRegenerate` in `frontend/src/components/session/EmbeddedSessionView.tsx` (line ~390)
- [ ] Test retry button appears when external agent interaction has error state
- [ ] Test clicking retry resends the original prompt to Zed agent
- [ ] Verify retry works in both EmbeddedSessionView (floating desktop panel) and standalone session views