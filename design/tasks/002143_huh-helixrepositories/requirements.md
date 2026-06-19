# Requirements: Remove Stale .gitconfig.lock Before Git Setup

## Problem

When a previous workspace setup run is interrupted (user closes the window, process killed, etc.), git leaves behind a `.gitconfig.lock` file at `~/.gitconfig.lock`. On the next setup attempt, `helix-workspace-setup.sh` calls `git config --global` and fails immediately:

```
error: could not lock config file /home/retro/.gitconfig: File exists
```

Setup exits with code 255 and the session never starts.

## User Story

As a Helix user whose workspace setup was previously interrupted, I want setup to succeed on retry without requiring me to manually delete a lock file.

## Acceptance Criteria

- `helix-workspace-setup.sh` removes `~/.gitconfig.lock` before the Git Configuration section runs
- If no lock file exists, setup behaves exactly as before (no change in behaviour)
- If a lock file exists, it is silently removed and git config proceeds successfully
- The fix applies before the first `git config --global` call so all git config commands benefit
