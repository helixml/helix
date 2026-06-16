# Implementation Tasks: Ensure 'main' Is Pushed Before 'helix-specs' on New GitHub Repos

## Primary fix — `desktop/shared/helix-specs-create.sh` (`create_helix_specs_branch`)

- [~] Normalize `RETURN_BRANCH` for the empty-repo case so it is never empty and prefers the intended default (`main`, falling back to the detected upstream default / `master` only when that is the repo's convention).
- [~] Make the default-branch seed authoritative: for an empty repo, only push the `helix-specs` orphan **after** the default-branch seed push succeeds. If the seed fails, retry or skip helix-specs creation (and surface a clear error) so `helix-specs` is never the first/only branch on the empty upstream.
- [ ] (Optional) After seeding, verify the upstream default did not resolve to `helix-specs` (e.g. `git ls-remote --symref origin HEAD`) and log/fail if it did.

## Cleanup — `desktop/shared/helix-workspace-setup.sh`

- [ ] Consolidate empty-repo initialization to a single source of truth: remove the redundant empty-repo init block (~line 324) and rely on the now-authoritative `create_helix_specs_branch` seeding, or keep it but ensure it pushes `main` first and stays consistent. Verify no regression.

## Tests

- [ ] Extend `desktop/shared/test-helix-specs-creation.sh`: for the empty-repo case, assert the remote default branch is **not** `helix-specs` (inspect `origin/HEAD`), in addition to the existing `git worktree add helix-specs` check.
- [ ] Add a test where the default-branch seed push is forced to fail, asserting `helix-specs` is not pushed as the first/default branch.
- [ ] Run the existing 11-case `test-helix-specs-creation.sh` suite and confirm all pass.

## Verification

- [ ] Manually connect a brand new empty GitHub repo, run project setup multiple times, and confirm the GitHub default branch is `main` every time (no flakiness).
