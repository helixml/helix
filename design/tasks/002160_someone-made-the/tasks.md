# Implementation Tasks: Restore Blocking Workspace Setup Terminal Prompt (Remove 60s Auto-Close)

- [ ] In `desktop/shared/helix-workspace-setup.sh`, delete the `HELIX_SETUP_PROMPT_TIMEOUT` variable and its comment block
- [ ] In `cleanup_and_prompt`, replace the timed `read -t "$HELIX_SETUP_PROMPT_TIMEOUT" ...` block with a blocking `read -p "Enter choice [1-2]: " choice`
- [ ] Restore the original `case` so `1` closes the window and `2|*` (including a bare Enter) opens an interactive shell — terminal stays open for inspection
- [ ] Keep the `~/.helix-setup.log` tee, the `~/.helix-setup-failed` sentinel write, and the start-of-script sentinel cleanup — do NOT remove these
- [ ] Leave `api/pkg/server/auto_wake_stuck_interactions.go` unchanged (avoids the merge conflict and keeps UI error surfacing)
- [ ] Run `./stack build-ubuntu` to bake the updated script into the desktop image
- [ ] Start a new session and verify the terminal prompt waits indefinitely (no 60s countdown) and pressing Enter drops into a shell so the stack can be inspected
