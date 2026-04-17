# Design: Rename "Just Do It" to "Skip Spec"

## Overview

Pure frontend relabeling. The backend field `just_do_it_mode` (DB column: `yolo_mode`) stays unchanged — only UI labels and helper text change. No API or data model modifications.

## Key Decisions

**Keep the internal field name `just_do_it_mode`** — Renaming the JSON/DB field would require a migration and API changes for a cosmetic fix. The UI label is what users see; the field name is an implementation detail.

**Add a "Skip spec" toggle to the backlog task card** — Currently users can only set this during task creation or in edit mode. Adding it to the backlog card (next to "Start Planning") makes the option discoverable without extra clicks.

**Single action button when skip spec is on** — When `just_do_it_mode` is true, the backlog card shows only "Start Implementation" (not "Start Planning" + a separate toggle). This makes the outcome unambiguous.

## Changes by File

### `SpecTaskActionButtons.tsx`
The main file with changes. Current backlog-phase logic (lines 250-328):

- **Button label** (lines 268-273): Replace `"Just Do It"` → `"Start Implementation"`. Replace `"Retry"` (JDI error case) → `"Retry Implementation"`.
- **Button color** (line 280): Change from `"success"` to `"warning"` (same as normal mode) or keep `"success"` to visually distinguish — recommend keeping `"success"` so users see the mode difference.
- **Add a "Skip spec" toggle** next to the action button for backlog tasks. This toggle calls the existing update API to flip `just_do_it_mode`. It should be a small switch or checkbox, not a full button.

### `NewSpecTaskForm.tsx`
- **Checkbox label** (around line 567-575): Replace "Just do it" references with "Skip spec".
- **Helper/placeholder text**: Update to "Skip spec generation and go straight to implementation" (similar to the tooltip already used on the Skip Spec button during planning).
- **Keyboard shortcut hint**: If displayed, update tooltip text.

### `SpecTaskDetailContent.tsx`
- **Edit mode checkbox** (lines 1306-1316): Label already says "Skip planning (go straight to implementation)". Optionally update to "Skip spec" for consistency.

### `TaskCard.tsx`
- No structural changes needed — it passes `just_do_it_mode` and action handlers through. The toggle in `SpecTaskActionButtons` handles the UI.

## Codebase Patterns Observed

- The project uses Material-UI components (`FormControlLabel`, `Checkbox`, `Button`, `Switch`, `Tooltip`).
- Snackbar notifications are shown via `enqueueSnackbar` (notistack) when toggling modes — reuse this pattern for the new toggle.
- The `useUpdateSpecTask` hook from `specTaskWorkflowService.ts` handles `just_do_it_mode` updates already.
- The existing "Skip Spec" button during spec_generation phase (lines 331-369) provides the exact UX language to copy.
