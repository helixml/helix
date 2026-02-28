# Design: ACP Agent Logs Terminal Minimized by Default

## Problem

The ACP agent logs terminal (Ghostty) launches in a visible state and is positioned in the left column by devilspie2. This terminal obscures part of the desktop view, making it harder for users to focus on the Zed IDE.

## Solution

Use devilspie2's `minimize()` function to minimize the ACP Agent Logs terminal immediately after it opens. The terminal is identified by its window title "ACP Agent Logs" which is set in `start-zed-core.sh`.

## Architecture

### Components Involved

1. **`helix/desktop/shared/start-zed-core.sh`** - Launches terminal with title "ACP Agent Logs"
2. **`helix/desktop/ubuntu-config/devilspie2/helix-tiling.lua`** - Window positioning rules

### Current Flow

```
start-zed-core.sh
    └── launch_terminal("ACP Agent Logs", ...)
            └── ghostty --title="ACP Agent Logs" ...
                    └── devilspie2 matches terminal class
                            └── Positions in left column (visible)
```

### New Flow

```
start-zed-core.sh
    └── launch_terminal("ACP Agent Logs", ...)
            └── ghostty --title="ACP Agent Logs" ...
                    └── devilspie2 matches window name "ACP Agent Logs"
                            └── Positions in left column
                            └── minimize()  ← NEW
```

## Implementation

### Option Considered: Ghostty startup flag

Ghostty does not have a `--start-minimized` flag. This approach is not viable.

### Chosen Approach: devilspie2 window rules

Add a specific rule in `helix-tiling.lua` that matches the window by name and calls `minimize()`.

```lua
-- ACP Agent Logs terminal -> minimize on launch
if win_name == "ACP Agent Logs" then
    debug_print("Minimizing ACP Agent Logs terminal")
    minimize()
end
```

### Why Match by Name, Not Class

- Window class for Ghostty is `com.mitchellh.ghostty` for all Ghostty windows
- Window name (title) is unique: "ACP Agent Logs" vs "Helix Setup"
- Matching by name allows selective minimization of only the logs terminal

## Key Discoveries

1. **Terminal title set in start-zed-core.sh line 138**: `launch_terminal "ACP Agent Logs" "$WORK_DIR" bash -c '...'`
2. **devilspie2 rules in**: `helix/desktop/ubuntu-config/devilspie2/helix-tiling.lua`
3. **devilspie2 provides `minimize()` function**: Standard devilspie2 API for minimizing windows
4. **Window matching uses `get_window_name()`**: Returns the title set by `--title` flag

## Risks

- **Low**: If devilspie2 rule doesn't match (e.g., title changes), terminal stays visible - no functional impact
- **Low**: minimize() behavior may vary by window manager - GNOME is the primary target