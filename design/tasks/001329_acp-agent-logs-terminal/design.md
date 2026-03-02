# Design: ACP Agent Logs Terminal Minimized by Default

## Problem

The ACP agent logs terminal (Ghostty) launches in a visible state when `SHOW_ACP_DEBUG_LOGS=true`. This terminal obscures part of the desktop view, making it harder for users to focus on the Zed IDE.

## Discovery: devilspie2 is NOT Used

**Initial assumption (WRONG):** The design initially assumed devilspie2 was being used for window management.

**Reality:** devilspie2 is NOT installed or used in the current Ubuntu GNOME desktop image. The `desktop/ubuntu-config/devilspie2/` directory exists but is **legacy/unused**.

**Evidence:**
- `Dockerfile.ubuntu-helix` does not install devilspie2
- `desktop/ubuntu-config/startup-app.sh` does not reference devilspie2
- Design doc `2025-12-08-ubuntu-layout.md` mentions devilspie2 in architecture but it's outdated

## Current Window Management

Ubuntu GNOME containers use **Mutter/GNOME Shell on Xwayland** (DISPLAY=:9). Windows are managed by GNOME Shell's built-in window manager (Mutter).

There is **no automatic window positioning** currently in place. Windows appear wherever GNOME Shell places them by default.

## Solution Options

### Option 1: Ghostty Config (CHOSEN)

Ghostty does not have a `--start-minimized` flag, but we can hide the window using the `window-height` and `window-position-y` trick to place it off-screen, or better yet, we can simply **not launch the terminal by default**.

**Better approach:** Add a condition in `launch_acp_log_viewer()` to only launch when explicitly requested, not by default.

### Option 2: wmctrl/xdotool

Install wmctrl or xdotool and minimize the window after launch:

```bash
ghostty --title="ACP Agent Logs" ... &
sleep 0.5
wmctrl -r "ACP Agent Logs" -b add,hidden
```

**Cons:** Requires installing additional package, adds complexity, timing-dependent.

### Option 3: GNOME Extension

Create a custom GNOME Shell extension to minimize windows by title.

**Cons:** Significant complexity for a simple feature.

### Option 4: Change Default Behavior (SIMPLEST - CHOSEN)

The real question is: **do we even need this terminal visible by default?**

Looking at the code in `start-zed-core.sh`:

```bash
launch_acp_log_viewer() {
    if [ "$SHOW_ACP_DEBUG_LOGS" = "true" ] || [ -n "$HELIX_DEBUG" ]; then
        echo "Starting ACP log viewer..."
        launch_terminal "ACP Agent Logs" "$WORK_DIR" bash -c '
            echo "ACP Agent Log Viewer - Tailing Zed logs"
            echo ""
            while [ ! -d ~/.local/share/zed/logs ]; do sleep 1; done
            tail -F ~/.local/share/zed/logs/*.log 2>/dev/null
        '
    fi
}
```

This is a **debug feature**. The logs are already being captured to `~/.local/share/zed/logs/*.log`. Users can:
1. View logs via the file system
2. Manually launch a terminal and run `tail -f ~/.local/share/zed/logs/*.log`
3. Access logs via the Helix API/UI

**Proposed solution:** Simply don't auto-launch the log viewer terminal. Remove or disable the `launch_acp_log_viewer` call.

## Revised Implementation Plan

**Approach:** Comment out or remove the automatic log viewer launch. Keep the function for manual debugging if needed.

**Change in:** `helix/desktop/shared/start-zed-core.sh`

```bash
# Commented out - log viewer terminal obscures Zed window
# Users can manually tail logs: tail -f ~/.local/share/zed/logs/*.log
# launch_acp_log_viewer
```

**Alternative (if we want to keep the feature):** Add an explicit env var `HELIX_SHOW_ACP_LOG_TERMINAL=true` separate from `SHOW_ACP_DEBUG_LOGS` so logs are still written but terminal is only shown on explicit request.

## Key Discoveries

1. **devilspie2 config exists but is NOT used** - Legacy artifact from earlier implementation
2. **No window positioning automation** in current Ubuntu image - GNOME Shell manages windows freely
3. **ACP logs are written to files** - The terminal viewer is just a convenience, not required
4. **Simplest solution:** Don't launch the terminal by default - it's a debug feature

## Chosen Approach

Remove the automatic launch of the ACP log viewer terminal in `start-zed-core.sh`. The logs will still be captured to `~/.local/share/zed/logs/*.log` and can be viewed manually when needed.

This is better than minimizing because:
- No additional dependencies needed
- No window management complexity
- Cleaner desktop by default
- Logs are still fully accessible
- Debug feature remains available for manual use

## Implementation

Modify `helix/desktop/shared/start-zed-core.sh` line 274 to comment out the `launch_acp_log_viewer` call.