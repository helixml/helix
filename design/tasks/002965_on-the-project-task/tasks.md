# Implementation Tasks: Show Only Active Branches When Creating Project Tasks

- [ ] Add an opt-in active-only mode to the repository branches API while preserving the current all-branches response by default.
- [ ] Filter default, `helix-specs`, known unchanged merged, and upstream-deleted branch refs in the backend.
- [ ] Correlate current branch tips with Helix task and provider pull-request merge records.
- [ ] Keep fresh branches and branches without affirmative merge evidence visible, including branches currently equal to the default tip.
- [ ] Make a merged branch active again when later commits change its tip.
- [ ] Update the project task creation form to request active branches from the primary repository.
- [ ] Add backend tests for fresh-at-default, active, known merged, reactivated, unknown-merge-state, deleted, default, and local-only branch cases.
- [ ] Add frontend tests for the request, sorted options, empty state, and errors.
- [ ] Regenerate API documentation/client artifacts if the API query contract requires it.
- [ ] Run targeted backend and frontend test suites.
