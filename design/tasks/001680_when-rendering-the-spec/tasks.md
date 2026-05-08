# Implementation Tasks

- [x] In `Interaction.tsx`, add a `splitSystemPrefix(message)` helper that splits on `**User Request:**` or `**Original Request (for context only...):**` and returns `{ prefix, userText }`
- [x] In `Interaction.tsx`, apply the split to `userMessage` before rendering the user bubble
- [x] Render the prefix (if present) as a collapsed section (MUI Accordion or `<details>`) labeled "Planning Instructions", collapsed by default
- [x] Render the `userText` as the primary user message content (existing markdown rendering)
- [x] Add unit tests for `splitSystemPrefix` (7 cases passing)
- [~] Verify rendering in the spec task details page: user message shows their request; prefix is collapsible
- [x] Verify messages without the marker render unchanged (covered by unit test)
