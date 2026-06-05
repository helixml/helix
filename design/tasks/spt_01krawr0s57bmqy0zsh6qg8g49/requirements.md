# Requirements: Fix Mobile Overflow on Spec Task Chat & Padding on Mobile Search Bar

## Background

Two visual issues exist in the mobile view of the spec task pages:

1. **Spec task detail page, chat view (mobile)** — the `RobustPromptInput` component (the prompt input box) and the queued-messages display that appears directly above it visually overflow the right-hand side of the viewport. The container does not horizontally scroll, so the overflowing pixels are clipped/inaccessible.
2. **Spec tasks list page (mobile)** — the search input that appears above the kanban board on mobile is jammed against the top of the visible area and has cramped horizontal padding, giving it a cropped, ugly look compared to the rest of the page.

Both issues live across three files:

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — mobile chat view (`currentView === "chat"`) at lines ~2664–2763, which wraps `RobustPromptInput`.
- `frontend/src/components/common/RobustPromptInput.tsx` — the prompt-input component itself (lines ~1142–1622). The queue is rendered at lines 1147–1234 (inside a `Collapse`) and sits as a sibling above the bordered input container at lines 1356–1622.
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — mobile-only search bar at lines 1652–1693.

## User Stories

### Story 1: Mobile chat input and queue fit within the viewport

> As a user on a mobile-sized viewport viewing the spec task detail "Chat" tab, I want both the prompt input box and the queued-messages display that sits above it to fit entirely within the screen width, so I can see the full input area, the queue header, and all queue-item controls (drag handle, edit, delete, restart) without horizontal clipping.

**Acceptance criteria**

- AC1.1 — On viewports ≤ `md` breakpoint (≤ 899.95px wide), the `RobustPromptInput` container — including the queued-messages panel above the input, the bordered input box, and the action-buttons row inside it — never visually overflows the right edge of the parent chat panel.
- AC1.2 — When the queue contains items, the queue header (icon + "Message queue (saved locally)" / "Editing - paused from here" / etc. text + count chip) and every queue-item row (drag handle, status icon, truncated message preview, action icons) stay fully inside the parent width.
- AC1.3 — The input textarea expands to the full available width minus the parent padding and is never clipped by the right edge of the screen.
- AC1.4 — No new horizontal scrollbar appears on the mobile chat view as a result of the fix; the chat panel remains `overflow: hidden` horizontally.
- AC1.5 — The desktop (≥ `md`) layout is visually unchanged. The same `RobustPromptInput` is also used in the split-view layout (lines ~1938–1964); that layout must continue to render unchanged on desktop.

### Story 2: Mobile search bar has appropriate top and side spacing

> As a user on a mobile-sized viewport viewing the spec tasks list page, I want the search input to have visible breathing room above and around it, so it looks intentional and consistent with the rest of the page rather than crashed into the top edge.

**Acceptance criteria**

- AC2.1 — On viewports < `md`, the mobile search bar inside `SpecTaskKanbanBoard` has visible top padding so it is not flush against the top edge of its container.
- AC2.2 — The horizontal padding on the mobile search bar is generous enough that the input is not flush against the screen edges and feels visually consistent with mobile content padding elsewhere on the page.
- AC2.3 — Bottom padding remains sufficient to separate the search bar from the kanban board content below it (current `pb: 1` is fine; do not regress).
- AC2.4 — Desktop layout is unchanged. The desktop header containing the search input (lines 1479–1650, hidden on mobile) is not affected.

## Out of scope

- No restyling of the search input itself (size, icon, placeholder text) — only its outer container's padding/margin.
- No behavior changes to `RobustPromptInput` (queue logic, history, send semantics, attachments, retry handling) — only layout/overflow fixes.
- No changes to the keyboard-shortcut hint row that sits *below* the input box — the user's report is about the queue display *above* the input, not the hint row below.
- No changes to the desktop split-view chat panel beyond what is required to also benefit from a `min-width: 0` flex fix if applicable (see design.md).
- No new responsive breakpoints introduced. Use the existing `xs`/`md` breakpoints already in use in these files.
