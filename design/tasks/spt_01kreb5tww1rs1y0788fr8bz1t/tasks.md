# Implementation Tasks: Fix dark-on-dark text in message queue header in light mode

- [ ] In `frontend/src/components/common/RobustPromptInput.tsx`, add a `color` rule to the queue-header `Box` `sx` (around line 1161) that mirrors the existing `bgcolor` conditional: `color: editingId ? 'info.contrastText' : isOnline ? 'primary.contrastText' : 'warning.contrastText'`
- [ ] Run `cd frontend && yarn build` (or rely on the Vite dev server in `helix-frontend-1` for HMR) and verify it compiles cleanly
- [ ] In the inner Helix at `http://localhost:8080`, switch to **light mode** and reproduce the queue header in all three states (online queued, offline queued, editing); confirm the label and icon are clearly readable on the dark header strip
- [ ] Switch to **dark mode** and confirm there is no visual regression (header still looks identical to before)
- [ ] Capture before/after screenshots into `screenshots/` in this task folder (one pair per state is enough; light-mode before is the key one)
- [ ] Commit with a conventional-commit subject like `fix(frontend): make message queue header readable in light mode` and open a PR against `helixml/helix`
