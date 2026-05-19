# Implementation Tasks: Persist Skip Planning and Start Immediately Toggles

- [x] In `frontend/src/components/tasks/NewSpecTaskForm.tsx`, add two new module-level constants beside `LAST_LABELS_KEY` (line 53): `LAST_JUST_DO_IT_KEY = "helix_new_spectask_just_do_it"` and `LAST_AUTO_START_KEY = "helix_new_spectask_auto_start"`.
- [x] Replace the `useState(false)` initialisers for `justDoItMode` and `autoStart` (lines 120-121) with lazy initialisers that read each key from `localStorage`, `JSON.parse` it, and fall back to `false` inside a `try/catch`.
- [x] Add `handleJustDoItChange` and `handleAutoStartChange` helpers in the component body that call `setJustDoItMode`/`setAutoStart` and then `localStorage.setItem(key, JSON.stringify(checked))` inside a `try/catch`.
- [x] Wire the two checkboxes' `onChange` props (lines 990 and 1034) to call the new handlers.
- [x] Update the Cmd/Ctrl+J keyboard shortcut at line 488 to route through `handleJustDoItChange` so keyboard toggling also persists. (Discovered during implementation — not in the original design.)
- [x] In `resetForm` (lines 319-342), delete the `setJustDoItMode(false)` and `setAutoStart(false)` lines so the toggles persist across task creations. Add a short comment near the deleted lines explaining the intentional persistence (mirroring the existing labels comment at line 323).
- [x] Run `cd frontend && yarn tsc --noEmit` and confirm it passes. (Full `yarn build` is blocked by a `dist/` bind-mount permission issue unrelated to this change; type-check is the meaningful signal.)
- [x] Manually verify in the inner Helix browser (`http://localhost:8080`). All four scenarios pass: (1) toggling each checkbox writes immediately to `localStorage`; (2) closing/reopening the form restores the checked state; (3) creating a task does NOT reset the checkboxes — they survive submit; (4) reloading the page with corrupt JSON (`"not-json"`, `"{broken"`) falls back to `false` cleanly with no console errors. Also verified: Cmd/Ctrl+J keyboard shortcut persists. Screenshots saved in `screenshots/`.
- [x] Write per-repo PR description (`pull_request_helix.md`).
- [x] Commit with `feat(frontend): persist Skip planning and Start immediately toggles` and push the feature branch `feature/002013-persist-skip-planning`.
