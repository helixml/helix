# Implementation Tasks: Remove Terminal Auto-Close Countdown Prompt from Workspace Setup

- [ ] In `desktop/shared/helix-workspace-setup.sh`, remove the `HELIX_SETUP_PROMPT_TIMEOUT` variable and its comment block (lines ~44–52)
- [ ] In `cleanup_and_prompt`, remove the "What would you like to do?" echo lines and the `read -t` call and surrounding if/else
- [ ] In `cleanup_and_prompt`, remove the `case "$choice" in` block; replace with a direct `trap - EXIT && exit $exit_code`
- [ ] Keep the `~/.helix-setup-failed` sentinel write on failure and the log tee — do not remove those
- [ ] Run `./stack build-ubuntu` to bake the updated script into the desktop image
- [ ] Start a new session and verify: the terminal shows setup output and exits cleanly without showing a "press 1 or 2" menu
- [ ] Verify the failure path: trigger a deliberate setup failure (e.g. bad `GIT_USER_EMAIL`) and confirm the `~/.helix-setup-failed` sentinel is written and the terminal exits non-zero without hanging
