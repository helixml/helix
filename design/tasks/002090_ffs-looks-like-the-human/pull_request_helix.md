# fix(api): reuse project's existing exploratory session in activation

## Summary

The "Resume Human Desktop" button could leave the project stuck on
**Resume Human Desktop / stopped** even after a successful resume — the
container was running, but the project status pill kept reading the
opposite.

Root cause: `StartExternalAgentSession` unconditionally minted a fresh
`ses_…` id whenever the helix-org spawner activated a worker, even
when the project already had an exploratory session row from a prior
`startExploratorySession` call. Two rows for the same
`(project_id, session_role="exploratory")` then disagreed —
`GetProjectExploratorySession`'s `ORDER BY created DESC LIMIT 1`
picked the newer row, while the resumed desktop was keyed off the
older row's id in hydra's `h.sessions` map. The status check
(`externalAgentExecutor.GetSession(latest.ID)`) saw nothing, so the
UI reported `stopped`.

Fix: in `StartExternalAgentSession`, when
`req.SessionRole == "exploratory" && req.ProjectID != ""`, pre-resolve
the session via `GetProjectExploratorySession` and reuse the existing
row (preserving owner/org/parent_app/metadata). Fall back to minting
a fresh id when no row exists. The downstream
`appendOrOverwrite → WriteSession → WriteInteractions → StartDesktop`
chain is unchanged — `WriteSession` already calls `Store.UpdateSession`
which is a GORM `Save` (upsert by PK), and `CreateInteractions`
already uses `OnConflict{UpdateAll:true}`, so one code path handles
both the reused and fresh cases.

## Changes

- `api/pkg/server/session_handlers.go`: ~15-line guard at the top of
  `StartExternalAgentSession` that pre-resolves the session id from
  `GetProjectExploratorySession` when the request shape is
  exploratory + project-scoped. Comment links to the spec for the
  full bug history.
- `api/pkg/server/exploratory_session_activation_test.go` (new):
  suite-based regression cover. Four cases:
  1. **Regression gate** — pre-seeded exploratory row + worker-activation-
     shape `StartExternalAgentSession` call must return the existing
     session id and invoke `StartDesktop` with that id. **Fails on
     main**, passes here.
  2. **Status pill** — `getProjectExploratorySession` reports
     `external_agent_status="stopped"` when the hydra map is empty,
     `"running"` when populated. Pins the read side the UI stares at.
  3. **No project → no reuse** — `SessionRole="exploratory"` with empty
     `ProjectID` still mints a fresh id (no
     `GetProjectExploratorySession` call). Confirms we didn't make
     every exploratory session globally singleton.
  4. **Different role → no reuse** —
     `SessionRole="planning"` with the same `ProjectID` still mints
     a fresh id. Confirms the guard is role-gated.

## Test Plan

- [x] `CGO_ENABLED=1 go test -run TestExploratorySessionActivationSuite ./api/pkg/server/`
      — all four cases pass.
- [x] `CGO_ENABLED=1 go test ./api/pkg/server/` — full server suite green.
- [x] `CGO_ENABLED=1 go test ./api/pkg/org/infrastructure/runtime/helix/...`
      — org spawner suite green.
- [ ] Reviewer manual check (deferred from this PR): on inner Helix,
      register a user → create a project → click "Open Human Desktop"
      → hire an AI Worker → wait for activation → reload SpecTasks →
      confirm "View Human Desktop / running" (not "Resume / stopped").
      DB cross-check:
      ```sql
      SELECT COUNT(*) FROM sessions
      WHERE config->>'project_id' = '<projectID>'
        AND config->>'session_role' = 'exploratory';
      ```
      Must return `1`, not `2`.

## Spec

helix-specs@`002090_ffs-looks-like-the-human`
