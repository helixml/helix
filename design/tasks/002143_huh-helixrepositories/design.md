# Design: Prevent git config Lock Contention During Parallel Clone

## Root Cause

`helix-workspace-setup.sh` launches all repository clones as background processes (line 291):

```bash
git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1 &
```

When multiple clones run concurrently they each invoke git's credential negotiation and/or auto-detection, which can write to `~/.gitconfig`. With five repos cloning in parallel, several processes attempt to acquire `~/.gitconfig.lock` simultaneously, and those that lose the race fail with a lock error.

## Fix: Run Clones Sequentially

Remove the `&` from the clone command and remove the parallel PID-tracking arrays. Each clone runs to completion before the next starts. The wait loop and CLONE_FAILED check remain unchanged so error reporting is unaffected.

Sequential clones are simpler and correct. The parallel optimisation was not load-bearing — typical clone times are dominated by network latency, and the number of repos is small (usually ≤ 5).

```bash
# Before (parallel)
git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1 &
CLONE_PIDS+=($!)

# After (sequential — clone blocks, no PID tracking needed)
if git clone "$GIT_CLONE_URL" "$CLONE_DIR" 2>&1; then
    echo "    ✅ $REPO_NAME cloned successfully"
else
    echo "    ❌ FAILED to clone $REPO_NAME"
    exit 1
fi
```

The PID arrays (`CLONE_PIDS`, `CLONE_NAMES`, `CLONE_DIRS`) and the post-loop `wait` section can be removed or simplified since success/failure is known immediately.

## Alternative Considered: flock Serialisation

Wrapping each git invocation with `flock` would preserve parallelism but adds complexity without meaningful benefit — the bottleneck is network I/O, not CPU, and the lock contention only manifests because of the parallel design. Sequential clones remove the problem entirely.

## Files to Change

- `desktop/shared/helix-workspace-setup.sh` — remove `&` from clone command, inline success/error handling, remove PID-tracking arrays and wait loop
