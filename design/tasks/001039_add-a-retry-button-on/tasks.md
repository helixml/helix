# Implementation Tasks

- [ ] Remove the `isExternalAgent` conditional that disables `onRegenerate` in `frontend/src/components/session/EmbeddedSessionView.tsx`
- [ ] Test retry button appears when external agent interaction has error state
- [ ] Test clicking retry resends the original prompt to Zed agent
- [ ] Verify retry works in both EmbeddedSessionView (floating desktop panel) and standalone session views