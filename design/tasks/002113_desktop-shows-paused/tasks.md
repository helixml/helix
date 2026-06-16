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
- [ ] End-to-end verify in inner Helix: start a session via the external-agent
      path and confirm the desktop viewer shows "running" (not "paused"); confirm
      the DB row keeps `container_name` and `external_agent_status`.
      **NOT DONE — environment limitation:** only `helix-registry-1` and
      `helix-sandbox-nvidia-1` are running; the inner Helix API, Postgres and
      frontend are down (`http://localhost:8080` returns 000), so this path
      cannot be exercised here. Behaviour is covered instead by the deterministic
      `StartExternalAgentPausedSuite` regression test, which was verified to FAIL
      against the pre-fix code and PASS after. A reviewer with a live stack should
      confirm via: trigger a Worker/cron session, then
      `SELECT metadata->>'container_name', metadata->>'external_agent_status'
      FROM sessions WHERE id='<ses_…>';`.
