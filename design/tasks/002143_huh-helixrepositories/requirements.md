# Requirements: Prevent git config Lock Contention During Parallel Clone

## Problem

The setup script clones all project repositories in parallel (background `git clone` jobs) to speed up startup. Occasionally one or more of the parallel `git clone` processes writes to `~/.gitconfig` (credential negotiation, auto-detected settings, etc.). When multiple clones do this simultaneously they race for `~/.gitconfig.lock` and one or more fail:

```
error: could not lock config file /home/retro/.gitconfig: File exists
```

Setup exits with code 255 and the session never starts.

## User Story

As a Helix user with multiple project repositories, I want workspace setup to succeed reliably even when cloning several repos at once.

## Acceptance Criteria

- Workspace setup completes successfully for projects with multiple repositories
- The `git config` lock contention error does not occur during the clone phase
- Existing parallel clone behaviour can be preserved if a serialisation approach is used, or clones may be made sequential if that is simpler
