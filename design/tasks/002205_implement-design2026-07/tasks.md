# Implementation Tasks: Project VCS Connection Lozenge & Loud Push-Failure Surfacing

## Workstream A — Loud push-failure surfacing (ship first, independent)

- [ ] Add `PushError` struct and `LastPushError *PushError` field to `SpecTask` in `api/pkg/types/simple_spec_task.go` (GORM AutoMigrate, json serializer)
- [ ] Populate and persist `PushError` at the mirror-push failure/rollback site in `api/pkg/services/git_http_server.go:687-691`, using the already-resolved acting user's connection for the account handle
- [ ] Clear `LastPushError` on the next successful push
- [ ] Gate the task "pushed/ready for review" signal on absence of `LastPushError` + a verified ref (stop confabulating success from the pre-mirror 200)
- [ ] Add a provider error-translation mapper (raw `Repository not found` → cause + next step referencing account, repo, switch action)
- [ ] Move the task to an explicit error state on push failure and surface the translated cause + next step on the task card/detail
- [ ] Regenerate OpenAPI (`./stack update_openapi`) and consume the new field via the generated frontend client
- [ ] Go unit tests: `PushError` write on failure, error clears on success, translate mapper output
- [ ] E2E in inner Helix: spec task against an unreachable external repo shows the error state and message; agent does NOT report success

## Workstream B — Provider capability abstraction (generic core)

- [ ] Add `VCSProviderCapability` (Type, AuthMechanism, RequiredScopes, VerifyAccess, IdentityHandle, TranslateError) and a registry keyed by `types.ExternalRepositoryType`
- [ ] Implement the GitHub capability entry (scopes `repo,read:user,user:email,read:org`; verify via `GET /repos/{o}/{r}`; identity from `OAuthConnection.ProviderUsername`)
- [ ] Wire verify probes through `oauth.Provider.MakeAuthorizedRequest`
- [ ] Stub/placeholder capability entries for GitLab / ADO / Bitbucket (verify probe + scopes) so adding them needs no shared-component change

## Workstream B — Scopes & forced account selection

- [ ] Source requested connect scopes from the capability registry; update `simple_sample_projects.go:731` to include `read:org` and stop storing `["repo"]` only
- [ ] Align/widen `GitHubRepoScopes` validation in `sample_project_access_handlers.go:257`
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
- [~] Apply at `stack:1098`, `stack:1139`, `stack:1176`
- [~] Apply at `sandbox/04-start-dockerd.sh:266`, `:285`
- [ ] Verify a fresh inner-Helix boot: pull output renders line-by-line, no mid-line truncation

## Workstream D — Warm desktop image via golden snapshot

- [ ] Read `api/pkg/services/golden_build_service.go` to confirm how far the golden build runs for this project type
- [ ] Extend the golden build to perform the desktop-image transfer (populate `sandbox-docker-storage` with `helix-ubuntu:<tag>`) before `PromoteSessionToGolden`/`PromoteSessionToGoldenZvol` (`api/pkg/hydra/golden.go`)
- [ ] Verify the skip-checks short-circuit on a warm session (`sandbox/04-start-dockerd.sh:220-224`; `stack:1074-1145`) — no re-transfer
- [ ] Verify cold first build still works and the non-ZFS / file-copy fallback path is unaffected
- [ ] Measure fresh-session startup before/after (target: eliminate the ~411 s transfer on warm sessions)

## Wrap-up

- [ ] `cd frontend && yarn build`; `go build ./pkg/...`; push and confirm CI green
- [ ] Update the design doc status and note any resolved open questions
