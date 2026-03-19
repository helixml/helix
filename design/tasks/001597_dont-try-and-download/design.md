# Design: Don't Download Sessions for Stopped Desktops

## Root Cause

`useSandboxState` in `ExternalAgentDesktopViewer.tsx` calls `GET /api/v1/sessions/{id}` (the full session object) every 3 seconds per task card. It only uses four fields from it:

- `config.external_agent_status` — "stopped" / "running" / "starting"
- `config.desired_state` — "running" / "stopped"
- `config.container_name` — existence check only
- `config.status_message` — transient message e.g. "Unpacking build cache (2.1/7.0 GB)"

There is already a lightweight endpoint `GET /api/v1/sessions/{id}/sandbox-state` but it's too basic: it only checks `session.SandboxID != ""` and returns "running" or "absent". It doesn't handle "starting", `desired_state`, `status_message`, or `external_agent_status`.

## Fix

Two-part change:

### 1. Extend the backend `/sandbox-state` endpoint

Add the missing fields to `SessionSandboxStateResponse` and populate them from session config:

```go
type SessionSandboxStateResponse struct {
    SessionID     string `json:"session_id"`
    State         string `json:"state"`          // "absent", "running", "starting"
    ContainerID   string `json:"container_id,omitempty"`
    StatusMessage string `json:"status_message,omitempty"` // transient message
}
```

Derive `State` using the same logic currently in `useSandboxState`:
- `external_agent_status == "stopped"` OR `desired_state == "stopped"` → "absent"
- `external_agent_status == "running"` OR (`container_name != ""` AND `desired_state == "running"`) → "running"
- `external_agent_status == "starting"` OR (`container_name == ""` AND `desired_state == "running"`) → "starting"
- default → "absent"

Return `status_message` from `config.status_message`.

Run `./stack update_openapi` to regenerate the frontend API client.

### 2. Switch `useSandboxState` to use the lightweight endpoint

Replace `apiClient.v1SessionsDetail(sessionId)` with `apiClient.v1SessionsSandboxStateDetail(sessionId)` and map the response fields directly. No logic change — just a different (tiny) response.

## Key Files

| File | Change |
|------|--------|
| `api/pkg/server/agent_sandboxes_handlers.go:80` | Extend `SessionSandboxStateResponse`, fix `getSessionSandboxState` logic |
| `api/pkg/server/swagger.json` + `docs.go` + `swagger.yaml` | Regenerated via `./stack update_openapi` |
| `frontend/src/api/api.ts` | Regenerated via `./stack update_openapi` |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:43` | Switch to `v1SessionsSandboxStateDetail` |

## Result

A stopped task card makes exactly 0 repeated requests (or 1 per render if we keep the initial fetch, which returns immediately as "absent"). A running task card fetches ~200 bytes every 3 seconds instead of the full session object.
