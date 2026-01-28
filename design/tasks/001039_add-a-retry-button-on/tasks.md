# Implementation Tasks

- [x] Fix retry button in `frontend/src/components/session/InteractionInference.tsx` (line ~385) to pass `interaction.prompt_message` instead of empty string
- [x] Remove the `isExternalAgent` conditional that disables `onRegenerate` in `frontend/src/components/session/EmbeddedSessionView.tsx` (line ~390)
- [x] Remove unused `AGENT_TYPE_ZED_EXTERNAL` import from EmbeddedSessionView.tsx
- [ ] Test retry button appears when external agent interaction has error state (manual test)
- [ ] Test clicking retry resends the original prompt to Zed agent (manual test)
- [ ] Verify retry works in both EmbeddedSessionView (floating desktop panel) and standalone session views (manual test)

## Notes

Code changes complete and pushed to `feature/001039-add-a-retry-button-on`. Testing tasks require manual UI verification.