# Implementation Tasks: Fix Missing Shell Prompt in Helix Desktop Setup Terminal

- [x] In `desktop/shared/helix-workspace-setup.sh`, add `exec 3>&1 4>&2` immediately before the `exec > >(tee -a "$SETUP_LOG") 2>&1` line (~line 40), with a comment explaining fd3/fd4 hold the real terminal.
- [x] In `cleanup_and_prompt()`, after the failure banner and `.helix-setup-failed` write but before the "What would you like to do?" menu, restore the terminal with `exec 1>&3 2>&4 3>&- 4>&-` and comment why (lets `tee` flush/exit; gives bash TTY stderr so it detects interactive mode).
- [x] Keep `exec bash` (no `-i` needed once fds are restored) and guard it with `[ -t 0 ]`, falling back to `exit $exit_code` when there is no controlling TTY (headless runs).
- [x] Verify the failure path is unchanged: `~/.helix-setup.log` still gets the full transcript and `~/.helix-setup-failed` still contains `{"exit_code": N, "log_tail": "..."}`.
- [x] Add `desktop/shared/test-setup-terminal-shell.sh` (styled after `test-helix-specs-creation.sh`) that drives the trap path under `script -qec`, feeds `2` then `echo "DOLLAR_DASH=$-"`, and asserts the flags contain `i` and a prompt was printed.
- [x] Run the new test locally and confirm it fails against the current script and passes after the fix.
- [x] Verify inside the real desktop image. Done without a full rebuild: `helix-ubuntu:latest` was already built locally, and the only change is a script `ADD`ed near the end of the Dockerfile, so the script was bind-mounted into the image and the pty test run there as uid 1000. Fixed script: 9/9 pass, `$- = himBHs`, job control on. Original script in the same image: `$- = hBs`, 2 failures. A `./stack build-ubuntu` is still needed before the change reaches live sessions.
- [x] Confirm `desktop/shared/helix-run-startup-script.sh` needs no change (it has no stdout redirect) and note this in the commit message.
- [x] Commit with a `fix(desktop):` message describing the non-TTY-stderr root cause and referencing commit `389435bb1` as where the redirect was introduced.
