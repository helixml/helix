# Implementation Tasks

Tests go in `api/pkg/server/project_handlers_test.go` using the suite pattern (see `access_grant_handlers_test.go` as a reference). Add a `ProjectRepositoryHandlersSuite` that mocks the store with `store.NewMockStore`.

## Step 1: Write failing (red) tests

- [x] Write all 6 tests in `project_handlers_test.go`
- [x] Confirm red: 5/6 fail (KeepsDefaultWhenNotDefault already passes — correct, handler already skips SetProjectPrimaryRepository for non-default detach)

## Step 2: Implement

- [x] Confirmed `SetProjectPrimaryRepository` accepts `""` (GORM UPDATE passes value directly)
- [~] Update `attachRepositoryToProject` handler: after successful attach, set `default_repo_id` if currently empty or stale
- [ ] Update `detachRepositoryFromProject` handler: after successful detach, update or clear `default_repo_id` if the detached repo was the default

## Step 3: Verify

- [ ] Confirm all 6 tests pass (green)
