# Implementation Tasks

- [ ] In `api/pkg/server/agent_sandboxes_handlers.go`, add `StatusMessage string` to `SessionSandboxStateResponse` and rewrite `getSessionSandboxState` to derive state from `session.Config` fields (`external_agent_status`, `desired_state`, `container_name`, `status_message`) using the same logic currently in `useSandboxState`
- [ ] Run `./stack update_openapi` to regenerate the frontend API client (`frontend/src/api/api.ts`)
- [ ] In `ExternalAgentDesktopViewer.tsx`, replace `apiClient.v1SessionsDetail(sessionId)` with `apiClient.v1SessionsSandboxStateDetail(sessionId)` and map `response.data.state` / `response.data.status_message` directly
- [ ] Verify the Kanban board no longer calls `GET /api/v1/sessions/{id}` for task cards (check Network tab)
- [ ] Verify running desktops still show correct state transitions (absent → starting → running)
