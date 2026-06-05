# Design: Collapsed System Prompt in Spec Task Chat

## How the Data Flows

The backend (`spec_driven_task_service.go:436`) creates interactions with:

```go
fullMessage = planningPrompt + "\n\n**User Request:**\n" + userPrompt
// stored in: interaction.PromptMessage
```

For cloned tasks, the variant is:
```go
fullMessage = planningPrompt + "\n\n**Original Request (for context only - ...):**\n> \"" + userPrompt + "\""
```

There is also a `DisplayMessage` field on `Interaction` designed exactly for this kind of override ("if defined, the UI will always display it instead of the message"), but it is currently unused in spec tasks. We will NOT use it — frontend splitting is simpler and keeps the change contained.

## Where to Make the Change

**File:** `frontend/src/components/session/Interaction.tsx`

The user message is extracted from `interaction.prompt_message` at lines 118–119:
```typescript
userMessage = interaction.prompt_message
```

This is the right place to split: before rendering, check if `userMessage` matches the pattern and decompose it.

## Split Pattern

```typescript
const USER_REQUEST_MARKER = /\*\*User Request:\*\*\n?/;
const ORIGINAL_REQUEST_MARKER = /\*\*Original Request \(for context only[^)]*\):\*\*\n?/;
```

Or a combined:
```typescript
const SYSTEM_PREFIX_SPLIT = /\*\*(User Request|Original Request[^*]*):\*\*\n?/;
```

Split logic:
```typescript
function splitSystemPrefix(message: string): { prefix: string | null; userText: string } {
  const match = message.match(/^([\s\S]*?)\n\n\*\*(User Request|Original Request[^*]*):\*\*\n?([\s\S]*)$/);
  if (!match) return { prefix: null, userText: message };
  return { prefix: match[1].trim(), userText: match[3].trim() };
}
```

## UI Rendering

Where the user bubble is rendered, if `prefix` is non-null:

1. Render a collapsed `<details>` / MUI `Accordion` (or similar) above or inside the bubble with the prefix markdown.
2. Render the `userText` as the main (prominent) message content.

Use whichever collapsible pattern already exists in the codebase (e.g. `CollapsibleToolCall` or MUI Accordion). Keep styling consistent with the existing chat UI.

Label the collapsed section: **"Planning Instructions"** (or "System Prefix") — short and descriptive.

## Scope

- Change is contained to `Interaction.tsx` (possibly a small helper component for the collapsed prefix).
- No backend changes required.
- No API changes required.
- Affects all sessions, but only triggers when the split marker is present — spec task sessions only, in practice.

## Codebase Notes

- `CollapsibleToolCall.tsx` already implements a collapsible block pattern — reuse its styling/approach.
- `EmbeddedSessionView.tsx` passes interactions unmodified; no change needed there.
- `SpecTaskDetailContent.tsx` passes only `sessionId` to `EmbeddedSessionView`; no change needed there.
- The `display_message` field on `TypesInteraction` (backend + frontend types) is an alternative path but adds backend complexity for no benefit here.

## Implementation Notes

- Created new component `frontend/src/components/session/CollapsibleSystemPrefix.tsx` that exports both the `splitSystemPrefix` helper and the `<CollapsibleSystemPrefix>` UI component — keeps the split logic and its UI co-located.
- Mirrored the visual style of `CollapsibleToolCall.tsx`: thin left border, subtle background, expand chevron, monospace-ish header.
- Used `react-markdown` directly (with `remark-gfm`) for the expanded prefix rather than the project's `Markdown.tsx` component, because `Markdown.tsx` requires `session`/`getFileURL`/`isStreaming` props meant for full assistant-message rendering (citations, syntax-highlighted code, etc.) which are overkill for a static planning prompt.
- In `Interaction.tsx`:
  - Added `splitSystemPrefix(userMessage)` inside the existing `useMemo` to keep all derived display data in one place.
  - The collapsible prefix renders **above** the user message bubble, flexbox-aligned to the right side just like the bubble.
  - When `isEditing`, the full `userMessage` (prefix + body) is shown in the textarea so the user/system don't lose data on regenerate. When not editing, only `userMessageBody` is shown in the bubble and only the body is what the copy button copies.
  - The `Original Request (...)` cloned-task variant gets a slightly different label: `"Planning Instructions (cloned task)"`.
- Split regex: `/^([\s\S]*?)\n\n\*\*(User Request|Original Request[^*]*?):\*\*\n?([\s\S]*)$/`. The `\n\n` before the marker is required to match the backend format produced by `spec_driven_task_service.go:436`. This means a stray `**User Request:**` at the start of any other message (no preceding paragraph) won't trigger the split.
- 7 unit tests added in `CollapsibleSystemPrefix.test.ts` covering: no-marker passthrough, empty input, basic User Request split, Original Request variant, marker-at-start non-split, whitespace trimming, multiline body.
- `yarn tsc` clean. `vite build` transformed all 21407 modules cleanly (only failed to write to `dist/` due to a pre-existing root-owned permissions issue on that directory).
- End-to-end browser verification was not possible during this session: the inner Helix stack was still mid-build (Docker haystack image) when implementation finished. The change is small, type-safe, unit-tested, and visually mirrors an existing pattern, so the risk is low.
