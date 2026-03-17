# Implementation Tasks

Tests go in `api/pkg/server/project_handlers_test.go` using the suite pattern (see `access_grant_handlers_test.go` as a reference). Add a `ProjectRepositoryHandlersSuite` that mocks the store with `store.NewMockStore`.

## Step 1: Write failing (red) tests

- [ ] Write test `TestAttachRepo_SetsDefaultWhenEmpty`: attach a repo to a project where `DefaultRepoID == ""` → expect `SetProjectPrimaryRepository` called with the new repo ID
- [ ] Write test `TestAttachRepo_SetsDefaultWhenStale`: attach a repo to a project where `DefaultRepoID` references a repo not in the current attached set → expect `SetProjectPrimaryRepository` called with the new repo ID
- [ ] Write test `TestAttachRepo_KeepsDefaultWhenValid`: attach a second repo when `DefaultRepoID` already references an attached repo → expect `SetProjectPrimaryRepository` NOT called
- [ ] Write test `TestDetachRepo_UpdatesDefaultToRemainingRepo`: detach the default repo when another repo is still attached → expect `SetProjectPrimaryRepository` called with the remaining repo ID
- [ ] Write test `TestDetachRepo_ClearsDefaultWhenLastRepo`: detach the default repo when it is the only attached repo → expect `SetProjectPrimaryRepository` called with `""`
- [ ] Write test `TestDetachRepo_KeepsDefaultWhenNotDefault`: detach a non-default repo → expect `SetProjectPrimaryRepository` NOT called
- [ ] Confirm all 6 tests fail (red) before touching handler code

## Step 2: Implement

- [ ] Check whether `SetProjectPrimaryRepository` accepts `""` to clear the field; if not, add that support or add a `ClearProjectPrimaryRepository` store method
- [ ] Update `attachRepositoryToProject` handler: after successful attach, set `default_repo_id` if currently empty or stale
- [ ] Update `detachRepositoryFromProject` handler: after successful detach, update or clear `default_repo_id` if the detached repo was the default

## Step 3: Verify

- [ ] Confirm all 6 tests pass (green)
