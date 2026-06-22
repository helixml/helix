# Design: Restore Blocking Workspace Setup Terminal Prompt (Remove 60s Auto-Close)

## Approach: Surgical Revert of the Timeout (not a full commit revert)

The user asked to "revert that one commit" (`389435bb1`). After investigation, a literal full revert is the wrong tool:

- `git revert 389435bb1` **conflicts** on `api/pkg/server/auto_wake_stuck_interactions.go` — a later commit (`ac9be34b7`) refactored `maybeKickColdStart` to load the session once at the top, so the reverted hunk no longer applies. (Verified by a no-commit dry-run.)
- The commit bundles an **unrelated, still-wanted feature**: surfacing real setup-failure reasons in the UI via the `~/.helix-setup-failed` sentinel. A full revert removes that too.

So we revert **only the timeout behavior**, which lives entirely in one function of one file.

## File to Change

`desktop/shared/helix-workspace-setup.sh` — the `cleanup_and_prompt` function and the `HELIX_SETUP_PROMPT_TIMEOUT` variable above it.

## Changes

### 1. Remove the `HELIX_SETUP_PROMPT_TIMEOUT` variable

Delete the variable and its comment block (currently ~lines 42–52):

```bash
# How long the cleanup_and_prompt menu waits ...
HELIX_SETUP_PROMPT_TIMEOUT="${HELIX_SETUP_PROMPT_TIMEOUT:-60}"
```

### 2. Restore the blocking `read` and the default-to-shell behavior

In `cleanup_and_prompt`, the current timed prompt:

```bash
local choice=""
if read -t "$HELIX_SETUP_PROMPT_TIMEOUT" -p "Enter choice [1-2] (auto-close in ${HELIX_SETUP_PROMPT_TIMEOUT}s): " choice; then
    : # got input
else
    echo ""
    echo "(no input within ${HELIX_SETUP_PROMPT_TIMEOUT}s — closing)"
    choice=1
fi

case "$choice" in
    2)
        echo ""
        echo "Starting interactive shell..."
        ...
        exec bash
        ;;
    *)
        # Default (1 or timeout): disable trap and exit with original code
        trap - EXIT
        exit $exit_code
        ;;
esac
```

becomes the original blocking form (pre-`389435bb1`):

```bash
read -p "Enter choice [1-2]: " choice

case "$choice" in
    1)
        # Disable trap before exiting to avoid infinite loop
        trap - EXIT
        exit $exit_code
        ;;
    2|*)
        echo ""
        echo "Starting interactive shell..."
        echo "Type 'exit' to close this window."
        echo ""
        if [ -n "$HELIX_PRIMARY_REPO_NAME" ] && [ -d "$HOME/work/$HELIX_PRIMARY_REPO_NAME" ]; then
            cd "$HOME/work/$HELIX_PRIMARY_REPO_NAME"
        else
            cd "$HOME/work"
        fi
        exec bash
        ;;
esac
```

Net effect: the prompt waits indefinitely; pressing `1` closes the window, anything else (including a bare Enter) drops into a shell so the user can inspect the running stack.

## What to KEEP (do NOT revert)

- `exec > >(tee -a "$SETUP_LOG") 2>&1` — the `~/.helix-setup.log` tee.
- The `~/.helix-setup-failed` sentinel write inside `cleanup_and_prompt`'s failure branch.
- The `rm -f "$HOME/.helix-setup-failed"` cleanup near the top of the script.
- `api/pkg/server/auto_wake_stuck_interactions.go` — untouched. The sentinel is still written (before the now-blocking `read`), so the auto-wake worker can still read it via hydra and surface the real error. Leaving this file alone also avoids the merge conflict.

## Considered and Rejected: Full `git revert 389435bb1`

- Does not apply cleanly (Go conflict from a later refactor) — needs manual conflict resolution.
- Removes the unrelated `~/.helix-setup-failed` error-surfacing feature, regressing UI error messages back to the generic "agent never connected" banner.
- Larger blast radius for no benefit to the user's actual complaint (the timeout).

If the user explicitly wants the full revert anyway, the fallback is: `git revert --no-commit 389435bb1`, then resolve the conflict in `auto_wake_stuck_interactions.go` by removing the sentinel-reading block while preserving the later `session`-reuse refactor, then `git revert --continue`.

## Trade-off the User Should Know

Restoring the blocking prompt reintroduces the original behavior where an **unattended** session whose setup *fails* parks on `read` instead of self-closing. This is acceptable for the user's interactive use case (they're watching the terminal). Error surfacing in the UI is unaffected because the sentinel is still written and read out-of-band by the API.

## Rebuild Required

The script is baked into the desktop image at `/usr/local/bin/helix-workspace-setup.sh`. After committing:

```
./stack build-ubuntu
```

Then start a new session to test.
