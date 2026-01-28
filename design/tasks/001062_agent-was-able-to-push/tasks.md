# Implementation Tasks

## Pre-Receive Hook Update

- [x] Update `PreReceiveHookVersion` to `"2"` in `api/pkg/services/gitea_git_helpers.go`
- [x] Modify `preReceiveHookScript` to read `HELIX_ALLOWED_BRANCHES` env var
- [x] Add branch restriction check logic to hook (reject if branch not in allowed list)
- [x] Ensure clear error message format: include rejected branch and allowed branches

## HTTP Handler Changes

- [x] In `handleReceivePack()`: Move `getBranchRestrictionForAPIKey()` call BEFORE `cmd.Run()`
- [x] If agent has restrictions, add `HELIX_ALLOWED_BRANCHES=branch1,branch2` to `environ` slice
- [x] Remove post-receive branch restriction check and rollback logic (lines ~565-593)
- [x] Keep upstream push failure rollback (that's a different code path)

## Testing

- [ ] Test: Agent push to unauthorized branch (e.g., `main`) → receives rejection error
- [ ] Test: Agent push to allowed branch (e.g., `helix-specs`) → succeeds
- [ ] Test: Agent push to assigned feature branch → succeeds  
- [ ] Test: Normal user push to any branch → succeeds (no restrictions)
- [ ] Test: Force-push to `helix-specs` still rejected (existing protection)

## Verification

- [ ] Check hook auto-updates on API startup (version bump triggers reinstall)
- [ ] Verify error message is visible in git client output
- [ ] Confirm no rollback log entries for branch restriction violations