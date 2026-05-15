# Implementation Tasks: Persist Skip Planning and Start Immediately Toggles

- [ ] In `frontend/src/components/tasks/NewSpecTaskForm.tsx`, add two new module-level constants beside `LAST_LABELS_KEY` (line 53): `LAST_JUST_DO_IT_KEY = "helix_new_spectask_just_do_it"` and `LAST_AUTO_START_KEY = "helix_new_spectask_auto_start"`.
- [ ] Replace the `useState(false)` initialisers for `justDoItMode` and `autoStart` (lines 120-121) with lazy initialisers that read each key from `localStorage`, `JSON.parse` it, and fall back to `false` inside a `try/catch`.
- [ ] Add `handleJustDoItChange` and `handleAutoStartChange` helpers in the component body that call `setJustDoItMode`/`setAutoStart` and then `localStorage.setItem(key, JSON.stringify(checked))` inside a `try/catch`.
- [ ] Wire the two checkboxes' `onChange` props (lines 990 and 1034) to call the new handlers.
- [ ] In `resetForm` (lines 319-342), delete the `setJustDoItMode(false)` and `setAutoStart(false)` lines so the toggles persist across task creations. Add a short comment near the deleted lines explaining the intentional persistence (mirroring the existing labels comment at line 323).
- [ ] Run `cd frontend && yarn build` and confirm it passes.
- [ ] Manually verify in the inner Helix browser (`http://localhost:8080`) that toggling each checkbox, creating a task, reloading the page, and switching projects all preserve the checkbox state. Verify corrupting the localStorage value to `"not-json"` falls back to `false` without throwing.
- [ ] Commit with `feat(frontend): persist skip-planning and start-immediately toggles in new task form` and push.
