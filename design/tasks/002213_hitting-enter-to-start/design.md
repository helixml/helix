# Design: Fix Interactive Shell Hang in Workspace Setup Terminal

## Summary

Restore the real terminal file descriptors when launching the interactive
debugging shell in `desktop/shared/helix-workspace-setup.sh`, so `bash` starts
in interactive mode (visible prompt, echoed input) while still keeping the
`tee`-to-log behaviour for the rest of the script.

## The Problem in One Line

`exec > >(tee -a "$SETUP_LOG") 2>&1` makes stdout/stderr a pipe, and bash needs
stderr to be a TTY to auto-detect interactive mode — so the later `exec bash`
starts non-interactive and hangs.

## Chosen Approach: Save and restore the terminal FDs

Save the original terminal stdout/stderr into spare file descriptors **before**
the tee redirect, then use them (plus an explicit `-i`) when launching the
interactive shell.

### 1. Before the tee redirect (around current line 40)

```bash
# Save the real terminal stdout/stderr so the interactive debug shell can be
# reconnected to the TTY later (the tee redirect below makes fd 1/2 a pipe,
# which would otherwise make `exec bash` start non-interactive and hang).
exec 3>&1 4>&2
SETUP_LOG="$HOME/.helix-setup.log"
: > "$SETUP_LOG" || true
exec > >(tee -a "$SETUP_LOG") 2>&1
```

### 2. In the interactive-shell branch of `cleanup_and_prompt` (case `2|*`)

Replace `exec bash` with a form that reconnects to the terminal and forces
interactive mode:

```bash
# Reconnect stdout/stderr to the real terminal (fds 3/4 saved before the tee
# redirect) and force interactive mode so we get a prompt. Without this, bash
# inherits the tee pipe on stderr and starts non-interactive (no prompt, hangs).
exec bash -i >&3 2>&4
```

stdin is never redirected in the script, so it is still the terminal and does
not need restoring. `-i` makes the intent explicit and guarantees interactive
mode even if FD detection is imperfect.

## Why not `/dev/tty`?

`exec bash -i < /dev/tty > /dev/tty 2>&1` is a simpler-looking alternative, but
it depends on `/dev/tty` being present and pointing at the ghostty pty inside the
container. Saving the FDs the script already owns is more robust and has no extra
dependency. (See requirements Open Questions.)

## Key Decisions

- **Fix at the shell-launch site, not by removing the tee redirect.** The tee
  redirect is load-bearing for the failure sentinel (`~/.helix-setup-failed`)
  and the setup log. We keep it and only reconnect the interactive shell.
- **Use saved FDs (3/4) rather than `/dev/tty`.** No dependency on `/dev/tty`.
- **Add `-i` explicitly.** Makes interactivity deterministic and self-documenting.

## Impact / Risk

- Single file changed: `desktop/shared/helix-workspace-setup.sh`.
- FDs 3 and 4 are otherwise unused in the script (verified — the script uses no
  other numbered FDs). No collision risk.
- The golden-build path (`exec sleep infinity`) and the "Close this window" path
  (choice 1) are unaffected.
- No behaviour change to logging, cloning, branch setup, or Zed launch.

## Testing / Verification

Manual verification (this is a container-terminal interaction, hard to unit
test):
1. In a desktop container, let `helix-workspace-setup.sh` run to the menu.
2. Choose option 2 (or press Enter). Confirm a bash prompt appears and typed
   commands echo and run.
3. Confirm `~/.helix-setup.log` still contains the setup output.
4. Force a setup failure (e.g. bad repo) and confirm `~/.helix-setup-failed`
   is still written, and option 2 still yields a working shell.

## Notes for Future Agents

- This script is launched by `desktop/shared/start-zed-core.sh` via
  `launch_terminal` → `ghostty -e bash helix-workspace-setup.sh` (ubuntu image;
  sway variant is analogous). Ghostty provides the real TTY the fix reconnects to.
- Pattern to remember: after `exec > >(tee ...) 2>&1`, any later attempt to run
  an interactive program (bash, an editor, a pager) inherits the pipe, not the
  TTY. Save the terminal FDs first if you need to hand the TTY back.
- The deprecated `helix-run-startup-script.sh` has the same menu but no tee
  redirect, so it is not broken; aligning it is optional.
