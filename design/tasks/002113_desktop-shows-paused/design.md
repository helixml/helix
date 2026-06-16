# Design: Fix Desktop Showing "Paused" While Container Is Running

## Overview

This is a small, well-isolated bug fix. A trailing read-modify-write on a stale
in-memory `session` struct overwrites container metadata that `StartDesktop`
already persisted to the same row. The fix removes the clobbering write.

## The clobber, in detail

`StartExternalAgentSession` (`api/pkg/server/session_handlers.go:2543-2554`):

```go
agentResp, err := s.externalAgentExecutor.StartDesktop(ctx, zedAgent)
// ... StartDesktop has ALREADY persisted container_name,
//     external_agent_status="running", dev_container_id, sandbox_id.

if agentResp.DevContainerID != "" || agentResp.SandboxID != "" {
    session.Metadata.DevContainerID = agentResp.DevContainerID  // already persisted
    session.SandboxID = agentResp.SandboxID                     // already persisted
    s.Controller.Options.Store.UpdateSession(ctx, *session)     // <-- wipes the rest
}
```

The in-memory `session` was last written at line 2516 (`WriteSession`), before
`StartDesktop` ran. It never received `container_name` or
`external_agent_status`, so persisting it back blanks those columns.

## Decision: re-fetch instead of re-save

Replace the stale-struct write with a re-fetch of the freshest row. The two
fields the old block applied (`DevContainerID`, `SandboxID`) are already written
by `StartDesktop`, so no separate persist is needed.

```go
// StartDesktop already persisted the container metadata (container_name,
// external_agent_status="running", dev_container_id, sandbox_id) onto the
// session row. Re-fetch the fresh row instead of re-saving our stale
// in-memory copy — re-saving it wipes container_name/external_agent_status
// and makes the desktop viewer render "paused".
if fresh, err := s.Store.GetSession(ctx, session.ID); err == nil {
    session = fresh
}
```

### Why re-fetch (not "just delete the block")

Deleting the block alone fixes the persisted row, but the function returns the
stale in-memory `session` to its callers (cron trigger system, helix-org).
Re-fetching makes the returned struct reflect the actual persisted state
(`external_agent_status="running"`, `container_name` set), so any caller reading
the returned value immediately sees the correct status.

### Alternatives considered

- **Partial/column-scoped update** of only `DevContainerID`/`SandboxID`: more
  code, and unnecessary since `StartDesktop` already persists both. Rejected.
- **Copy container fields from `agentResp` onto `session` before saving**: still
  a read-modify-write on a stale base; would need to copy `ContainerName`,
  `ExternalAgentStatus`, `ContainerID`, etc. — fragile and duplicates
  `StartDesktop`'s persistence. Rejected.

## Edge case: already-running early return

`StartDesktop` returns early (hydra_executor.go:149-159) when the container is
already running, with empty `DevContainerID`/`SandboxID` in the response and
**without** running the DB-update block. The old guard
(`if agentResp.DevContainerID != "" || agentResp.SandboxID != ""`) was already a
no-op in that case, so the row keeps its previously-persisted metadata. The
re-fetch is safe here too — it simply reloads the existing good row.

## Files Touched

| File | Change |
|------|--------|
| `api/pkg/server/session_handlers.go` | Replace stale `UpdateSession(*session)` block (~2548-2554) with a re-fetch of the session row. |
| `api/pkg/server/session_handlers_test.go` (or suite) | Add regression test asserting metadata survives. |

## Testing

- **Regression unit test**: with a mock store + mock executor, simulate
  `StartDesktop` writing `container_name`/`external_agent_status="running"` to
  the stored row, then assert `StartExternalAgentSession` leaves those fields
  intact (does not blank them). Note: `MockExecutor.StartDesktop` does not write
  to the store by itself, so the test must either stub `GetSession` to return the
  enriched row or drive the store through an in-memory store.
