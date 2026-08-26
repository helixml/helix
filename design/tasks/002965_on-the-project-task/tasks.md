# Implementation Tasks: Show Only Active Branches When Creating Project Tasks

- [x] Update the repository branches API to return active branches using its existing response shape.
- [~] Filter default, `helix-specs`, known unchanged merged, and upstream-deleted branch refs in the backend.
- [ ] Correlate current branch tips with stored Helix task merge records.
- [ ] Keep fresh branches and branches without affirmative merge evidence visible, including branches currently equal to the default tip.
- [ ] Make a merged branch active again when later commits change its tip.
- [ ] Verify the project task creation form consumes the filtered branch response without changing its current submission behavior.
- [ ] Add backend tests for fresh-at-default, active, known merged, reactivated, unknown-merge-state, deleted, default, and local-only branch cases.
- [ ] Add frontend tests for the request, sorted options, empty state, and errors.
- [ ] Regenerate API documentation/client artifacts if the API query contract requires it.
- [ ] Run targeted backend and frontend test suites.
