# Requirements: Tooltip for Truncated Task Titles + Canonical Prompt Field

## Background

A spec task currently has three prompt-related fields that are all set at creation to the same value and then diverge:

| Field | Role | Editable |
|---|---|---|
| `name` | Short title — auto-generated from prompt at creation, then independently user-editable as its own field; NOT recalculated when `description` changes | Yes (detail panel, as a separate field) |
| `description` | Working prompt — what the agent receives | Yes (detail panel + backlog inline) |
| `original_prompt` | Immutable original text for audit | No |

The agent already uses `description` as the canonical prompt (with `original_prompt` as fallback only when `description` is empty). However, the frontend is inconsistent:
- `BacklogTableView.tsx` populates the inline prompt editor from `original_prompt`, but saves edits back to `description`. This means edits made via the detail panel disappear when you re-open the inline editor in the backlog.
- Tooltip/display code uses a `description || original_prompt || name` fallback chain, implying it isn't clear which is authoritative.

## Scope of This Task

### 1. Fix the canonical field inconsistency

The inline editor in `BacklogTableView.tsx` must read `description` (not `original_prompt`) so that edits round-trip correctly regardless of which editor the user uses. `original_prompt` stays in the database as an immutable audit field but should not be the source of truth for editing.

### 2. Add tooltips for truncated task names

In three locations, task titles are truncated without showing the full text on hover:

- **Kanban card** (`TaskCard.tsx`): Shows `task.name` truncated; tooltip should show `task.description`.
- **Split-screen tab heading** (`TabsView.tsx`): Tab label truncated at 280px; the existing tooltip should show `description` in the no-session branch.
- **Notifications panel** (`GlobalNotifications.tsx`): `event.title` and `event.spec_task_name` lines both truncated with CSS ellipsis and no tooltip.

## Acceptance Criteria

1. The backlog inline editor initializes from `task.description` (not `task.original_prompt`).
2. Hovering the Kanban card title shows a tooltip with the full `task.description`.
3. Hovering a tab label (no session title history) shows `task.description` in the tooltip.
4. Hovering a notification row shows the full `event.title` and `event.spec_task_name`.
5. Multi-line prompt text is rendered with preserved line breaks (`whiteSpace: "pre-wrap"`).
6. Tooltip enter delay ~500ms, consistent with existing tooltips.
7. No display regressions — tasks where `description` is empty (edge case for very old tasks) fall back to `name`.
