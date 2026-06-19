# Design: Remove Stale .gitconfig.lock Before Git Setup

## Root Cause

`git config --global` creates `~/.gitconfig.lock`, writes the new config, then renames the lock file to `~/.gitconfig`. If the process is killed between create and rename, the lock file persists. Git refuses to write again while the lock exists.

## Fix

In `desktop/shared/helix-workspace-setup.sh`, add a single line immediately before the Git Configuration section (around line 198):

```bash
rm -f ~/.gitconfig.lock
```

The `-f` flag makes this a no-op when the lock file doesn't exist, so there is no need for an existence check.

## Location

File: `helix/desktop/shared/helix-workspace-setup.sh`
Insert before: line 198 (`# Git Configuration` comment block)

## Why Not a Broader Fix

This is the only place in the setup script that writes to `~/.gitconfig`. A targeted one-liner is sufficient — no retry logic or locking abstraction is needed.
