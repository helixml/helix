# fix(desktop): restore tty for the setup terminal's debug shell

## Summary

The "Helix Setup" terminal that pops up in the desktop ends with a menu offering
"Start an interactive shell for debugging". Choosing it (or just hitting Enter,
which takes the `2|*` default) gave a shell with **no prompt**, no tab
completion, no arrow-key history and no Ctrl-C job control — yet typing `ls`
still printed output.

`helix-workspace-setup.sh` tees its output with:

```bash
exec > >(tee -a "$SETUP_LOG") 2>&1
```

which replaces stdout **and stderr** with a pipe for the rest of the process.
The EXIT trap's `exec bash` inherited that pipe. Bash only treats itself as
interactive when **stdin and stderr are both ttys**, so with stdin still on the
terminal and stderr on a pipe it started non-interactive: `PS1` was never set,
so no prompt was ever written — but it kept reading commands from the tty and
their output flowed back through `tee` to the screen. That asymmetry is the
whole bug.

The tee redirect was added in `389435bb1` to capture a log tail for the failure
sentinel. That behaviour is unchanged here; only its leakage into the debug
shell is fixed.

## Changes

- `desktop/shared/helix-workspace-setup.sh`
  - Save the real terminal on fd 3/4 (`exec 3>&1 4>&2`) before the tee redirect.
  - Restore it in `cleanup_and_prompt()` (`exec 1>&3 2>&4 3>&- 4>&-`) after the
    failure sentinel is written and before the menu. This also unblocks the
    block-buffered `Enter choice [1-2]:` prompt and lets `tee` see EOF and exit
    instead of being held open forever by the exec'd shell.
  - Guard `exec bash` with `[ -t 0 ]` so headless runs exit instead of spawning
    a shell with no terminal. No `-i` is needed — with the fds restored bash
    detects interactive mode itself.
- `desktop/shared/test-setup-terminal-shell.sh` (new) — pty test that drives the
  **real** script under `script(1)`. An empty environment leaves
  `GIT_USER_EMAIL` unset, so the script fails fast and fires the EXIT trap; the
  test answers `2` and inspects the resulting shell.

`desktop/shared/helix-run-startup-script.sh` ends with the same `exec bash` but
has no output redirect, so its shell was already interactive — left unchanged.

## Testing

`desktop/shared/test-setup-terminal-shell.sh` asserts the shell reports `i` in
`$-`, has job control on, and that `~/.helix-setup.log` and
`~/.helix-setup-failed` still contain what they did before.

| Script | Result | `$-` |
|---|---|---|
| Fixed | 9/9 pass | `himBHs` |
| Pre-fix (`git show HEAD:…`) | 7 pass, 2 fail | `hBs` |

Run both on the host and **inside `helix-ubuntu:latest` as uid 1000** (the
script bind-mounted over the image, exercising the image's real bash 5.2.37 and
`/etc/bash.bashrc`) — same results in both environments. The pre-fix run still
executes the command while showing no prompt, reproducing the reported symptom
exactly.

**Not done:** a full `./stack build-ubuntu` / `build-sway`. Both images bake the
script into `/usr/local/bin/`, so a rebuild is required before the fix reaches
live sessions.
