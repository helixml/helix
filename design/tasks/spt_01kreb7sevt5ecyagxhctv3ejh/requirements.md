# Requirements: Persist Skip Planning and Start Immediately Toggles

## Background

The "New Spec Task" form (`frontend/src/components/tasks/NewSpecTaskForm.tsx`) has
two checkboxes that influence how a task is created:

- **Skip planning** (`justDoItMode`) — go straight to implementation, no planning
  agent run.
- **Start immediately** (`autoStart`) — bypass the backlog and start as soon as
  the task is created.

Today both default to `false` on every mount and are reset to `false` after each
successful task creation (`resetForm`, lines 326-327). A user who always wants
to skip planning has to tick the box every single time. Other form fields
already persist this way — `taskLabels` is restored from
`helix_last_task_labels` and the prompt has a per-project draft — but the two
checkboxes do not.

## User Story

> As a user who creates many tasks in a row, I want the form to remember
> whether I had "Skip planning" and/or "Start immediately" checked the last
> time I created a task, so that I don't have to re-tick them on every new
> task.

## Acceptance Criteria

1. On mounting `NewSpecTaskForm`, the initial values of `justDoItMode` and
   `autoStart` come from `localStorage` rather than always being `false`.
2. When the user toggles either checkbox, the new value is written to
   `localStorage` immediately (so the next mount reflects it even if no task
   is submitted).
3. After a successful task creation, the form does NOT reset these two
   checkboxes back to `false` — they retain whatever the user last set them
   to. (All the other reset behaviour in `resetForm` is unchanged.)
4. Behaviour is preserved across:
   - Closing and reopening the new-task panel/dialog.
   - Reloading the page.
   - Switching between projects (the preference is global to the user, not
     project-scoped — see "Design").
5. If `localStorage` is unavailable or contains corrupt JSON for these keys,
   the form must fall back to `false` for both — no crashes, no thrown
   errors during render.
6. No new API surface, no backend changes — this is a frontend-only change.

## Out of Scope

- Surfacing these preferences anywhere outside the form (e.g. user settings
  page).
- Adding TTL/expiry for the saved values. Labels don't expire either; we keep
  things consistent.
- Persisting any of the other form fields beyond what is already persisted.
