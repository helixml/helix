# Implementation Tasks

- [x] In `api/pkg/server/session_handlers.go` (~line 1869), replace the unconditional `projectRepos[0].ID` assignment with the `DefaultRepoID`-first pattern (ensure `project` is fetched from store if not already in scope)
- [x] Verify `project` struct is available at that point in session_handlers.go; if not, fetch it using the session's ProjectID before the primary repo selection block
- [ ] Manually test: create a project with multiple repos where `docs` was added most recently, set `helix-4` as primary, restart a session, confirm `HELIX_PRIMARY_REPO_NAME=helix-4`
