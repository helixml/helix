# Requirements: Prevent git config Lock Contention During Container Startup

## Problem

When a task phase transition fires while the container is still running its startup setup, two processes attempt to write `~/.gitconfig` simultaneously:

1. `helix-workspace-setup.sh` — runs sequential `git config --global` calls (user.name, user.email, pull.rebase, credential.helper)
2. `syncGitIdentityToUser` in the API — fires `git config --global user.email` and `git config --global user.name` via `ExecInDesktop` on a task phase transition

One of the writers loses the lock and fails:

```
error: could not lock config file /home/retro/.gitconfig: File exists
```

Setup exits with code 255 and the session never starts.

## User Story

As a Helix user whose session restarts at the same time as a task phase transition, I want workspace setup to succeed without a `git config` lock conflict.

## Acceptance Criteria

- When `syncGitIdentityToUser` fires while `helix-workspace-setup.sh` is still running, it does not cause a lock conflict with the setup script's git config calls
- Setup succeeds reliably even when phase transitions occur concurrently
- If setup is still in progress, `syncGitIdentityToUser` either waits or defers gracefully
- No user-visible change when there is no contention
