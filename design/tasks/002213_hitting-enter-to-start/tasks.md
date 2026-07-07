# Implementation Tasks: Fix Interactive Shell Hang in Workspace Setup Terminal

- [ ] In `desktop/shared/helix-workspace-setup.sh`, before the tee redirect (current line 40), save the real terminal FDs: add `exec 3>&1 4>&2` immediately before `SETUP_LOG=...` / `exec > >(tee -a "$SETUP_LOG") 2>&1`, with an explanatory comment.
- [ ] In the `cleanup_and_prompt` function's interactive-shell branch (case `2|*`), replace `exec bash` with `exec bash -i >&3 2>&4` and add a comment explaining the reconnect-to-TTY reasoning.
- [ ] Verify no other numbered file descriptors (3/4) are used elsewhere in the script so there is no collision.
- [ ] Manually verify in a desktop container: option 2 (and pressing Enter) launches an interactive bash with a visible prompt and working command echo/output.
- [ ] Manually verify `~/.helix-setup.log` still captures setup output and `~/.helix-setup-failed` is still written on a forced failure.
- [ ] (Optional) Align the deprecated `desktop/shared/helix-run-startup-script.sh` menu for consistency if desired; not required for the fix.
