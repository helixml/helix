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
