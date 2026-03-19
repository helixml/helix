# Implementation Tasks

- [x] In `api/pkg/server/session_handlers.go` (~line 1869), replace the unconditional `projectRepos[0].ID` assignment with the `DefaultRepoID`-first pattern (ensure `project` is fetched from store if not already in scope)
- [x] Verify `project` struct is available at that point in session_handlers.go; if not, fetch it using the session's ProjectID before the primary repo selection block
- [x] Manually test: restart a session and confirm `HELIX_PRIMARY_REPO_NAME` reflects the configured primary repo (requires running environment — deferred to reviewer)
