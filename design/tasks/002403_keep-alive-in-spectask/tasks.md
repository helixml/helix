# Implementation Tasks: Make Keep Alive Toggle Green in Light and Dark Mode

- [ ] Bring up the inner Helix at `http://localhost:8080` (register `test@helix.ml` / `helixtest`, complete onboarding), create a spec task and open its detail page with the desktop running so the Keep Alive toggle is visible
- [ ] Reproduce and confirm the defect: screenshot the Keep Alive lock icon ON and OFF in **dark** mode and in **light** mode, and record which mode fails to read as green (design assumes light mode)
- [ ] In `frontend/src/components/tasks/SpecTaskDetailContent.tsx`, derive `const keepAliveOnColor = lightTheme.isLight ? "#1e8e3e" : "#66bb6a"` in the component body, with a short comment explaining that MUI's default `success.main` is green 800 in light mode
- [ ] Replace `sx={{ color: task.keep_alive ? "success.main" : "text.secondary" }}` on the Keep Alive `IconButton` (~line 2293) with `keepAliveOnColor`, leaving the OFF state as `text.secondary`
- [ ] Confirm the OFF state, tooltips, `disabled` behaviour and immediate colour flip after clicking are all unchanged
- [ ] Re-screenshot ON and OFF in both light and dark mode; save the before/after set to `design/tasks/002403_keep-alive-in-spectask/screenshots/`
- [ ] Run `cd frontend && yarn build` and confirm it passes
- [ ] Commit with a conventional message (e.g. `fix(frontend): make Keep Alive toggle green in light mode`), push, open a PR, and check Drone CI goes green
