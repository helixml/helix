# Implementation Tasks

- [ ] In `Interaction.tsx`, add a `splitSystemPrefix(message)` helper that splits on `**User Request:**` or `**Original Request (for context only...):**` and returns `{ prefix, userText }`
- [ ] In `Interaction.tsx`, apply the split to `userMessage` before rendering the user bubble
- [ ] Render the prefix (if present) as a collapsed section (MUI Accordion or `<details>`) labeled "Planning Instructions", collapsed by default
- [ ] Render the `userText` as the primary user message content (existing markdown rendering)
- [ ] Verify rendering in the spec task details page: user message shows their request; prefix is collapsible
- [ ] Verify messages without the marker render unchanged
