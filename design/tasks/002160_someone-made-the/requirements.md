# Requirements: Restore Blocking Workspace Setup Terminal Prompt (Remove 60s Auto-Close)

## Background

`desktop/shared/helix-workspace-setup.sh` runs inside a ghostty terminal at session start. It clones repos, checks out branches, and runs the project startup script. When it exits (success or failure), an EXIT trap fires `cleanup_and_prompt`, which shows:

```
What would you like to do?
  1) Close this window
  2) Start an interactive shell for debugging

Enter choice [1-2] (auto-close in 60s):
```

Commit `389435bb1` ("fix(desktop): surface workspace-setup failures instead of hanging cold-start") changed this prompt from a **blocking** `read -p` (which waited indefinitely, default → open a shell) to a **timed** `read -t 60` that auto-closes the window after 60 seconds.

The user dislikes the auto-close: they often want to check whether the stack actually started up correctly, and 60 seconds isn't enough — the window closes out from under them.

## What the User Asked For

"Revert the change that times out the terminal" — restore the terminal so it stays open for inspection. The user described this as "just revert that one commit," assuming the whole change lived in a single commit.

## Investigation Finding (informs the design)

Commit `389435bb1` bundles **two unrelated concerns**:

1. **The timeout** on the menu's `read` (the part the user wants gone), plus a change of the default from "open a shell" to "close the window."
2. **An error-surfacing feature** — writing a `~/.helix-setup-failed` JSON sentinel + a `~/.helix-setup.log` tee in the script, and Go code in `api/pkg/server/auto_wake_stuck_interactions.go` that reads the sentinel to show the real failure reason in the UI instead of a generic "agent never connected" banner.

A literal full `git revert 389435bb1` does **not** apply cleanly: the Go file conflicts because a later commit (`ac9be34b7`) refactored the affected function. The full revert would also throw away the error-surfacing feature, which is unrelated to the timeout complaint.

Therefore the design targets **only the timeout** (a small change confined to the shell script) and leaves the error-surfacing feature intact.

## Acceptance Criteria

1. After setup runs, the terminal prompt **no longer auto-closes** — it waits for the user, however long they take to inspect the stack.
2. The menu still offers: 1) close the window, 2) open an interactive shell for debugging. The default (pressing Enter, or any non-`1` input) **opens a shell**, restoring the pre-`389435bb1` behavior so the window stays open.
3. The error-surfacing feature is **preserved**: the `~/.helix-setup-failed` sentinel and `~/.helix-setup.log` tee are still written, and `auto_wake_stuck_interactions.go` is left unchanged.
