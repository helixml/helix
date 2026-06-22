# Remove workspace-setup terminal auto-close timeout

## Summary

Commit `389435bb1` changed the post-setup prompt in `helix-workspace-setup.sh`
from a blocking `read` (waited indefinitely, default → open a shell) to a
`read -t 60` that auto-closes the terminal after 60 seconds. The countdown
closes the window out from under a user who is trying to check whether the
stack actually started up correctly.

This restores the original blocking-prompt behavior: the terminal stays open
until the user chooses. The error-surfacing parts of `389435bb1` (the
`~/.helix-setup-failed` sentinel and `~/.helix-setup.log` tee, plus the
matching reader in `auto_wake_stuck_interactions.go`) are intentionally kept —
this is a surgical revert of only the timeout, not the whole commit.

## Changes

- `desktop/shared/helix-workspace-setup.sh`:
  - Remove the `HELIX_SETUP_PROMPT_TIMEOUT` variable and its comment block.
  - Replace the timed `read -t "$HELIX_SETUP_PROMPT_TIMEOUT" ...` with a
    blocking `read -p "Enter choice [1-2]: "`.
  - Restore the original `case`: `1` closes the window; `2` or any other
    input (including a bare Enter) opens an interactive shell.
- Kept: log tee, `~/.helix-setup-failed` sentinel write, and start-of-script
  sentinel cleanup. `api/pkg/server/auto_wake_stuck_interactions.go` is
  unchanged.

## Trade-off

Restoring the blocking prompt means an unattended session whose setup *fails*
parks on the prompt again instead of self-closing. UI error surfacing is
unaffected — the API still reads the `~/.helix-setup-failed` sentinel
out-of-band.

## Testing

- `bash -n` syntax check passes.
- Menu-routing harness confirms: `1` → close + exit with original code;
  `2`, bare Enter, and any other input → interactive shell (terminal stays open).
- NOT run: `./stack build-ubuntu` + live session test. `build-ubuntu` invokes
  `build-qwen-code`, which currently fails on pre-existing TypeScript errors
  (`error TS5083: Cannot read file '/build/tsconfig.json'`) unrelated to this
  change. Verify end-to-end once the qwen-code build is fixed.
