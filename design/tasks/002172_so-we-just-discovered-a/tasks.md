# Implementation Tasks: Friendly Error When a Project's Agent Is Missing

## Backend — error classification & handling

- [ ] Add `agentNotConfiguredMarkers` and `isAgentNotConfiguredError(errMsg)` in `api/pkg/server/websocket_external_agent_sync.go` (marker: `"is not registered"`).
- [ ] In `handleThreadLoadError`, branch on `isAgentNotConfiguredError` (checked before crash/transient): set a friendly, stable interaction `Error` (e.g. `AGENT_NOT_CONFIGURED: …`) and mark the prompt terminal via `MarkPromptAsCrashed` to suppress auto-retry.
- [ ] Ensure this branch does NOT call `maybeAutoRestartCrashedAgent` (restart cannot fix a missing agent).
- [ ] Confirm existing crash and transient classifications are unchanged.

## Backend — prevent dangling references

- [ ] In `deleteApp` (`api/pkg/server/app_handlers.go`), find projects whose `DefaultHelixAppID == id` and clear that field, so no project is left pointing at a deleted agent.

## Frontend — friendly surfacing

- [ ] In `RobustPromptInput.tsx`, add an `AGENT_NOT_CONFIGURED` detection branch (mirror the backend marker) as a third, mutually-exclusive failure state.
- [ ] Render it in info/warning style with the friendly message and a "Configure agent" CTA navigating to the project's agent settings (fallback: `/orgs/:org_id/agents`).

## Tests

- [ ] Go unit test: `thread_load_error` with `"is not registered"` → friendly message + terminal (no auto-retry); crash & transient cases keep prior behavior.
- [ ] Go test: deleting an App referenced by a project clears the project's `DefaultHelixAppID`.
- [ ] End-to-end in inner Helix with a live Zed session: delete the project's agent, send a message, verify friendly message + "Configure agent" CTA (no raw error, no retry loop).

## Wrap-up

- [ ] `cd frontend && yarn build` and `go build ./pkg/server/...` pass.
- [ ] Verify final user-facing wording with the team.
