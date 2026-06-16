# Implementation Tasks: Fix Desktop Showing "Paused" While Container Is Running

- [x] In `api/pkg/server/session_handlers.go`, replace the stale
      `UpdateSession(*session)` block (~lines 2548-2554) in
      `StartExternalAgentSession` with a re-fetch of the session row via
      `s.Store.GetSession(ctx, session.ID)`, assigning the result back to
      `session`.
- [x] Add an explanatory comment noting `StartDesktop` already persisted the
      container metadata and re-saving the stale struct caused the "paused" bug.
- [x] Add a regression test (mock store + mock executor) asserting that
      `container_name` and `external_agent_status="running"` survive
      `StartExternalAgentSession` and are not blanked.
      (`start_external_agent_paused_test.go`)
- [x] Build the API: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`.
- [x] Run the relevant Go test suite with CGO enabled
      (`CGO_ENABLED=1 go test -run <Suite> ./pkg/server/ -count=1`). New suite
      passes; verified it fails against the pre-fix code, then passes after.
      Related suites (Exploratory, AutoWakeColdStart, AttachProjectContext) green.
- [x] End-to-end verify in inner Helix: start a session via the external-agent
      path and confirm the DB row keeps `container_name` and
      `external_agent_status`. **DONE** — exercised the real
      `StartExternalAgentSession` path on `localhost:8080` via the cron-trigger
      execute endpoint (the exploratory/Human-Desktop UI path was ruled out: it
      calls `StartDesktop` directly and already re-fetches, so it never hit the
      bug). Result row `ses_01kv7z4pdz0ceefjt723rk0kvy`:
      `container_name = ubuntu-external-01kv7z4pdz0ceefjt723rk0kvy`,
      `external_agent_status = running` (stable on re-poll). Under the bug both
      would be blank → "paused". See design.md "End-to-end verification".
