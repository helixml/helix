# fix(desktop): ensure 'main' is the default branch on new GitHub repos

## Summary

When a project was connected to a brand-new, empty GitHub repository, Helix
could end up with `helix-specs` as the repo's **default branch** instead of
`main`. GitHub auto-promotes the first branch pushed to an empty repo, and the
desktop session-startup scripts could push (or leave only) `helix-specs` first.
A non-`main` default branch also breaks the design-docs `git worktree` step on
subsequent runs.

The desktop scripts already *tried* to seed the default branch first, but that
seed was best-effort: if it failed or was skipped, `helix-specs` was still
pushed and became the upstream default. This change makes seeding the default
branch **authoritative** — `helix-specs` is never pushed to an empty upstream
unless the default branch has landed there first.

## Changes

- `desktop/shared/helix-specs-create.sh` (`create_helix_specs_branch`):
  - Normalize `RETURN_BRANCH` to `main` when it would otherwise be empty.
  - For empty repos, abort before creating/pushing `helix-specs` if the
    default-branch seed push fails (return non-zero, set
    `HELIX_SPECS_BRANCH_EXISTS=false`, restore any stash).
  - After pushing `helix-specs` on an empty repo, verify via
    `git ls-remote --symref origin HEAD` that the upstream default did not become
    `helix-specs`, and warn if it did.
- `desktop/shared/helix-workspace-setup.sh`: the empty-repo init now hard-fails
  (`exit 1`) if `git push -u origin main` fails, instead of logging and
  continuing (which previously let `helix-specs` win the default-branch slot).
- `desktop/shared/test-helix-specs-creation.sh`: assert the remote default
  branch is never `helix-specs` in every case, and add an "empty repo with
  failing seed push" test. Suite now 12/12 passing.

## Testing

- `bash desktop/shared/test-helix-specs-creation.sh` → 12 passed, 0 failed.
- Not verified end-to-end against a live empty GitHub repo (requires real
  GitHub OAuth credentials unavailable in the dev sandbox) — please confirm
  before merge.
