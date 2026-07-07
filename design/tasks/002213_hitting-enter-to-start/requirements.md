# Requirements: Fix Interactive Shell Hang in Workspace Setup Terminal

## Background

In the Helix desktop container, the ghostty "Helix Setup" terminal runs
`desktop/shared/helix-workspace-setup.sh`. After setup finishes (or fails), an
EXIT trap (`cleanup_and_prompt`) prints a menu:

```
What would you like to do?
  1) Close this window
  2) Start an interactive shell for debugging
```

Choosing `2` (or pressing Enter, since `2|*` is the default) is supposed to drop
the user into an interactive `bash` shell in the primary repo so they can debug
the startup.

**Regression:** Selecting option 2 no longer starts a usable shell. The terminal
just hangs, printing no output and showing no prompt. This used to work.

## Root Cause (already diagnosed)

Line 40 of `helix-workspace-setup.sh`:

```bash
exec > >(tee -a "$SETUP_LOG") 2>&1
```

was added on 2026-05-20 (commit `389435bb1`, "fix(desktop): surface
workspace-setup failures instead of hanging cold-start") to capture setup output
into `~/.helix-setup.log` for the failure sentinel. It redirects the script's
stdout **and** stderr into a `tee` pipe, so neither is a TTY any more.

At the end of the script the EXIT trap runs `exec bash` (line 102/103). Bash
only starts in **interactive** mode when *both* stdin and stderr are terminals.
Because stderr is now the `tee` pipe, the new `bash` starts **non-interactive**:
it prints no prompt, produces no output, and silently reads commands from stdin —
which looks exactly like a hang. The interactive-shell menu itself predates the
regression (Jan 2026) and worked fine until the `tee` redirect landed.

## User Stories

### US1: Debug a failed/succeeded setup with a real shell
As a developer whose workspace setup finished (or failed), I want to pick "Start
an interactive shell" and get a working bash prompt, so I can inspect the
container and debug.

**Acceptance Criteria:**
- Choosing option 2 (or pressing Enter) launches an interactive bash with a
  visible prompt.
- Typed commands echo and execute normally; command output is visible.
- The shell starts in the primary repo directory (or `~/work` if none), matching
  the existing behaviour.
- Typing `exit` closes the window as before.

### US2: Setup log capture is preserved
As the platform, I want the setup output to still be captured to
`~/.helix-setup.log` and the failure sentinel (`~/.helix-setup-failed`) to keep
working, so the API can still surface real setup errors in the UI.

**Acceptance Criteria:**
- Normal setup output is still teed to `~/.helix-setup.log`.
- On failure, `~/.helix-setup-failed` is still written with the log tail.
- The fix does not reintroduce the original "hanging cold-start" problem that the
  tee redirect solved.

## Out of Scope
- The deprecated `helix-run-startup-script.sh` (manual-use only; not launched
  automatically). It uses `exec bash` too but is not affected because it does not
  perform the tee redirect. Optional to align, not required.
- Any change to the setup logic, cloning, or Zed launch flow.

## Open Questions
- Confirm `/dev/tty` is reliably available in every desktop image (ubuntu-helix
  and sway variants). If there is any doubt, the design's FD-saving approach
  (saving the original terminal FDs before the tee redirect) is preferred over
  `/dev/tty`, since it does not depend on `/dev/tty` being present. Current plan
  assumes FD-saving.
