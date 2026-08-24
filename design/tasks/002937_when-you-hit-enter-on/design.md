# Design: Fix Missing Shell Prompt in Helix Desktop Setup Terminal

## Scope

One file: `desktop/shared/helix-workspace-setup.sh`. Roughly 6 changed lines plus
comments. Plus image rebuilds, since the script is baked into both desktop images.

## The Mechanism (so future agents don't re-derive it)

```
kitty/ghostty  ──TTY──► helix-workspace-setup.sh
                          line 40: exec > >(tee -a $SETUP_LOG) 2>&1
                          ⇒ fd1, fd2 are now a PIPE for the rest of the process
                          ...
                          EXIT trap → cleanup_and_prompt()
                          line 102: exec bash   ← inherits the PIPE on fd1+fd2
```

Bash's interactivity test is `isatty(0) && isatty(2)`. fd2 is a pipe ⇒
non-interactive ⇒ no `PS1`, no readline, no job control, no interactive
`~/.bashrc`. Commands still run (fd0 is the real TTY) and their output still
reaches the screen (pipe → `tee` → the terminal's original fd1). That asymmetry
is the whole bug: **input side kept the TTY, output side did not.**

Corollaries that the same fix cleans up:
- Writes to a pipe are block-buffered by libc, writes to a TTY are line-buffered.
  So the `read -p "Enter choice [1-2]: "` prompt (written to stderr → pipe) can
  appear late or out of order.
- `exec bash` inherits the pipe's write end and never closes it, so `tee` never
  sees EOF while the debug shell lives.

## Chosen Approach: save and restore the original terminal fds

Save the real terminal on spare fds *before* the tee redirect, then restore them
inside the trap once the log tail has been captured.

```bash
# near line 39, BEFORE the redirect
exec 3>&1 4>&2                          # fd3/fd4 = the real terminal
exec > >(tee -a "$SETUP_LOG") 2>&1
```

```bash
cleanup_and_prompt() {
    local exit_code=$?
    ...                                  # failure banner + .helix-setup-failed
                                         # (still goes to the log — unchanged)

    # Hand the terminal back: closing fd1/fd2 lets tee flush and exit, and gives
    # the menu + debug shell real TTY stdout/stderr so bash detects interactive.
    exec 1>&3 2>&4 3>&- 4>&-

    echo "What would you like to do?"
    ...
    read -p "Enter choice [1-2]: " choice
    ...
    2|*) exec bash ;;
}
```

Restoring **before** the menu (not just before `exec bash`) also fixes US-2. The
failure banner and `.helix-setup-failed` write happen before the restore, so the
log content is byte-identical to today.

Once fds are restored, plain `exec bash` is correct: stdin and stderr are both
TTYs, so bash auto-detects interactive mode, sets `PS1`, enables readline and job
control, and sources the interactive part of `~/.bashrc`. No `-i` needed. Add a
`[ -t 0 ]` guard so a headless run (no controlling TTY) doesn't end up in a
prompt loop against a dead stdin:

```bash
if [ -t 0 ]; then exec bash; else exit $exit_code; fi
```

## Alternatives Considered

| Option | Why not |
|---|---|
| `exec bash -i` without restoring fds | Forces interactive, so a prompt appears — but it goes through `tee`, so it's block-buffered and interleaves badly with command output; terminal size/`SIGWINCH` handling and readline echo stay wrong because fd2 isn't the tty. Treats the symptom. |
| Redirect only the setup body: `{ main; } \| tee -a "$SETUP_LOG"` and run the trap outside | Structurally cleaner but a much bigger diff, changes `set -e`/`$?` propagation through the pipeline, and `PIPESTATUS` handling. Not worth it for this bug. |
| Wrap the whole thing in `script -q -c ...` to give `tee` a pty | Heavier, adds a dependency, changes log contents (embeds control sequences). |
| Drop `tee` and log to a file only | Loses live setup output in the window, which is the point of the terminal. |

Chosen option is the smallest change that fixes cause rather than symptom.

## Verification

Automated (cheap, no desktop image needed) — a `script(1)` pty harness in
`desktop/shared/test-setup-terminal-shell.sh` following the style of the existing
`desktop/shared/test-helix-specs-creation.sh`:

1. Run a trimmed copy of the redirect + trap under `script -qec` so a pty exists.
2. Feed `2\n` then `echo "DOLLAR_DASH=$-"\nexit\n` into it.
3. Assert the captured output contains `DOLLAR_DASH=` with an `i` in the flags,
   and that a prompt string was emitted.

Manual: rebuild one desktop image, start a session, press Enter at the menu,
confirm the prompt renders, tab completion and Ctrl-C work, and that
`~/.helix-setup.log` still holds the full setup transcript.

## Deployment Note

`desktop/shared/helix-workspace-setup.sh` is `ADD`ed into both images
(`Dockerfile.sway-helix:1006`, `Dockerfile.ubuntu-helix:1443`) as
`/usr/local/bin/helix-workspace-setup.sh`. Editing the repo file alone changes
nothing for running sessions — both desktop images must be rebuilt and the
sandbox version pinned in `sandbox-versions.txt` bumped as usual.

## Learnings for Future Agents

- **Helix desktop terminal layout**: `start-zed-core.sh` owns terminal launching;
  desktop-specific wrappers (`desktop/sway-config/start-zed-helix.sh` → kitty,
  `desktop/ubuntu-config/start-zed-helix.sh` → ghostty/gnome-terminal) only define
  `launch_terminal()`. Shared behaviour belongs in `desktop/shared/`.
- **The pop-up terminal is "Helix Setup"**, running
  `desktop/shared/helix-workspace-setup.sh`. A second "ACP Agent Logs" terminal
  exists only when `SHOW_ACP_DEBUG_LOGS=true` or `HELIX_DEBUG` is set.
- **Rule of thumb**: any script that does a process-wide `exec > >(...)` and later
  hands control to an interactive program must save the original fds first
  (`exec 3>&1 4>&2`). Bash interactivity depends on fd0 *and fd2*, not fd1 — a
  shell that runs commands but shows no prompt is almost always non-TTY stderr.

---

## Implementation Notes (added during implementation)

### What was changed

`desktop/shared/helix-workspace-setup.sh` — three small edits, exactly as designed:

1. `exec 3>&1 4>&2` immediately before the `tee` redirect.
2. `exec 1>&3 2>&4 3>&- 4>&-` in `cleanup_and_prompt()`, placed *after* the
   failure banner and `.helix-setup-failed` write and *before* the menu, so log
   contents are unchanged but the menu and shell get the real tty.
3. The `2|*` branch keeps plain `exec bash`, guarded by `[ -t 0 ]`; with no
   controlling tty it falls through to `trap - EXIT; exit $exit_code`.

`desktop/shared/test-setup-terminal-shell.sh` — new pty test (below).

### How the test drives the real script (the useful trick)

The design left "how do you test a 1000-line setup script" open. The answer:
**run the real script with an empty environment and a temp `$HOME`.** With
`GIT_USER_EMAIL` unset it prints `FATAL: GIT_USER_EMAIL not set` and `exit 1` at
line ~216 — early, fast, deterministic, and it fires the EXIT trap, which is the
exact path under test. No harness copy of the logic, no mocking.

```bash
( sleep 3; printf '2\n'; sleep 2; printf 'echo HELIX_TEST_FLAGS=$-\n'; ... ) \
  | env -i HOME="$FAKE_HOME" PATH="$PATH" TERM=xterm \
      script -q -e -c "bash $SETUP_SCRIPT" "$TRANSCRIPT"
```

Gotchas found while writing it:
- `env -i` matters. Inheriting the agent's environment leaks `GIT_USER_EMAIL`
  and the script no longer fails fast.
- The sleeps are load-bearing. Feeding all input at once races the script's
  `read` and the debug shell's readline; the trailing sleep keeps the pty open
  long enough for the last line to flush.
- The script takes an optional path argument so you can point it at a
  pre-fix copy (`git show HEAD:… > /tmp/original-setup.sh`) to confirm the test
  actually catches the bug.
- Assertions must tolerate readline noise: the transcript contains `^M` and
  bracketed-paste escapes (`\e[?2004h`) interleaved with the echoed command, so
  match on a distinctive token (`HELIX_TEST_FLAGS=`), never on a whole line.
- `set -o | grep ^monitor` is padded with spaces/tabs — `awk '{print $2}'`, not
  `tr`.

### Verification result

Against the fixed script: **9/9 pass**, `$- = himBHs`, `monitor=on`, and the
transcript shows a real prompt (`retro@…:~/work$`).

Against the pre-fix script (`/tmp/original-setup.sh`): **7 pass, 2 fail** —
`$- = hBs` (no `i`) and job control off, while the shell still executed the
command. That is precisely the reported symptom, reproduced and then fixed.

Logging assertions pass identically in both runs, confirming `~/.helix-setup.log`
and `~/.helix-setup-failed` are untouched by the change.

### Open questions, as resolved

The design's open questions were not answered before implementation started, so
the documented assumptions were used: keep `tee` for the whole setup run (Q2),
stop logging the debug shell's own session (Q3), plain `exec bash` with a
`[ -t 0 ]` guard rather than `-i`/`-l` (Q4), and add the pty test (Q5). Q6
(colour/progress suppression during setup) was left out of scope. Q1 was
confirmed by the symptom match — this is the in-desktop window, not the web
drawer.
