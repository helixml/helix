# Implementation Tasks: Show Only Active Branches When Creating Project Tasks

- [ ] Add an opt-in active-only mode to the repository branches API while preserving the current all-branches response by default.
- [ ] Filter default, `helix-specs`, merged-tip, and upstream-deleted branch refs in the backend.
- [ ] Reuse Git ancestry checks so a merged branch with later commits is active again.
- [ ] Update the project task creation form to request active branches from the primary repository.
- [ ] Add backend tests for active, merged, reactivated, deleted, default, and local-only branch cases.
- [ ] Add frontend tests for the request, sorted options, empty state, and errors.
- [ ] Regenerate API documentation/client artifacts if the API query contract requires it.
- [ ] Run targeted backend and frontend test suites.
