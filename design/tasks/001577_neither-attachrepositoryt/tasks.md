# Implementation Tasks

- [ ] In `attachRepositoryToProject` handler: after successful attach, check if `project.DefaultRepoID` is empty or not in the current attached repo set; if so, call `SetProjectPrimaryRepository` with the newly attached repo ID
- [ ] In `detachRepositoryFromProject` handler: after successful detach, if `project.DefaultRepoID == repoID`, fetch remaining attached repos and call `SetProjectPrimaryRepository` with the first remaining repo or `""` if none remain
- [ ] Verify `SetProjectPrimaryRepository` handles empty string (`""`); if not, add support or use a direct DB update for the clear case
- [ ] Test: attach repo to project with no default → default_repo_id should be set
- [ ] Test: detach the default repo when another repo is attached → default_repo_id should update to remaining repo
- [ ] Test: detach the default repo when it is the only repo → default_repo_id should be cleared
- [ ] Test: detach a non-default repo → default_repo_id should remain unchanged
