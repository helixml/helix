# Requirements: Remove Terminal Auto-Close Countdown Prompt from Workspace Setup

## Background

`desktop/shared/helix-workspace-setup.sh` runs inside a ghostty terminal window at session start. It clones repos, checks out branches, and runs the project startup script. When the script exits (success or failure), an EXIT trap fires `cleanup_and_prompt`, which shows:

```
What would you like to do?
  1) Close this window
  2) Start an interactive shell for debugging

Enter choice [1-2] (auto-close in 60s):
```

This prompt appears on **every** run — including successful ones. Commit `389435bb1` added the 60-second countdown so unattended spec-task agents don't block forever on `read`.

The user finds the countdown prompt disruptive and wants it removed.

## User Stories

**As a developer watching the startup terminal**, I want the terminal to behave naturally after setup completes without demanding I press 1 or 2 within 60 seconds, so I can read the output at my own pace.

**As a developer whose startup script failed**, I want the error details surfaced in the Helix UI without the terminal hanging on an interactive prompt, so failed sessions are diagnosed automatically.

## Acceptance Criteria

1. After a **successful** run of `helix-workspace-setup.sh`, no "press 1 or 2" menu appears.
2. After a **failed** run, the terminal exits non-zero without blocking on `read`.
3. The `~/.helix-setup-failed` JSON sentinel is still written on failure (the API reads it to surface the real error in the UI — this must be preserved).
4. The `~/.helix-setup.log` tee is still written on both success and failure (useful for debugging).
5. No regression: unattended spec-task sessions whose setup fails do not hang forever — they exit promptly and let the API observe the failure.
