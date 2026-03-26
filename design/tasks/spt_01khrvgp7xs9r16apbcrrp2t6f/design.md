# Design: Archive All Merged Tasks Button

## Architecture

All changes are confined to `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` plus a new animation component.

### Key files
| File | Change |
|------|--------|
| `SpecTaskKanbanBoard.tsx` | Add button to `KanbanColumn` header for `completed` column; add bulk archive handler; wire up confirmation dialog and animation |
| New: `TaDaAnimation.tsx` | Full-screen confetti/sparkle overlay component |

## Column Header Button

The `KanbanColumn` component already has a pattern for column-specific header actions: the backlog auto-start button (lines 462–557) renders only when `column.id === "backlog"` and a callback prop is provided.

Follow the same pattern for the Merged column:
- Add an optional `onArchiveAllMerged?: () => void` prop to `KanbanColumn`.
- Render a small `Tooltip`-wrapped icon button (e.g. MUI `CelebrationIcon` or `ArchiveIcon`) in the header's right-side `Box` when `column.id === "completed"` and `column.tasks.length > 0`.
- Pass the handler from `SpecTaskKanbanBoard` where `KanbanColumn` is rendered.

## Confirmation Dialog

Create a small inline confirmation (not reusing `ArchiveConfirmDialog` which is single-task focused). Use MUI `Dialog` with:
- Title: "Archive all merged tasks?"
- Body: "This will archive N completed tasks. They can be restored from the archive."
- Actions: Cancel / "Archive All" (primary, color warning)

State: `archiveAllConfirmOpen: boolean` added to `SpecTaskKanbanBoard`.

## Bulk Archive Handler

```
handleArchiveAllMerged():
  1. Get merged tasks = tasks.filter(t => t.phase === "completed" || t.status === "done")
  2. Open confirmation dialog

onConfirmArchiveAll():
  1. Close dialog
  2. Trigger TaDa animation
  3. Call v1SpecTasksArchivePartialUpdate(id, { archived: true }) for each task (Promise.all)
  4. Refresh task list (same as performArchive)
```

## Ta-Da Animation (`TaDaAnimation.tsx`)

A full-screen fixed overlay (`z-index: 9999`, pointer-events: none) that renders for ~2.5 seconds then unmounts.

**Implementation**: Use CSS `@keyframes` + MUI `keyframes` (already used in the project — see `pulseRing`, `spin` etc. in `TaskCard.tsx`). No external confetti library needed.

Approach:
- Scatter ~40–60 colored `Box` elements at random positions across the viewport.
- Each animates: fade-in + scale-up + fall/float with randomized delays (0–1s), durations (1–2s), and colors.
- A large "🎉" or "✨" emoji briefly flashes in the center.
- After 2.5s the parent sets `showAnimation = false` to unmount.

Controlled via a `show: boolean` prop (or just mount/unmount the component).

**Pattern found**: Project uses `@emotion/react`'s `keyframes` and MUI's `sx` prop animation strings — use the same approach, no new deps required.

## Key Decisions

- **No new library**: Implemented with CSS keyframes + MUI primitives to match existing patterns (`pulseRing`, `autostart-orbit`).
- **Promise.all for bulk archive**: Archives all merged tasks in parallel for speed; individual errors are caught and shown via existing `setError`.
- **Animation triggers immediately on confirm**: Doesn't wait for API calls to complete, making it feel snappier.
- **Separate confirmation dialog**: The existing `ArchiveConfirmDialog` is single-task focused (shows task name, archiving state). A separate dialog avoids awkward re-use.

## Codebase Patterns

- Project uses React 18 + MUI v5 + TypeScript.
- Animations use MUI `keyframes` from `@emotion/react` — no `framer-motion` or `react-spring`.
- API calls use `api.getApiClient()` (NEVER raw fetch).
- After mutations, refresh via re-fetching tasks (see `performArchive` at line 1081).
- Column-specific header buttons follow the backlog auto-start pattern (line 462).
