# Implementation Tasks

## Phase 1 — Evidence
- [ ] Reply to User Y: ask for spec task id, GitHub OAuth account vs task-starter account, and `[GitPush]`/`Receive-pack` API log lines
- [ ] If logs are pasted, classify which candidate root cause (#1–#5 in design.md) is firing

## Phase 2 — Local repro
- [ ] In inner Helix at `http://localhost:8080`: register, complete onboarding, connect GitHub OAuth in Settings
- [ ] Create test project pointing at a small real GitHub repo we control
- [ ] Start a spec task; capture `[GitPush]` and `Receive-pack` API logs while the agent pushes
- [ ] Document exact repro steps in `design.md` (or `repro.md` next to it)

## Phase 3 — Make the failure visible (smallest viable surface)
- [ ] Add `LastPushError string` and `LastPushErrorAt *time.Time` to `types.GitRepository` (GORM AutoMigrate handles the column)
- [ ] Add `UpdatePushStatus(ctx, repoID, err)` on `GitRepositoryService` — clears on `nil`, stores classified message otherwise
- [ ] Strip embedded credentials (`://x-access-token:...@`) from error strings before persisting (use existing `stripCredentialsFromURL` pattern)
- [ ] Add `classifyAuthError(err) string` helper: prepend hint for 401/403/Bad credentials/Resource not accessible/empty creds
- [ ] In `git_http_server.go::handleReceivePack`, call `UpdatePushStatus` after the per-branch upstream push loop (success or failure)
- [ ] Verify `GET /api/v1/git/repositories/{id}` returns the new fields (likely automatic via struct serialisation — confirm)

## Phase 4 — Fix the root cause (branch on what Phase 1+2 found)
- [ ] **If #1/#3 (no OAuth on phase-chain user):** in the credential resolver, fall back to project owner / org owner GitHub OAuth before falling through to repo PAT
- [ ] **If #2 (wrong external type):** fix the project-creation handler to set `ExternalRepositoryType=GitHub` and write `OAuthConnectionID` on the repo row when "Connect via OAuth" UI flow is used
- [ ] **If #4 (expired token):** scope this task to surfacing it — token refresh is a separate task; document the boundary
- [ ] **If something else surfaces:** update design.md before coding

## Phase 5 — Verify end-to-end (mandatory per CLAUDE.md)
- [ ] Re-run Phase 2 repro with the fix applied; confirm commits arrive on GitHub feature branch with no manual intervention
- [ ] Negative test: revoke the GitHub token mid-flow, attempt another push, confirm `last_push_error` is set with an actionable message
- [ ] Regression test: PAT-only repo (no OAuth) still pushes successfully
- [ ] Regression test: Helix-internal repos (no `ExternalURL`) unaffected — no upstream push attempted
- [ ] Frontend smoke: confirm the existing repo/project pages don't crash on the new fields (they may just be ignored, which is fine for this task)

## Phase 6 — Ship it
- [ ] PR description references this spec task and the candidate root cause it addressed
- [ ] CI green (Drone) before merge
- [ ] Update User Y on the bug thread with what was found and the fix
