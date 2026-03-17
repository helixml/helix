# Design

## Current Layout (Desktop, active session)

```
┌──────────────────┬──────────────────────────────────┐
│ Chat (30%)       │ Desktop / Changes / Details (70%) │
│ ─────────────    │                                   │
│ [System prompt]  │  <desktop stream or diff view>    │
│ [huge wall]      │                                   │
│ [Agent reply]    │                                   │
│ ...              │                                   │
│ [Input box]      │                                   │
└──────────────────┴──────────────────────────────────┘
```

## Proposed Layout (Desktop, active session)

```
┌─────────────────────────────┬──────────────────────────────┐
│ Task Brief (55%)            │  Right Tabs (45%)            │
│ ───────────────             │  Chat | Desktop | Changes |  │
│ ┌─ Original Prompt ──────┐  │  Details                     │
│ │ "What the user typed"  │  │                              │
│ └────────────────────────┘  │  <selected tab content>      │
│                             │                              │
│ Spec Tabs:                  │                              │
│ [Requirements][Design][Plan]│                              │
│ ─────────────────────────── │                              │
│ <markdown rendered spec>    │                              │
└─────────────────────────────┴──────────────────────────────┘
```

## Key Decisions

### 1. Swap panel roles

The left panel changes from "Chat" → "Task Brief + Spec". Chat moves to the right panel as a tab. This is the most impactful change — the spec becomes the first thing you see.

**Rationale**: The spec represents the human-AI agreement on what to build. It's far more valuable to show at a glance than the raw LLM conversation.

### 2. New left panel: `TaskSpecPanel`

A new component (or inline section in `SpecTaskDetailContent`) that renders:
- **Original prompt** — `task.original_prompt` styled as a quoted request block
- **Spec tabs** — if `task.requirements_spec`, `task.technical_design`, or `task.implementation_plan` are populated:
  - Tab: Requirements → renders `task.requirements_spec` as markdown
  - Tab: Design → renders `task.technical_design` as markdown
  - Tab: Tasks → renders `task.implementation_plan` as markdown
- **If no spec yet** — show the prompt + a small status chip ("Spec not yet generated")

This reuses the markdown rendering already in `DesignReviewContent.tsx` but without the comment/review overlay.

### 3. Right panel: Chat moves here

Right panel has a `ToggleButtonGroup` with: **Chat | Desktop | Changes | Details**.
- Chat tab → `EmbeddedSessionView` + prompt input (same as current left panel)
- Desktop tab → `ExternalAgentDesktopViewer` (unchanged)
- Changes tab → `DiffViewer` (unchanged)
- Details tab → current `renderDetailsContent()` (agent, timestamps, debug, etc.)

Default tab when session starts: **Chat** (so the user can follow agent activity).

### 4. Collapse system prompt in chat

The first interaction in a session (`interactions[0]`) always contains the full system prompt as `prompt_message`. This is a very long string and visually dominates the chat.

**Fix**: In `EmbeddedSessionView`, pass `isFirstInteraction={index === 0}` to each `<Interaction>`. In `Interaction`, when `isFirstInteraction` is true and the message length exceeds ~500 chars, render a collapsed placeholder instead of the full message bubble:

```
┌──────────────────────────────────────────┐
│ 🔒 System prompt  [Show ▼]               │
└──────────────────────────────────────────┘
```

Clicking "Show" expands to the full user message bubble. This is a `useState(false)` toggle — simple, no persistence needed.

### 5. No-session / mobile fallback

Unchanged from current behaviour — single panel with tab toggles. The "Details" tab still renders `renderDetailsContent()`. The left spec panel only appears in desktop split-screen mode.

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Restructure desktop layout: left=spec, right=tabs with chat |
| `frontend/src/components/session/EmbeddedSessionView.tsx` | Pass `isFirstInteraction` prop to `<Interaction>` |
| `frontend/src/components/session/Interaction.tsx` | Accept `isFirstInteraction`; collapse if long |
| `frontend/src/components/tasks/TaskSpecPanel.tsx` | New component: prompt + spec tabs |

## Codebase Patterns Found

- `TypesSpecTask` has `original_prompt`, `requirements_spec`, `technical_design`, `implementation_plan` fields — all already fetched via `useSpecTask(taskId)`.
- `DesignReviewContent.tsx` already renders spec docs as markdown tabs — can borrow its markdown rendering approach (ReactMarkdown + remarkGfm).
- The resizable `Panel` / `PanelGroup` / `PanelResizeHandle` from `react-resizable-panels` is already used in `SpecTaskDetailContent.tsx` — keep the same pattern.
- The `ToggleButtonGroup` pattern for view tabs is already used in the right panel — extend it to include "Chat" and move the prompt input alongside it.
- The component is already large (~2500 lines). Extract `TaskSpecPanel` as a separate file rather than adding more inline JSX.
