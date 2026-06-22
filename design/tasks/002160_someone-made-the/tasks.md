# Implementation Tasks: Restore Blocking Workspace Setup Terminal Prompt (Remove 60s Auto-Close)

- [x] In `desktop/shared/helix-workspace-setup.sh`, delete the `HELIX_SETUP_PROMPT_TIMEOUT` variable and its comment block
- [x] In `cleanup_and_prompt`, replace the timed `read -t "$HELIX_SETUP_PROMPT_TIMEOUT" ...` block with a blocking `read -p "Enter choice [1-2]: " choice`
- [x] Restore the original `case` so `1` closes the window and `2|*` (including a bare Enter) opens an interactive shell — terminal stays open for inspection
- [x] Keep the `~/.helix-setup.log` tee, the `~/.helix-setup-failed` sentinel write, and the start-of-script sentinel cleanup — do NOT remove these
- [x] Leave `api/pkg/server/auto_wake_stuck_interactions.go` unchanged (avoids the merge conflict and keeps UI error surfacing)
- [x] Verify script-level: `bash -n` passes, menu routing test confirms `1`=close+exit, `2`/Enter/other=shell, no `HELIX_SETUP_PROMPT_TIMEOUT` references remain
- [ ] BLOCKED: `./stack build-ubuntu` to bake the script into the desktop image — `build-ubuntu` calls `build-qwen-code` which fails on pre-existing TypeScript errors (unrelated to this change; same failure broke inner-stack startup). Run once qwen-code build is fixed.
- [ ] BLOCKED: Start a new session and verify the prompt waits indefinitely (no countdown) and Enter drops into a shell — requires the desktop image rebuild above + a running stack (no containers up in this env)
