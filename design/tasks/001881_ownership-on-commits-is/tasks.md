# Implementation Tasks

## Validate OAuth at spec approval time
- [ ] In `approveSpecs` handler (`api/pkg/server/spec_driven_task_handlers.go`), add GitHub OAuth validation for the approving user before transitioning to `spec_approved` status — same pattern as `approveImplementation` in `spec_task_workflow_handlers.go:141-152`. Return `oauth_required` error if missing.

## Update git identity in running container on transition to implementation
- [ ] Add an `ExecInContainer(ctx, sessionID, command []string)` method (or similar) to the `Executor` interface and `HydraExecutor` in `api/pkg/external-agent/` that can run a command inside a running container (Hydra API or `docker exec` equivalent)
- [ ] In `ApproveSpecs()` (`api/pkg/services/spec_driven_task_service.go:1118`), after setting status to implementation, call exec to run `git config --global user.name "ApproverName"` and `git config --global user.email "approver@email"` inside the running container. Fetch approver's user record from store using `task.SpecApprovedBy`.

## Update push credentials to use approver's OAuth
- [ ] Ensure that when the agent pushes branches during implementation, `task.SpecApprovedBy` is used as the `actingUserID` in `PushBranchToRemote()` calls. Trace the push path from the agent through the API to find where `userID` is (or isn't) being passed, and wire in the approver's ID.

## Update session API key ownership (if needed)
- [ ] Investigate whether the ephemeral session API key (minted for `task.CreatedBy` in `OnBeforeCreate`) affects push credential resolution. If pushes use the API key's owner to determine the acting user, mint a new key for the approver or add an override mechanism.

## Testing
- [ ] Add a unit test in `spec_driven_task_service_test.go` or `spec_driven_task_handlers_test.go` verifying that `approveSpecs` returns an OAuth error for a GitHub repo when the approving user lacks OAuth
- [ ] Add a unit test verifying that `ApproveSpecs` calls `ExecInContainer` with the correct git config commands using the approver's name and email
- [ ] Manually test end-to-end: User 1 creates task, User 2 approves specs, verify commits on the feature branch are authored as User 2