- **End-to-end (inner Helix)**: start a session via the external-agent path
  (cron trigger / helix-org Worker) and confirm the desktop viewer shows
  "running", not "paused". Verify the DB row:
  `SELECT id, metadata->>'container_name', metadata->>'external_agent_status'
  FROM sessions WHERE id = '<ses_...>';`

## Implementation Notes

- The fix is a 3-line swap in `StartExternalAgentSession`: the trailing
  `if agentResp.DevContainerID != "" || agentResp.SandboxID != "" { ... UpdateSession(*session) }`
  block was replaced with a re-fetch `if fresh, err := s.Store.GetSession(ctx, session.ID); err == nil { session = fresh }`
  (with a warn-log fallback that keeps the pre-start copy if the reload fails).
- Removing the block left `agentResp` unused, so the `StartDesktop` call was
  changed from `agentResp, err := ...` to `if _, err := ...; err != nil`.
- Regression test lives in `api/pkg/server/start_external_agent_paused_test.go`
  (`StartExternalAgentPausedSuite`). It uses a stateful `MockStore` (a
  `map[string]*types.Session` backing `GetSession`/`UpdateSession`) plus a
  `MockExecutor` whose `StartDesktop` mirrors `HydraExecutor.StartDesktop` by
  persisting `container_name`/`external_agent_status="running"` onto the row.
  It asserts both the returned struct and the persisted row keep that metadata.
- Verified the test is a real gate: temporarily reinstated the buggy
  `UpdateSession(*session)` and the test failed on all four assertions
  (empty `container_name`, empty `external_agent_status`); restored the fix and
  it passes. Related suites (Exploratory, AutoWakeColdStart, AttachProjectContext)
  remain green.
- Test gotcha: `pkg/store/memorystore` is unusable here because its `GetUser`
  returns `ErrNotFound`, which `StartExternalAgentSession` treats as fatal. The
  stateful `MockStore` approach (modeled on `exploratory_session_activation_test.go`)
  sidesteps that.
- CGo is required to compile the server test package (tree-sitter):
  `sudo apt-get install -y gcc libc6-dev` then `CGO_ENABLED=1 go test ...`.

## End-to-end verification (localhost:8080)

Confirmed live on the inner Helix stack (2026-06-16).

- The exploratory / "Human Desktop" UI path was ruled out as a repro: its
  handler `startExploratorySession` (`project_handlers.go`) calls `StartDesktop`
  directly and **already re-fetches** the row afterwards, so it never exhibited
  the bug. The bug is exclusive to `StartExternalAgentSession`, reached only by
  cron triggers and helix-org workers.
- Reproduced through the real runtime via the cron-trigger execute endpoint
  (`POST /api/v1/triggers/{id}/execute` → `cron.ExecuteCronTask` →
  `executeExternalAgentCronTask` → `StartExternalAgentSession`):
  1. Register `test@helix.ml`, create org `testorg`, mint an org API key.
  2. Create a Helix-hosted repo (`is_external:false`), an app, and a project
     (project requires `default_repo_id` + `default_helix_app_id`).
  3. Create a cron trigger with `trigger.cron.agent_type = "zed_external"` and
     `project_id` set; fire it with the execute endpoint.
- Session row `ses_01kv7z4pdz0ceefjt723rk0kvy` (note: metadata serializes to the
  `config` jsonb column, not `metadata`):
  `config->>'container_name' = ubuntu-external-01kv7z4pdz0ceefjt723rk0kvy`,
  `config->>'external_agent_status' = running`, `dev_container_id` set. Stable on
  re-poll. Pre-fix, the trailing `UpdateSession(*session)` would have left both
  blank → `useSandboxState` maps empty `container_name` to `absent` → "paused".

## Notes / Learnings

- Two code paths perform read-modify-write on the same session row; the inner
  (`StartDesktop`) reloads fresh, the outer (`StartExternalAgentSession`) held a
  stale copy. When a callee persists to a row, the caller must re-read before
  writing again — never re-save a struct captured before the callee ran.
- Frontend status mapping lives in `useSandboxState`; empty `container_name`
  deterministically means `absent`/paused. That contract is correct — the bug is
  purely backend metadata loss.
