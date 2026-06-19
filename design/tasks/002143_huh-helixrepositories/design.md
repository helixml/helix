# Design: Prevent git config Lock Contention During Container Startup

## Root Cause

Two concurrent processes write to `~/.gitconfig`:

- **`helix-workspace-setup.sh`**: sets `user.name`, `user.email`, `pull.rebase`, `credential.helper` sequentially at container startup
- **`syncGitIdentityToUser`** (`api/pkg/services/spec_driven_task_service.go:1548`): called by the API on task phase transitions, runs `git config --global user.email` then `git config --global user.name` via `ExecInDesktop`

These can overlap when a container restarts mid-task. The setup script already writes a signal file (`~/.helix-setup-complete`) when it finishes.

## Fix: Check Setup Signal Before Running git config in the API

In `syncGitIdentityToUser`, before calling `ExecInDesktop` for git config, check whether `~/.helix-setup-complete` exists. If it does not, the setup script is still running — skip the identity sync silently and return `nil`.

The setup script already sets the correct identity (it reads `GIT_USER_NAME` / `GIT_USER_EMAIL` env vars), so the API's sync is redundant during the startup window. The next legitimate phase transition after setup completes will sync the identity correctly.

```go
// Skip if the container setup hasn't finished yet — the setup script
// writes git identity itself and holds the .gitconfig lock.
checkSetup := []string{"test", "-f", "/home/retro/.helix-setup-complete"}
if err := s.ExecInDesktop(ctx, sessionID, checkSetup); err != nil {
    // File absent → setup still running; skip to avoid lock contention
    log.Info()...Msg("Container setup not complete, skipping git identity sync")
    return nil
}
```

Place this check at the top of `syncGitIdentityToUser`, after the early-exit guards.

## Why Not flock / Retry

- `flock` requires changing both the script and the Go code, and adds complexity for a race window that is seconds wide
- Retry loops paper over the symptom; the API calling git config during setup is the root issue
- The signal file already exists and encodes exactly the condition we need to check

## Files to Change

- `api/pkg/services/spec_driven_task_service.go` — add setup-complete check in `syncGitIdentityToUser`
