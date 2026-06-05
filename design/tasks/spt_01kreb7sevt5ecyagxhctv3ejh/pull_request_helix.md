# feat(frontend): persist Skip planning and Start immediately toggles

## Summary

The "Skip planning" (`justDoItMode`) and "Start immediately" (`autoStart`)
checkboxes on the New SpecTask form used to default to unchecked on every
mount and reset back to unchecked after each task creation. A user who
always wants to skip planning had to tick the box for every new task.
This PR makes both toggles remember their state across uses, using
`localStorage` — mirroring the existing label-persistence pattern in the
same component.

## Changes

- `frontend/src/components/tasks/NewSpecTaskForm.tsx`
  - Added `LAST_JUST_DO_IT_KEY` and `LAST_AUTO_START_KEY` module
    constants.
  - `justDoItMode` and `autoStart` `useState` hooks now lazy-initialise
    from `localStorage` with a `try/catch` fallback to `false`.
  - New `handleJustDoItChange` / `handleAutoStartChange` callbacks update
    React state AND write through to `localStorage` on every toggle.
  - Wired both checkbox `onChange` props and the existing Cmd/Ctrl+J
    keyboard shortcut through the new handlers, so keyboard and mouse
    toggles both persist.
  - Removed `setJustDoItMode(false)` / `setAutoStart(false)` from
    `resetForm` so the toggles survive task creation.

## Why this approach

- One file changed; no new utility, no backend, no API surface.
- Matches the existing `taskLabels` persistence pattern in the same file
  (no TTL — these are workflow preferences, not transient hints).
- Keys are global rather than project-scoped because these are user
  workflow preferences ("I always want to skip planning"), not state
  specific to one project.

## Screenshots

![Form reopened with toggles persisted](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb7sevt5ecyagxhctv3ejh/screenshots/02-form-reopened-toggles-persisted.png)
![Toggles survive task creation](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01kreb7sevt5ecyagxhctv3ejh/screenshots/03-toggles-survive-task-creation.png)

## Test plan

- [x] `cd frontend && yarn tsc --noEmit` passes (full `yarn build` is
  blocked by an unrelated `dist/` permission issue in the dev sandbox; all
  21104 modules transformed successfully before the write step).
- [x] End-to-end browser verification in the inner Helix
  (`http://localhost:8080`):
  - Tick both checkboxes → `localStorage` immediately contains
    `"true"` for both keys.
  - Close and reopen form → both boxes remain checked.
  - Create a task → task goes to "In Progress" (proving both flags were
    sent); reopen form → both boxes still checked.
  - Corrupt the `localStorage` values to `"not-json"` and reload → form
    mounts with both unchecked, no console errors.
  - Cmd/Ctrl+J also persists (`justDoIt: "true"` in `localStorage` after
    the shortcut fires).
