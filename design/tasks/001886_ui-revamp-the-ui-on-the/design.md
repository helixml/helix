# Design: Project Board "Skip Planning" UI Polish

## Current State

**File:** `frontend/src/components/tasks/SpecTaskActionButtons.tsx`

### Backlog phase (lines 253-365)
- A `Switch` toggle labeled "Skip planning" sits below the "Start Planning" button.
- Toggling the switch sets `just_do_it_mode: true` on the task via a mutation.
- This changes the button color from yellow to green and the label from "Start Planning" to "Start Implementation."
- The toggle and button are tightly coupled — the switch mutates backend state and the button reads it back to decide its label and color.

### Spec generation phase (lines 367-406)
- A separate "Skip Planning" `Button` (outlined, warning color, with a `SkipNext` icon) is rendered.
- Clicking it calls `skipSpecMutation.mutate()` which sets `status: implementation` and `just_do_it_mode: true`.

## Proposed Changes

### Change 1: Replace the toggle with a split-button or secondary text link

**Decision:** Replace the `Switch` toggle with a small text link below the primary button: **"or skip to implementation"**.

**Rationale:**
- A text link is visually subordinate — planning stays the dominant action.
- No toggle state means the primary button always says "Start Planning" (no confusing label swap).
- Clicking the link directly starts implementation (sets `just_do_it_mode: true` and triggers `onStartPlanning`).
- This eliminates the intermediate "toggle on, then click" two-step interaction.

**Behavior:**
- Primary button: "Start Planning" (yellow, `PlayIcon`) — unchanged behavior, calls `onStartPlanning()` with `just_do_it_mode: false`.
- Text link: "or skip to implementation" — calls `updateSpecTaskMutation` to set `just_do_it_mode: true`, then immediately calls `onStartPlanning()`.
- Error/retry state: If `task.metadata?.error` exists, show "Retry Planning" on the button and "or retry as implementation" on the link.
- The button color stays yellow (warning) always — no green state, since `just_do_it_mode` is no longer a persistent toggle.

**Inline variant** (used in compact views):
- Show just the "Start Planning" button. The skip link is omitted in inline mode for space reasons — users can use the card's expanded view to skip.

### Change 2: Remove the "Skip Planning" button during spec generation

**Decision:** Delete the entire `if (task.status === "spec_generation")` block (lines 367-406).

**Rationale:**
- If planning is already running, let it complete. The user can still archive/delete the task if they truly want to abandon it.
- Removing mid-flight skip avoids wasted compute (the planning container keeps running even after skip).

**Note:** The `useSkipSpec` hook in `specTaskWorkflowService.ts` (lines 118-145) can remain — it may be used elsewhere or could be cleaned up in a follow-up. The priority is removing it from the UI.

## Codebase Patterns Observed

- **Material-UI** used throughout (`Button`, `Tooltip`, `Typography`, `Box`, etc.)
- **React Query mutations** for all state changes (`useMutation` + `queryClient.invalidateQueries`)
- **Two render paths** per phase: `isInline` (compact row) vs stacked (card column)
- **`CompactActionButton`** is the inline variant's button component
- The `SpecTaskActionButtons` component handles all phases: backlog, spec_generation, review, implementation, PR

## Impact

- **Files changed:** 1 (`SpecTaskActionButtons.tsx`)
- **Backend:** No changes needed — `just_do_it_mode` and `onStartPlanning` handler logic remain the same
- **Risk:** Low — purely cosmetic UI change within a single component
