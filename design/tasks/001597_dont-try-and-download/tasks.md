# Implementation Tasks

- [x] In `api/pkg/types/simple_spec_task.go`, add `SandboxState string` and `SandboxStatusMessage string` fields with `gorm:"-"` and appropriate JSON tags
- [x] In `api/pkg/server/spec_driven_task_handlers.go`, inside the existing session loop (around line 243), derive sandbox state from `session.Config` fields (`ExternalAgentStatus`, `DesiredState`, `ContainerName`, `StatusMessage`) using the same logic as `useSandboxState`, and assign to `task.SandboxState` / `task.SandboxStatusMessage`
- [x] Run `./stack update_openapi` to regenerate `frontend/src/api/api.ts`
- [x] In `ExternalAgentDesktopViewer.tsx`, add optional `sandboxState`/`sandboxStatusMessage` props; when provided (screenshot mode from Kanban), use them directly and skip the `useSandboxState` polling
- [x] In `TaskCard.tsx`, pass `task.sandbox_state` and `task.sandbox_status_message` to `LiveAgentScreenshot` → `ExternalAgentDesktopViewer`
- [x] ScreenshotViewer `enabled` prop not needed — `ScreenshotViewer` is already gated behind `isRunning` in `ExternalAgentDesktopViewer`; when `sandbox_state` is "absent" `isPaused=true` and `ScreenshotViewer` never renders
- [x] In screenshot mode `isPaused` branch, removed the one-time screenshot `<img>` fetch — shows dark background instead since we know the sandbox is absent
- [x] Verified: `GET /api/v1/sessions/{id}` no longer called from Kanban board
- [x] Verified: Screenshot polling only happens when `sandbox_state === "running"` (correct behaviour)
- [x] Verified: `sandbox_state` correctly populated from backend session config
