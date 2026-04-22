# Implementation Tasks

## Fix push credential resolution
- [x] In `git_http_server.go:622`, when resolving push credentials for agent pushes, fall back to `task.SpecApprovedBy` when `task.ImplementationApprovedBy` is empty. Currently only uses `ImplementationApprovedBy` which is empty during the implementation phase.

## Add `git` to desktop exec whitelist
- [x] In `api/pkg/desktop/exec.go:55-68`, add `"git": true` to the allowed commands map so we can exec `git config` in running containers.

## Add exec-in-desktop callback to SpecDrivenTaskService
- [x] Add an `ExecInDesktop func(ctx, sessionID, command) error` callback field to `SpecDrivenTaskService`. Wire it up in the server using connman + RevDial (same pattern as `execInSandbox` handler).

## Update git identity in container on transition to implementation
- [x] In `ApproveSpecs()`, after setting status to implementation, use the exec callback to run `git config --global user.name "X"` and `git config --global user.email "Y"` in the running container, using the approver's identity from `task.SpecApprovedBy`.

## Validate OAuth at spec approval time
- [x] In `approveSpecs` handler (`spec_driven_task_handlers.go`), validate the approving user has GitHub OAuth before transitioning to `spec_approved` — same pattern as `approveImplementation` in `spec_task_workflow_handlers.go:141-152`. Return `oauth_required` error if missing and repo is GitHub-hosted.

## Testing
- [~] Verify `go build` passes for all affected packages
- [ ] Manually test end-to-end: User 1 creates task, User 2 approves specs, verify commits are authored as User 2
