# Implementation Tasks: Show Only Active Branches When Creating Project Tasks

- [x] Update the repository branches API to return active branches using its existing response shape.
- [x] Filter default, `helix-specs`, known unchanged merged, and upstream-deleted branch refs in the backend.
- [x] Correlate current branch tips with stored Helix task merge records.
- [x] Keep fresh branches and branches without affirmative merge evidence visible, including branches currently equal to the default tip.
- [x] Make a merged branch active again when later commits change its tip.
- [x] Verify the project task creation form consumes the filtered branch response without changing its current submission behavior.
- [x] Add backend tests for fresh-at-default, active, known merged, reactivated, unknown-merge-state, deleted, default, and local-only branch cases.
- [x] Verify the existing frontend branch-selector test still passes with the unchanged API contract.
- [x] Regenerate API documentation/client artifacts if the API query contract requires it (not required; response type is unchanged).
- [x] Run targeted backend and frontend test suites.
- [x] Replace Helix project/task merge lookup with repository-provider merged pull-request evidence.
- [x] Re-run tests, merge current main, and push the corrected implementation.
- [x] Remove provider API merge-history dependencies and implement local merge-parent detection.
- [x] Add local Git tests for fresh, merged, advanced, deleted, and uncertain branches.
- [x] Re-run tests, merge current main, and push the clean local-only implementation.
