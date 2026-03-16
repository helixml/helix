# Design: Show User Prompt + Spec as Primary Content

## Current Behaviour

`SpecTaskDetailContent.tsx` renders a two-panel layout:
- **Left panel** (30% default): `EmbeddedSessionView` showing the full chat thread + input box
- **Right panel** (70% default): toggle between Desktop / File Diff / Details views

`EmbeddedSessionView` → `Interaction` → renders each `TypesInteraction`.
The first interaction's `display_message` (or `prompt_message`) contains the agent instructions prepended to the user's original message, making it look like a wall of technical instructions.

The spec documents (requirements.md, design.md, tasks.md) live in the "Details" view (right panel), but that panel defaults to "Desktop" view when an active session exists — so the spec is not shown by default.

## Proposed UX

### Layout Change: Add a "Spec" Tab to the Right Panel

Add a new **"Spec"** toggle button to the existing `ToggleButtonGroup` in the right panel header:

```
[ Desktop ] [ File Diff ] [ Spec ] [ Details ]
```

- **Default state when spec exists** (task has `design_docs_pushed_at` set): right panel defaults to `"spec"` view
- **Default state without spec** (backlog, queued): right panel defaults to `"details"` or `"desktop"` as before

### Spec Tab Content

When `currentView === "spec"`, render a scrollable markdown view with three collapsible sections:

```
┌─────────────────────────────────────────────────────┐
│  📋 Original Request                                │
│  ─────────────────────────────────────────────────  │
│  [The user's original prompt, large readable text]  │
│                                                     │
│  ▼ Requirements                         [collapse]  │
│  [Rendered requirements.md]                         │
│                                                     │
│  ▼ Design                               [collapse]  │
│  [Rendered design.md]                               │
│                                                     │
│  ▼ Tasks                                [collapse]  │
│  [Rendered tasks.md with checkbox progress]         │
└─────────────────────────────────────────────────────┘
```

All three spec sections start **expanded** (the content is valuable), but each has a collapse toggle.

### Agent Instructions Collapse in Chat Thread

In `EmbeddedSessionView` (or `Interaction.tsx`), for the **first interaction only**, detect if `prompt_message` is very long (>500 chars) or if `interaction.system_prompt` is set. If so:

- Render a collapsed `<Accordion>` or similar for the agent instructions portion
- Label it: `"Agent Instructions"` with a `ExpandMore` icon
- Show only the **user's actual message** (the part after the agent instructions) by default

**Implementation approach**: The backend stores the agent instructions separately in `interaction.system_prompt` on `TypesInteraction` (note: despite the field name, this is technically the first user message sent to the LLM, not a true system prompt — it just contains agent instructions that read like one). The `prompt_message` field on the first interaction currently contains the combined `[agent instructions + user_message]`. We can use `interaction.system_prompt` to identify and collapse the instructions portion.

- If `interaction.system_prompt` is present and non-empty: render it inside a `<Accordion defaultExpanded={false}>` above or below the visible user message
- The visible user portion = `interaction.prompt_message` with the `interaction.system_prompt` prefix stripped (or just show `interaction.display_message` if it's cleaner)

**Note**: Investigate whether `display_message` already strips the agent instructions — if so, displaying `display_message` instead of `prompt_message` in `EmbeddedSessionView` may be the simpler fix.

## Data Flow

### Spec Documents

```
useSpecTaskDocuments(taskId)
  → v1SpecTasksDocumentsDetail(taskId, "requirements")
  → v1SpecTasksDocumentsDetail(taskId, "design")
  → v1SpecTasksDocumentsDetail(taskId, "tasks")
```

These three requests can be made in parallel. Display results as rendered markdown (the existing `Markdown` component is already used elsewhere in the codebase).

**Availability check**: Only show the Spec tab and default to it when `task.design_docs_pushed_at` is set (i.e., the spec agent has pushed the docs).

### Original Prompt

Already available on the `TypesSpecTask` object:
- `task.original_prompt` — the raw user request
- `task.description` — edited description (use this if set, fallback to `original_prompt`)

No additional API calls needed for the prompt.

## Component Changes

| Component | Change |
|---|---|
| `SpecTaskDetailContent.tsx` | Add `"spec"` to the `currentView` type and `ToggleButtonGroup`. Add `renderSpecContent()` function. Change default view logic: if `task.design_docs_pushed_at`, default to `"spec"`. |
| `EmbeddedSessionView.tsx` | Pass `isFirstInteraction` prop to `Interaction`. |
| `Interaction.tsx` | When `isFirstInteraction && interaction.system_prompt`, render agent instructions inside a collapsed `<Accordion>`. Show user message without the system prefix. |
| `specTaskService.ts` | Add `useSpecTaskDocuments(taskId)` hook that fetches requirements, design, and tasks docs in parallel. |

## Key Decisions

**Decision: New "Spec" tab vs. enhancing "Details" tab**
Chose a new tab because the Details tab already has its own purpose (priority, dependencies, metadata). A dedicated Spec tab makes it clear what the content is and can have its own behaviour (defaulting on when spec exists).

**Decision: Collapse agent instructions in chat vs. strip it**
Chose collapse (not strip) so users can still inspect the full agent instructions when curious. Stripping would hide information permanently.

**Decision: Show original prompt in Spec tab (not as a chat bubble)**
The prompt is displayed in the Spec tab as a styled "context" block at the top, not just as a chat bubble. This gives it visual prominence and separates "what you asked" from "the conversation history."

**Decision: Default to Spec tab only when design_docs_pushed_at is set**
Tasks in backlog or queued states don't have a spec yet. Defaulting to the Spec tab only when docs exist avoids showing an empty/loading panel.

## Codebase Patterns to Follow

- Use `TypesSpecTaskStatus` enum values for conditional logic (already imported in `SpecTaskDetailContent.tsx`)
- Use the existing `Markdown` component for rendering spec docs (found in `InteractionInference.tsx`)
- Use MUI `Accordion`/`AccordionSummary`/`AccordionDetails` for the agent instructions collapse (consistent with `TestsEditor.tsx` pattern)
- Fetch spec docs with `useQuery` + `api.getApiClient().v1SpecTasksDocumentsDetail()` (follow patterns in `specTaskService.ts`)
- The right panel `currentView` state is persisted to URL via `router.mergeParams({ view: newView })` — the new `"spec"` value should work automatically
