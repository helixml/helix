# Design: Chat/Spec Tab Toggle on Issue View

## Current Architecture

```
SpecTaskDetailContent.tsx  (93 KB)
├── Left Panel  (30% default, react-resizable-panels)
│   ├── Thread selector dropdown
│   └── EmbeddedSessionView  (chat)
└── Right Panel (70% default)
    ├── Task details + metadata
    ├── SpecTaskActionButtons  → "Review Spec" button → handleReviewSpec()
    └── File diffs / execution history
```

`handleReviewSpec()` (line ~774 in SpecTaskDetailContent.tsx):
- Fetches the latest design review via `v1SpecTasksDesignReviewsDetail()`
- If `onOpenReview` callback exists → opens a new **Review** tab in `TabsView` (workspace mode)
- Otherwise → navigates to the standalone `SpecTaskReviewPage` (`project-task-review` route)

`DesignReviewContent.tsx` (46 KB) renders the review with three internal tabs:
- Requirements Specification
- Technical Design
- Implementation Plan

## Proposed Changes

### 1. Add Chat/Spec tabs to the left panel (`SpecTaskDetailContent.tsx`)

Add a small tab strip at the top of the left panel with two options: **Chat** and **Spec**.

```
Left Panel
├── [Chat] [Spec]   ← new tab strip
└── <content based on selected tab>
    - Chat tab: EmbeddedSessionView  (unchanged)
    - Spec tab: triggers handleReviewSpec() (same as today's button)
```

State: add `leftTab: 'chat' | 'spec'` to component state.

**Default tab logic:**
```ts
const defaultTab = (task.phase === 'planning' || task.phase === 'review') && hasDesignReview
  ? 'spec'
  : 'chat'
```

Only render the Spec tab when `hasDesignReview` is true (same guard used by the Review Spec button today).

Clicking **Spec** calls the existing `handleReviewSpec()` — no new navigation logic needed. The tab simply acts as a shortcut to the already-working flow.

### 2. Add "Back to issue" affordance in the spec review view

There are two contexts where the review renders:

**a) Workspace / TabsView mode** (`TabsView.tsx`)
The review opens as a new tab next to the task tab. The user can simply click back to the task tab. No code change needed — the tab bar already provides navigation.

**b) Standalone page** (`SpecTaskReviewPage.tsx`)
Add a **"← Back"** / **"Close"** button in the page header that calls `router.back()` (or navigates to the task detail route). This is the case where the user is most "stranded".

Also check if `DesignReviewContent.tsx` renders its own close/back button — if a prop `onClose` / `onBack` already exists (or can be added), call it from `SpecTaskReviewPage`.

### 3. Keep "Review Spec" button

The existing button in `SpecTaskActionButtons.tsx` is kept as-is. It is now a secondary entry point (for users who miss the tab). No change required.

## Key Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Add `leftTab` state, tab strip UI, default tab logic |
| `frontend/src/pages/SpecTaskReviewPage.tsx` | Add back/close button |
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | Accept optional `onBack` prop; render back button if provided |

## Decisions

- **Spec tab calls `handleReviewSpec()`** rather than rendering inline. This reuses the existing, tested flow and avoids embedding a third complex component into an already large file. Inline rendering could be a follow-up.
- **Default to Spec only for `planning`/`review` phases** — implementation-phase tasks should not surprise the user with a spec view.
- **Workspace mode (TabsView) already handles back navigation** via the tab bar, so the fix is only needed for standalone page mode.
