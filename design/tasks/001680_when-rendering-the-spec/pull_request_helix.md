# Collapse system prompt prefix in spec task chat

## Summary

The first interaction of a spec task session has its `prompt_message` constructed by the backend as:

```
{planningPrompt}

**User Request:**
{userPrompt}
```

Previously the entire combined string was rendered as the user's chat bubble in the spec task details page, drowning out what the user actually wrote.

This change splits on the `**User Request:**` (or `**Original Request (for context only...):**` for cloned tasks) marker and shows the planning prefix in a small collapsed accordion above the bubble. The user's own text is rendered prominently as the chat bubble — what they primarily care about.

## Changes

- New component `frontend/src/components/session/CollapsibleSystemPrefix.tsx` — exports `splitSystemPrefix()` helper and a `<CollapsibleSystemPrefix>` UI component (visual style mirrors `CollapsibleToolCall.tsx`).
- `frontend/src/components/session/Interaction.tsx` — applies the split inside the existing `useMemo`, renders the collapsible prefix above the user bubble, passes only the user portion as the bubble's message.
- The collapsible is collapsed by default and expandable on click. Cloned-task variants get the label "Planning Instructions (cloned task)".
- When the user enters edit mode, the full original `prompt_message` is shown in the textarea so a regenerate doesn't lose data.
- Messages without the marker render exactly as before — no behavior change for non-spec sessions.
- 7 unit tests in `CollapsibleSystemPrefix.test.ts` covering edge cases (empty, no marker, marker-at-start, both variants, whitespace, multiline).

## Test plan

- [x] `yarn tsc` passes
- [x] `yarn test src/components/session/CollapsibleSystemPrefix.test.ts` — 7/7 passing
- [x] `vite build` transforms cleanly (21407 modules; failure to write `dist/` is a pre-existing permissions issue unrelated to this change)
- [ ] Manually verify in spec task details page: first interaction shows collapsed "Planning Instructions" above a clean user-message bubble; expanding reveals the full planning prompt; non-first interactions render unchanged

## Screenshots

Verified end-to-end in the inner Helix on a fresh spec task ("Add a CONTRIBUTING.md file...").

**Collapsed (default):** small "Planning Instructions" pill above a clean user-message bubble that shows only what the user actually wrote.

![Collapsed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001680_when-rendering-the-spec/screenshots/01-collapsed.png)

**Expanded:** clicking the pill reveals the full planning prompt; the user's request remains visible as the bubble below.

![Expanded](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001680_when-rendering-the-spec/screenshots/02-expanded.png)
