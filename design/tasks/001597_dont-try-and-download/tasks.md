# Implementation Tasks

- [x] In `api/pkg/types/simple_spec_task.go`, add `SandboxState string` and `SandboxStatusMessage string` fields with `gorm:"-"` and appropriate JSON tags
- [x] In `api/pkg/server/spec_driven_task_handlers.go`, inside the existing session loop (around line 243), derive sandbox state from `session.Config` fields (`ExternalAgentStatus`, `DesiredState`, `ContainerName`, `StatusMessage`) using the same logic as `useSandboxState`, and assign to `task.SandboxState` / `task.SandboxStatusMessage`
- [x] Run `./stack update_openapi` to regenerate `frontend/src/api/api.ts`
- [x] In `ExternalAgentDesktopViewer.tsx`, add optional `sandboxState`/`sandboxStatusMessage` props; when provided (screenshot mode from Kanban), use them directly and skip the `useSandboxState` polling
- [x] In `TaskCard.tsx`, pass `task.sandbox_state` and `task.sandbox_status_message` to `LiveAgentScreenshot` → `ExternalAgentDesktopViewer`
- [ ] In `ScreenshotViewer.tsx`, add an `enabled` prop (default `true`); add `!enabled` to the guard condition in the auto-refresh `useEffect` so polling stops when the desktop is absent
- [ ] In `ExternalAgentDesktopViewer.tsx`, pass `enabled={sandboxState !== 'absent'}` to `ScreenshotViewer`
- [~] Verify in the browser Network tab that `GET /api/v1/sessions/{id}` is never called while browsing the Kanban board
- [ ] Verify `GET /api/v1/external-agents/{id}/screenshot` is never called for stopped/absent task cards
- [ ] Verify sandbox state (absent / starting / running) still displays correctly on task cards
