# Implementation Tasks: Reinstate Role/Identity File Republish on Worker Activation

- [x] In `api/pkg/org/infrastructure/runtime/helix/project.go`, add a call to `a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent, worker.IdentityContent())` inside the fast-path `else` block, immediately before `return state.ProjectID, state.AgentAppID, state.RepoID, nil`.
- [x] Delete (or rewrite) the comment block at `project.go:223-229` that currently explains why files are NOT republished on the fast path — the rationale no longer applies and leaving stale doc lies about the code.
- [~] In `api/pkg/org/infrastructure/runtime/helix/project_test.go`, flip the assertions in `TestEnsureWithPersistedProjectFastPaths` at lines 418-425: branch creation MUST happen, and `role.md` MUST appear in `git.putFileByPath` with the seeded role content (`# Role v1`).
- [~] Add new test `TestEnsureFastPathPropagatesRoleEdits` in the same file: persist a project, call `Ensure`, mutate the role content in the store, reset `git.putFileByPath`, call `Ensure` again, assert the second push carries the new role content.
- [ ] Run `go build ./api/pkg/org/...` to confirm no compile breakage.
- [ ] Run `go test ./api/pkg/org/infrastructure/runtime/helix/...` and confirm the updated test passes alongside `TestEnsureFastPathRefreshesAgentSpec`, `TestEnsureRolePropagatesFromFirstPosition`, `TestEnsureSkipsRolePushIfRoleMissing`, and `TestEnsureLogsButDoesNotFailOnPutFileError`.
- [ ] Manual end-to-end check against the helix-org dev stack: hire a Worker, edit its role content directly in the DB (bypass `update_role`), activate it, confirm the helix-specs branch's `workers/<id>/.context/role.md` matches the new DB content after activation completes.
- [ ] Open the PR. Title: `fix(api/org/runtime/helix): republish role/identity on every activation`. PR body should reference both `4a6cb33c51` (original feature) and `4f7837ac0c` (where it regressed) so reviewers see the full history.
