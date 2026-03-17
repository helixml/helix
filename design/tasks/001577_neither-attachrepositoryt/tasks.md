# Implementation Tasks

Tests go in `api/pkg/server/project_handlers_test.go` using the suite pattern (see `access_grant_handlers_test.go` as a reference). Add a `ProjectRepositoryHandlersSuite` that mocks the store with `store.NewMockStore`.

## Step 1: Write failing (red) tests

- [~] Write all 6 red tests in `project_handlers_test.go`
- [ ] Confirm all 6 tests fail (red) before touching handler code

## Step 2: Implement

- [x] Confirmed `SetProjectPrimaryRepository` accepts `""` (GORM UPDATE passes value directly)
- [ ] Update `attachRepositoryToProject` handler: after successful attach, set `default_repo_id` if currently empty or stale
- [ ] Update `detachRepositoryFromProject` handler: after successful detach, update or clear `default_repo_id` if the detached repo was the default

## Step 3: Verify

- [ ] Confirm all 6 tests pass (green)
