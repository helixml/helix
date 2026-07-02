# Implementation Tasks: Project VCS Connection Lozenge & Loud Push-Failure Surfacing

## Workstream A — Loud push-failure surfacing (ship first, independent)

- [x] Add `PushError` struct and `LastPushError *PushError` field to `SpecTask` in `api/pkg/types/simple_spec_task.go` (GORM AutoMigrate, json serializer)
- [x] Populate and persist `PushError` at the mirror-push failure/rollback site in `api/pkg/services/git_http_server.go` (`recordPushError`), resolving the acting user's account handle via new `GetActingAccountHandle`
- [x] Clear `LastPushError` on the next successful push (`clearPushError`)
- [x] Add a provider error-translation mapper `types.NewPushError` (raw `Repository not found`/403/401 → cause + next step referencing account, repo, switch action)
- [x] Go unit test for the translate mapper (`push_error_test.go`) — passing
- [~] Move the task to an explicit error state on push failure and surface the translated cause + next step on the task card/detail (data is now persisted on `SpecTask.LastPushError`; frontend surfacing pending — see Lozenge UI)
- [ ] Regenerate OpenAPI (`./stack update_openapi`) so the frontend client sees `last_push_error`
- [ ] E2E in inner Helix: spec task against an unreachable external repo shows the error state and message; agent does NOT report success

Note on "gate the ready-for-review signal": the persisted `LastPushError` is the
queryable signal the board/agent now reads. A push that fails is no longer silent
— the error is on the task. Fully rewiring the agent's completion message is a
larger cross-cutting change tracked under frontend surfacing; the backend truth
(error persisted, cleared on next success) is in place.

## Workstream B — Provider capability abstraction (generic core)

- [x] Add `vcs.Capability` (Type, Provider, AuthMechanism, RequiredScopes, AccessProbePath) and a registry keyed by `types.ExternalRepositoryType` — new package `api/pkg/vcs/capability.go`
- [x] Implement the GitHub capability entry (scopes `repo,read:user,user:email,read:org`; probe `GET /repos/{o}/{r}`; identity via `vcs.IdentityHandle` from `OAuthConnection.ProviderUsername`)
- [x] Stub capability entries for GitLab / ADO / Bitbucket (scopes + probe) so adding them needs no shared-component change
- [~] Wire verify probes through `oauth.Provider.MakeAuthorizedRequest` (probe URL builder `vcs.AccessProbeURL` in place; service-layer probe call pending — needed by board API + pre-flight)

## Workstream B — Scopes & forced account selection

- [x] Source requested connect scopes from the capability registry; `simple_sample_projects.go` now uses `vcs.RequiredScopes(github)` (includes `read:org`), stops hardcoding `["repo","read:user","user:email"]`
- [x] `GitHubRepoScopes` validation in `sample_project_access_handlers.go`: kept at operational minimum `repo` (documented) — widening the hard validation would reject existing connections; identity/org scopes degrade gracefully per design
- [ ] Force the provider account picker in Switch/Reconnect flows (pass account-selection param into `GetAuthorizationURL`)

## Workstream B — Board data API

- [ ] Add `GET /api/v1/projects/{id}/vcs-connections` returning one entry per distinct provider among the project's external repos, with state, acting_user, pushing_as, per-repo access, missing_scopes
- [ ] Compute presence from `distinct(repo.ExternalType) where repo.IsExternal`; compute state from the verify probe against the acting user's connection
- [ ] Swagger annotations + `./stack update_openapi`

## Workstream B — Pre-flight verify

- [ ] Run the verify probe before starting planning (and/or at repo-link time — resolve open question); on failure block the run with the switch-account prompt
- [ ] Confirm no silent fallback to repo-level/admin credential for user-initiated pushes (acting-user-first stays in `getCredentialsForRepo`)

## Workstream B — Lozenge UI

- [ ] Build the generic lozenge component (one per provider entry), rendered top-right in `SpecTaskKanbanBoard.tsx`, using the generated client + React Query
- [ ] Implement the three states (Verified / Needs attention / Disconnected) and "acting as X · pushing as @y" display
- [ ] Implement the menu: Switch account, Reconnect, Disconnect (unbind project only), Remove from my account, View on provider — reusing the existing OAuth connect flow
- [ ] E2E in inner Helix: verified lozenge for a reachable GitHub repo; no-access → needs_attention; switch forces account picker; provider-with-no-repos shows no lozenge

## Workstream C — Readable dev-startup pull output

- [x] Choose the approach: `grep --line-buffered -v "^$"` (minimal; keeps per-layer progress, flushes each line)
- [x] Apply at `stack:1098/1139/1176` (also fixed the `docker push` grep at `:1089` — same buffering)
- [x] `sandbox/04-start-dockerd.sh:266/285` — **no change needed**: those `docker pull` calls have no `grep` pipe (write directly, already line-flushed by docker), and piping them through `grep` would break the `if`/exit-code check that hard-fails the boot. The buffering artifact came only from the `stack` grep path.
- [ ] Verify a fresh inner-Helix boot: pull output renders line-by-line, no mid-line truncation (deferred — requires a fresh session; low-risk cosmetic change)

## Workstream D — Warm desktop image via golden snapshot

- [ ] Read `api/pkg/services/golden_build_service.go` to confirm how far the golden build runs for this project type
- [ ] Extend the golden build to perform the desktop-image transfer (populate `sandbox-docker-storage` with `helix-ubuntu:<tag>`) before `PromoteSessionToGolden`/`PromoteSessionToGoldenZvol` (`api/pkg/hydra/golden.go`)
- [ ] Verify the skip-checks short-circuit on a warm session (`sandbox/04-start-dockerd.sh:220-224`; `stack:1074-1145`) — no re-transfer
- [ ] Verify cold first build still works and the non-ZFS / file-copy fallback path is unaffected
- [ ] Measure fresh-session startup before/after (target: eliminate the ~411 s transfer on warm sessions)

## Wrap-up

- [ ] `cd frontend && yarn build`; `go build ./pkg/...`; push and confirm CI green
- [ ] Update the design doc status and note any resolved open questions
