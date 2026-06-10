# Implementation Tasks: Populate ProjectID and RepositoryIDs when Helix-OR worker starts its first session

- [ ] Add `attachProjectContext(ctx, agent, projectID)` helper on `*HelixAPIServer` in `api/pkg/server/session_handlers.go` that sets `ProjectID`, `RepositoryIDs`, and `PrimaryRepositoryID` (mirroring the existing inline blocks).
- [ ] In `StartExternalAgentSession` (`session_handlers.go` ~line 2474), call `s.attachProjectContext(ctx, zedAgent, session.ProjectID)` after constructing the `DesktopAgent`. Return the wrapped error on failure.
- [ ] Replace the inline `ListGitRepositories` + `GetProject` block in `spec_task_design_review_handlers.go:967-983` with a call to the helper.
- [ ] Replace the inline `ListGitRepositories` block in `session_handlers.go:1991-2007` with a call to the helper (keep the separate `GetProject` error-check above it — that path returns 500 on missing project and is orthogonal).
- [ ] Run `go build ./api/pkg/server/...` to confirm the package still compiles.
- [ ] Add or extend a unit test that exercises `StartExternalAgentSession` against a project with attached repos and asserts the resulting `DesktopAgent` (or the recorded `StartDesktop` call) has non-empty `RepositoryIDs` and the project's `DefaultRepoID` as `PrimaryRepositoryID`. Pattern: see `api/pkg/server/helix_org_inproc_test.go` and existing session-handler tests.
- [ ] Manual verification in the inner Helix: hire a worker on a project with at least one attached git repo, watch the new container start, and confirm `helix-workspace-setup.sh` clones the repo and Zed launches (i.e. `/home/retro/.helix-setup-failed` does **not** appear, `start-zed-helix.sh` exits the waiting loop).
- [ ] Commit using conventional format, e.g. `fix(api): populate project context when worker starts first session`. Push and verify CI is green via `gh pr checks` / Drone MCP tools.
