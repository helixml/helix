# Requirements: Fix Build Cache Progress Reporting Bugs

## Problem

Two bugs in the golden cache copy progress reporting during session startup:

1. **Parallel sessions share the same progress**: When two spec task sessions for the same project start simultaneously, they both see identical status updates (e.g., "Unpacking build cache (2.1/7.0 GB)"). If one finishes first, the other's progress display disappears or shows stale data.

2. **Progress starts at 100% then drops to 0%**: The progress briefly shows the full size before resetting and counting up properly.

## Root Cause Analysis

### Bug 1: Shared progress (project-level keying)

The golden copy progress is stored in `goldenCopyProgress map[string]*GoldenCopyProgress`, keyed by **project ID**. When two sessions for the same project start concurrently:

- Both call `dm.setGoldenCopyProgress(req.ProjectID, copied, total, false)` — writing to the **same map key**.
- The polling side (`GetGoldenCopyProgress`) returns a single entry for the project, so both sessions' progress goroutines read the same (interleaved) values.
- When the first copy finishes, it calls `dm.setGoldenCopyProgress(req.ProjectID, 0, 0, true)` which **deletes** the entry — killing progress reporting for the still-running second copy.

### Bug 2: Progress starts at 100%

In `SetupGoldenCopy()` (golden.go), the progress monitor uses `du -sb dockerDir` to measure copied bytes. But `du -sb` reports the size of whatever is already at `dockerDir`. The `parallelCopyDir` function creates subdirectories (like `overlay2/`) and starts copying into them via concurrent `cp -a` processes. On the very first `du` poll (before `parallelCopyDir` begins its work but after `os.MkdirAll` or early directory creation), the reported size can be misleading. More critically, the initial `onProgress(0, goldenSize)` call at line 253 sets `CopiedBytes=0`, but there's a race: the API-side polling goroutine starts 2 seconds after `CreateDevContainer` is called, and if `SetupGoldenCopy` has already reported `onProgress(goldenSize, goldenSize)` (the 100% final report at line 300) from a **previous** session's stale progress entry (bug 1 interaction), or the Hydra progress map still has a stale entry from a prior invocation that finished, the first poll picks up 100%.

The simpler explanation: the initial `onProgress(0, goldenSize)` call sets `{CopiedBytes: 0, TotalBytes: goldenSize}` in the map, but the API-side poller in `hydra_executor.go` may have already polled and cached a previous result. Additionally, if two sessions race, session B's poller can see session A's final 100% progress before session A clears it.

## User Stories

1. **As a user starting multiple spec task sessions in parallel**, I want each session to show its own independent build cache progress, so I can track each session's startup separately.

2. **As a user starting a session**, I want the progress to start at 0% and count up monotonically, so I get an accurate picture of how far along the cache unpacking is.

## Acceptance Criteria

- [ ] Each session's progress is tracked independently — two sessions for the same project show their own copy progress
- [ ] Progress begins at 0 bytes and increases monotonically until completion
- [ ] When one session's copy finishes, it does not affect another session's progress display
- [ ] The status message is cleared when the copy completes (existing behavior preserved)
- [ ] No regressions: single-session progress still works correctly
- [ ] `go build ./pkg/hydra/ ./pkg/external-agent/` compiles cleanly