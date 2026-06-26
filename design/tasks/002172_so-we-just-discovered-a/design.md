# Design: Friendly Error When a Project's Agent Is Missing

## How the error happens today (root cause)

1. A project stores its default agent as `project.DefaultHelixAppID` →
   `api/pkg/types/project.go:237`.
2. Sessions resolve the agent name via `getAgentNameForSession`
   (`api/pkg/server/zed_config_handlers.go:504`). For an existing thread it returns the
   **stored** `session.Metadata.ZedAgentName` (e.g. `claude-acp`).
3. `deleteApp` (`api/pkg/server/app_handlers.go:1191`) deletes the agent App but does
   **not** clear `DefaultHelixAppID` on projects that reference it → dangling pointer.
4. On thread load, Zed can't register the agent and returns
   `Custom agent server `claude-acp` is not registered`
   (`zed/crates/agent_servers/src/custom.rs:258`), wrapped as
   `agent connect failed: …` (`zed/crates/external_websocket_sync/src/thread_service.rs:1698`)
   and sent to Helix as a `thread_load_error` SyncEvent.
5. Helix `handleThreadLoadError`
   (`api/pkg/server/websocket_external_agent_sync.go:3504`) stores the raw string on the
   interaction and, because the error matches neither `agentCrashErrorMarkers` nor the
   recurrence gate immediately, classifies it as transient → `MarkPromptAsFailed` →
   **auto-retries forever**.
6. The frontend (`RobustPromptInput.tsx`) shows the raw text with a "retrying now…"
   spinner; nothing tells the user the agent is gone.

## Approach

Translate and reclassify this error class in the **Helix layers** (Go backend +
React frontend). No behavioral change is required in the Rust fork — the existing
`is not registered` string is a sufficient, stable signal. Add a small delete-time
guard so new occurrences stop being created.

### 1. Backend: classify "agent not configured" (Go)

In `api/pkg/server/websocket_external_agent_sync.go`:

- Add `agentNotConfiguredMarkers` + `isAgentNotConfiguredError(errMsg)` alongside the
  existing `isAgentCrashError`. Markers: `"is not registered"` (covers
  `Custom agent server ... is not registered`, `Claude Agent is not registered`, etc.).
  This is robust because Zed already retries the "not registered" race 10× before
  surfacing it (`thread_service.rs:1686`), so by the time Helix sees it, it is terminal.
- In `handleThreadLoadError`, when `isAgentNotConfiguredError(errorMsg)` is true:
  - Set the interaction `Error` to a **friendly, stable message** (a fixed marker string
    the frontend can detect), e.g.
    `"AGENT_NOT_CONFIGURED: This project doesn't have an agent configured. ..."`.
  - Treat it as **terminal**: `MarkPromptAsCrashed` (pins `next_retry_at` far future to
    suppress auto-retry) — same suppression as crashes, but a distinct message so the UI
    can show a different CTA. Do **not** trigger `maybeAutoRestartCrashedAgent` (restart
    won't help — the agent is gone, not crashed).
- Keep the existing crash / transient paths unchanged (check the new class first).

### 2. Backend: clear dangling references on agent delete (Go)

In `deleteApp` (`api/pkg/server/app_handlers.go:1191`), before/after `DeleteApp`:
- Find projects with `DefaultHelixAppID == id` and clear the field (set empty) via the
  store. Keep it simple — a list + update loop using existing project store methods.
- This prevents the broken state from recurring. Existing broken projects are still
  handled by step 1/3 at runtime.

### 3. Frontend: friendly rendering + CTA (React)

In `frontend/src/components/common/RobustPromptInput.tsx`:
- Add an `AGENT_NOT_CONFIGURED` marker to the failure detection (mirrors the backend
  string, like the existing `crashedErrorMarkers` list).
- When detected, render the failure in an info/warning style (not the red crash style)
  with the friendly text and a **"Configure agent"** action that navigates to the
  project's agent settings (`router.navigate` per the react-router5 pattern; Project
  Settings already manages `default_helix_app_id` at
  `frontend/src/pages/ProjectSettings.tsx:659`). Fall back to the org Agents route
  (`/orgs/:org_id/agents`, `frontend/src/router.tsx:106`) if no project context.
- This is a third state alongside the existing transient and crashed states — keep the
  three mutually exclusive.

## Key Decisions

- **Translate in Helix, not Zed.** The friendly message and reclassification belong in
  the product layer; the fork only needs to keep emitting a recognizable string. Avoids a
  Rust rebuild + `sandbox-versions.txt` bump for a UX fix.
- **Match on `is not registered`, terminal.** Zed's own 10× retry means this string only
  reaches Helix when truly unrecoverable, so suppressing auto-retry is safe and correct.
- **Distinct from crash.** Reuse `MarkPromptAsCrashed`'s retry-suppression mechanism but
  with a separate message + CTA ("Configure agent", not "Restart"), because a missing
  agent is a configuration problem, not a process death.
- **Fix the source too.** Clearing `DefaultHelixAppID` on delete stops new broken
  projects from appearing; the runtime translation handles already-broken ones.

## Testing

- **Go unit tests** (`websocket_external_agent_sync_test.go`, gomock/testify suite
  pattern already present): add a case feeding a `thread_load_error` with
  `"... is not registered"` and assert the interaction gets the friendly message and is
  marked terminal (not auto-retried), and that a hard-crash / transient error keep their
  existing classification.
- **Delete guard test**: deleting an App referenced by a project clears the project's
  `DefaultHelixAppID`.
- **End-to-end (inner Helix at `localhost:8080`)**: create a project + agent, start a
  live spec-task session (provisions repo so Zed connects), delete the agent App, send a
  message, and confirm the friendly message + "Configure agent" CTA appear instead of the
  raw error and the retry loop. Live Zed testing is required for lifecycle changes.

## Files to touch (estimate)

- `api/pkg/server/websocket_external_agent_sync.go` — new classifier + handling branch.
- `api/pkg/server/app_handlers.go` — clear dangling project references on delete.
- `frontend/src/components/common/RobustPromptInput.tsx` — friendly state + CTA.
- Tests: `api/pkg/server/websocket_external_agent_sync_test.go` (+ a store/delete test).
