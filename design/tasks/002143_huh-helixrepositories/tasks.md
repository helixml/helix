# Implementation Tasks: Prevent git config Lock Contention During Parallel Clone

- [ ] In `desktop/shared/helix-workspace-setup.sh`, change `git clone ... &` to a blocking call with inline success/failure handling and `exit 1` on failure
- [ ] Remove the `CLONE_PIDS`, `CLONE_NAMES`, `CLONE_DIRS` arrays and the post-loop `wait` block (no longer needed when clones are sequential)
- [ ] Test with a project that has ≥ 3 repositories and confirm setup completes without a `gitconfig` lock error
