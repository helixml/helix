# Requirements: Rename "Just Do It" to "Skip Spec"

## Context

The "Just do it" toggle when creating a spec task is unclear to users. A "Skip Spec" option already exists during the planning phase with clear semantics. We should unify the terminology and simplify the backlog actions based on the toggle state.

## User Stories

**US-1: Consistent terminology**
As a user, I want the option to skip spec creation to be labeled "Skip spec" everywhere, so I immediately understand what it does.

**US-2: Skip spec toggle on backlog tasks**
As a user, I want to toggle "Skip spec" directly from a backlog task (alongside "Start Planning"), so I can decide before starting work.

**US-3: Simplified actions when skip spec is enabled**
As a user, when "Skip spec" is toggled on for a backlog task, I want to see a single "Start Implementation" action instead of "Start Planning", so the button matches what will actually happen.

## Acceptance Criteria

1. **Task creation form**: The "Just do it" checkbox/toggle is renamed to "Skip spec". Helper text updated to match (e.g., "Skip spec generation and go straight to implementation").
2. **Backlog task card**: A "Skip spec" toggle is visible alongside the primary action button, allowing users to enable/disable it without entering edit mode.
3. **Backlog action button label**:
   - When skip spec is OFF → "Start Planning" (existing behavior)
   - When skip spec is ON → "Start Implementation"
4. **Error/retry labels**:
   - When skip spec is OFF + error → "Retry Planning" (existing)
   - When skip spec is ON + error → "Retry Implementation"
5. **Task detail edit mode**: The existing checkbox label "Skip planning (go straight to implementation)" can remain as-is or be updated to "Skip spec" for consistency.
6. **No backend changes required** — the underlying `just_do_it_mode` field and API behavior stay the same; this is a UI-only rename.
7. **Keyboard shortcut** (Ctrl/Cmd+J) in the creation form continues to work, toggling the renamed "Skip spec" option.
