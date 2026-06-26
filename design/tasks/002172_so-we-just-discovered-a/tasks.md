# Implementation Tasks: Handle Deleted Agents Gracefully in Projects and Sessions

## Facet A — friendly error for missing project agent

- [ ] Add `agentNotConfiguredMarkers` + `isAgentNotConfiguredError(errMsg)` in `api/pkg/server/websocket_external_agent_sync.go` (marker: `"is not registered"`).
- [ ] In `handleThreadLoadError`, branch on this class first: set a stable friendly interaction `Error` (e.g. `AGENT_NOT_CONFIGURED: …`) and mark the prompt terminal via `MarkPromptAsCrashed` (no auto-retry, no auto-restart).
- [ ] In `RobustPromptInput.tsx`, add an `AGENT_NOT_CONFIGURED` failure state (mirror the marker) rendered in info/warning style with a "Configure agent" CTA to the project's agent settings (fallback `/orgs/:org_id/agents`).

## Facet B — block sessions whose agent was deleted

- [ ] Add a computed `agent_missing` (or `agent_available`) field to the session GET response: in the session handler, resolve the session's agent App (spec-task `HelixAppID` then `ParentApp`); set true when the reference is set but the App no longer exists. Add the field to `types.go` and regenerate the API client (`./stack update_openapi`).
- [ ] In the session/spec-task chat view (`SpecTaskDetailContent.tsx`, `Session.tsx`), when `agent_missing` is true render the banner ("There is currently no agent assigned to this session. Before we can proceed, please assign one.") and **disable the message input + send**.
- [ ] In that banner, surface agent assignment by reusing `SwitchAgentControl` / `useSwitchAgent`; on success invalidate the session query so the banner clears and input re-enables.
- [ ] Handle the paused-session edge case (direct the user to the active descendant instead of offering a switch that 409s).

## Facet E — prevent dangling references on delete

- [ ] In `deleteApp` (`app_handlers.go`), clear `DefaultHelixAppID` on any project that referenced the deleted App.

## Tests

- [ ] Go unit test: `thread_load_error` with `"is not registered"` → friendly terminal message (no auto-retry); crash & transient cases unchanged.
- [ ] Go test: `GetSession` reports `agent_missing` true when the bound App is deleted, false when it exists.
- [ ] Go test: deleting an App referenced by a project clears the project's `DefaultHelixAppID`.
- [ ] End-to-end in inner Helix with a live Zed session: delete the session's agent → banner + disabled input; assign a new agent inline → banner clears and a follow-up message succeeds on the same session; confirm Facet A friendly message + CTA also appear.

## Wrap-up

- [ ] `cd frontend && yarn build` and `go build ./pkg/server/...` pass.
- [ ] Verify final user-facing wording (both banner and Facet A message) with the team.
