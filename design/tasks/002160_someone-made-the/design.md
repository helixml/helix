# Design: Remove Terminal Auto-Close Countdown Prompt from Workspace Setup

## File to Change

`desktop/shared/helix-workspace-setup.sh` — specifically the `cleanup_and_prompt` function and EXIT trap (lines 56–127 in the current file).

## What to Change

### Current behaviour (introduced in commit `389435bb1`)

```bash
HELIX_SETUP_PROMPT_TIMEOUT="${HELIX_SETUP_PROMPT_TIMEOUT:-60}"

cleanup_and_prompt() {
    local exit_code=$?
    ...
    # [error sentinel writing on failure]
    ...
    echo "What would you like to do?"
    echo "  1) Close this window"
    echo "  2) Start an interactive shell for debugging"
    read -t "$HELIX_SETUP_PROMPT_TIMEOUT" -p "Enter choice [1-2] (auto-close in ${HELIX_SETUP_PROMPT_TIMEOUT}s): " choice
    # ... handles choice
}
trap cleanup_and_prompt EXIT
```

### Desired behaviour

Remove the interactive menu entirely. Keep the rest:

```bash
cleanup_and_prompt() {
    local exit_code=$?
    if [ $exit_code -ne 0 ]; then
        echo "❌ Setup failed with exit code $exit_code"
        # [write ~/.helix-setup-failed sentinel — KEEP THIS]
    fi
    trap - EXIT
    exit $exit_code
}
trap cleanup_and_prompt EXIT
```

On success (`exit_code=0`), the script reaches the end of its main body (after the startup script runs) and exits 0. The trap fires, sees exit_code=0, and exits immediately — no menu, no read.

On failure, the sentinel is written and the script exits non-zero immediately — no blocking `read`, so unattended sessions still self-report failures to the API.

## What to Remove

- The `HELIX_SETUP_PROMPT_TIMEOUT` variable and its comment block
- The `echo "What would you like to do?"` / `echo "  1) Close"` / `echo "  2) Start an interactive shell"` lines
- The `read -t` call and the `if/else` block around it
- The `case "$choice" in 2) exec bash ;; *) exit ;; esac` block

## What to Keep

- The `exec > >(tee -a "$SETUP_LOG") 2>&1` log tee
- The `~/.helix-setup-failed` sentinel write on failure (API depends on it)
- The `rm -f "$HOME/.helix-setup-failed"` cleanup at script start
- The `trap cleanup_and_prompt EXIT` itself — it's needed to catch unexpected exits and write the failure sentinel

## Why Not Keep the Interactive Shell Option?

The previous default before `389435bb1` was `2|*)` → exec bash — meaning a blank enter opened a shell. The user's request is to remove the menu, so we drop the interactive shell option as well. If someone wants a shell after setup, they can open a new terminal.

## Rebuild Required

This script is baked into the desktop image at `/usr/local/bin/helix-workspace-setup.sh`. After the code change is committed, run:

```
./stack build-ubuntu
```

Then start a new session to test.
