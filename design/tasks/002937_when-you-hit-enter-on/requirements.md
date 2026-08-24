# Requirements: Fix Missing Shell Prompt in Helix Desktop Setup Terminal

## Background

The terminal window that pops up in the Helix desktop ("Helix Setup", launched by
`start-zed-core.sh` → `helix-workspace-setup.sh`) ends with an interactive menu:

```
What would you like to do?
  1) Close this window
  2) Start an interactive shell for debugging
Enter choice [1-2]:
```

Pressing Enter (choice `2|*`) runs `exec bash`. The user gets a shell with **no
prompt**, no tab completion, no arrow-key history and no Ctrl-C job control — but
typing `ls` still prints output.

## Root Cause (confirmed)

`desktop/shared/helix-workspace-setup.sh:40`

```bash
exec > >(tee -a "$SETUP_LOG") 2>&1
```

This permanently replaces the script's **stdout and stderr** with a pipe to `tee`.
Every process the script later execs inherits those pipes — including the debug
shell at `helix-workspace-setup.sh:102` (`exec bash`).

Bash decides it is interactive by checking that **stdin _and_ stderr are both
TTYs** (bash(1): "An interactive shell is one started without non-option
arguments … and whose standard input and error are both connected to terminals").
Stdin is still the kitty/ghostty TTY, but stderr is the `tee` pipe → bash starts
**non-interactive**: `PS1` is never set and no prompt is ever written. It still
reads commands from the TTY on stdin and runs them, and their stdout flows through
`tee` back to the terminal — hence "no prompt, but `ls` works".

Reproduced locally: the child shell reports `$- = hBc` (no `i`) and `PS1=unset`.

Introduced by commit `389435bb1` (2026-05-20, "fix(desktop): surface
workspace-setup failures instead of hanging cold-start"), which added the `tee`
redirect so the failure sentinel could include a log tail. The logging is
desirable; only its leakage into the debug shell is the bug.

## User Stories

**US-1 — Working debug shell**
As a developer debugging a Helix desktop session, when I choose "Start an
interactive shell" in the setup terminal, I get a normal interactive bash so I can
actually debug.

- [ ] The shell prints a prompt (`PS1`) before each command.
- [ ] `echo $-` contains `i`; `[[ $- == *i* ]]` is true.
- [ ] Readline works: arrow-key history, tab completion, Ctrl-R.
- [ ] Job control works: Ctrl-C interrupts the foreground command instead of
      killing the shell; `fg`/`bg`/`jobs` work.
- [ ] `~/.bashrc` interactive sections are sourced (PATH, aliases, nvm, prompt).
- [ ] Command stdout and stderr appear on the terminal, unbuffered/in order.

**US-2 — Menu prompt appears immediately**
As a user staring at the setup terminal, the `Enter choice [1-2]:` prompt appears
as soon as setup finishes.

- [ ] The menu text and the `read -p` prompt are visible before any keypress
      (today they are written to a pipe and can be block-buffered/delayed).

**US-3 — Setup logging still works**
As the Helix API, I still get `~/.helix-setup.log` and `~/.helix-setup-failed`
so failures surface in the UI instead of the generic "agent never connected"
banner.

- [ ] `~/.helix-setup.log` contains the full setup output as it does today.
- [ ] On non-zero exit, `~/.helix-setup-failed` still contains
      `{"exit_code": N, "log_tail": "..."}` with the last 80 log lines.
- [ ] `tee` receives EOF and flushes/exits once the debug shell takes over
      (today it is kept alive forever by the exec'd shell holding the pipe).

**US-4 — Both desktop images fixed**
- [ ] Fix lands in the shared script so both `Dockerfile.sway-helix` (kitty) and
      `Dockerfile.ubuntu-helix` (ghostty/gnome-terminal) pick it up; both copy
      `desktop/shared/helix-workspace-setup.sh` to
      `/usr/local/bin/helix-workspace-setup.sh`.

## Non-Goals

- Changing what the setup script does, the menu wording, or the log format.
- `desktop/shared/helix-run-startup-script.sh` — it has the same `exec bash`
  ending but **no** stdout redirect, so its shell is already interactive. Leave it.
- The `bash -i "$STARTUP_SCRIPT" 2>&1 | tee /tmp/helix-startup.log` calls
  (lines 993, 1026). `-i` there is to source `~/.bashrc`; those run a script file,
  not a prompt loop, so they are unaffected.

## Which Terminal This Is

Helix has two unrelated terminals; this spec targets the first.

1. **In-desktop "Helix Setup" window** (this bug) — a kitty (Sway) or
   ghostty/gnome-terminal (Ubuntu) window launched inside the streamed desktop by
   `desktop/shared/start-zed-core.sh:271`, running
   `desktop/shared/helix-workspace-setup.sh`. It literally asks you to press a
   key (`Enter choice [1-2]:`), and Enter takes the `2|*` default → `exec bash`.
2. **Web terminal drawer** (not this bug) — xterm.js in the browser
   (`frontend/src/components/session/PersistentTerminalPane.tsx`) over
   `WS /api/v1/sessions/{id}/terminal`, served by
   `api/pkg/hydra/sandbox_handlers.go:266` via `docker exec` with `Tty: true`
   into tmux. That path already sets `PS1` through a generated
   `--rcfile` and contains no output-capturing redirection.

## Open Questions

1. **Terminal identification.** This spec assumes "the default terminal that pops
   up in the Helix desktop" is the in-desktop "Helix Setup" window (#1 above) —
   it matches the symptom exactly and is the only shell in the codebase started
   with redirected stderr. Confirm, in case you meant the web drawer.
2. **Should setup output after the fix still be teed?** The proposed fix keeps
   `tee` for the whole setup run and only restores the real TTY at the menu/debug
   shell. Assumption: yes, keep logging identical. Confirm.
3. **Should the debug shell's own session be logged too?** Currently it
   accidentally is (via the inherited pipe). The fix intentionally stops that so
   the shell gets a real TTY. Assumed acceptable — confirm nobody relies on debug
   shell transcripts landing in `~/.helix-setup.log`.
4. **`exec bash` vs `exec bash -i` vs `exec bash -l`?** Plan is plain `exec bash`
   with restored TTY fds (bash then auto-detects interactive), with a `[ -t 0 ]`
   guard falling back for headless. Say if you'd prefer a login shell (`-l`).
5. **Verification depth.** Is a `script(1)`-based pty test in
   `desktop/shared/` wanted, or is manual verification in a rebuilt desktop image
   enough? Existing precedent: `desktop/shared/test-helix-specs-creation.sh`.
6. **Colour/progress output.** The tee pipe also makes `git`/`npm` see a non-TTY
   stdout during setup, so progress bars and colour are suppressed in the setup
   log window. Out of scope unless you want it addressed here.
