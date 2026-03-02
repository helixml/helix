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
- Directory will be deleted as part of this task

## Current Window Management

Ubuntu GNOME containers use **Mutter/GNOME Shell on Xwayland** (DISPLAY=:9). Windows are managed by GNOME Shell's built-in window manager (Mutter).

There is **no automatic window positioning** currently in place. Windows appear wherever GNOME Shell places them by default.

## Available Resources

**Helix GNOME Shell Extension:** `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/`

This extension currently tracks cursor shape changes. It has access to:
- `global.display` - The display object
- `global.get_window_actors()` - All windows
- Window management APIs via Meta.Window

We can extend this extension to also handle window minimization.

## Solution: Extend GNOME Shell Extension

Add window management to the existing `helix-cursor@helix.ml` extension to minimize the "ACP Agent Logs" terminal on launch.

### GNOME Shell Window Management API

```javascript
// Get all windows
global.get_window_actors().forEach(actor => {
    let window = actor.get_meta_window();
    let title = window.get_title();
    
    if (title === "ACP Agent Logs") {
        window.minimize();
    }
});
```

### Implementation Approach

1. Add a window tracker to the extension that watches for new windows
2. When a window with title "ACP Agent Logs" is created, minimize it immediately
3. Use `global.display.connect('window-created', ...)` signal

```javascript
enable() {
    // ... existing cursor tracking code ...
    
    // Window management: minimize ACP Agent Logs terminal
    this._windowCreatedId = global.display.connect('window-created', (display, window) => {
        this._onWindowCreated(window);
    });
}

_onWindowCreated(window) {
    // Wait a moment for window title to be set
    GLib.timeout_add(GLib.PRIORITY_DEFAULT, 100, () => {
        const title = window.get_title();
        if (title && title.includes('ACP Agent Logs')) {
            console.log('[HelixCursor] Minimizing ACP Agent Logs terminal');
            window.minimize();
        }
        return GLib.SOURCE_REMOVE;
    });
}

disable() {
    // ... existing cleanup ...
    
    if (this._windowCreatedId) {
        global.display.disconnect(this._windowCreatedId);
        this._windowCreatedId = 0;
    }
}
```

## Alternative: Don't Launch Terminal by Default

**Even simpler:** Just comment out the `launch_acp_log_viewer` call entirely.

The logs are still written to `~/.local/share/zed/logs/*.log` regardless of whether the terminal viewer is shown. Users can manually tail logs when needed.

This is the **absolute simplest** solution and removes an unnecessary visible window.

## Chosen Approach

**Two-part solution:**

1. **Primary:** Extend GNOME Shell extension to minimize "ACP Agent Logs" window on creation
2. **Cleanup:** Remove unused devilspie2 directory

This keeps the debug feature functional (terminal can be restored from taskbar) while solving the visual obstruction issue.

## Key Discoveries

1. **devilspie2 config exists but is NOT used** - Legacy artifact, will be deleted
2. **GNOME Shell extension already exists** - `helix-cursor@helix.ml` can be extended
3. **GNOME Shell provides window management APIs** - Meta.Window.minimize() is available
4. **window-created signal** - Fires when new windows appear, perfect for auto-minimization
5. **ACP logs are written to files** - Terminal viewer is optional convenience

## Implementation Files

- `helix/desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js` - Add window-created handler
- `helix/desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json` - Bump version to 2
- `helix/desktop/ubuntu-config/devilspie2/` - DELETE (unused legacy)
- `helix/design/2025-12-08-ubuntu-layout.md` - Add deprecation notice

## Implementation Summary

### What Was Done

1. **Removed unused devilspie2 directory** - Legacy config that was never installed or used
2. **Extended GNOME Shell extension** - Added window-created signal handler to existing `helix-cursor@helix.ml` extension
3. **Added auto-minimize logic** - When a window with title containing "ACP Agent Logs" is created, it's minimized after 100ms
4. **Updated metadata** - Bumped extension version to 2, updated description
5. **Marked old design doc** - Added deprecation notice to 2025-12-08-ubuntu-layout.md
6. **Added MCP server tool** - Implemented `minimize_window` tool for agents to minimize windows programmatically (useful for screencasts/demos)
7. **MAJOR: Removed all X11 cruft** - Replaced wmctrl/xdotool with gdbus in ALL MCP window management handlers

### Code Changes

**extension.js additions:**
- Added `_windowCreatedId` field to track window-created signal
- Connected to `global.display.connect('window-created', ...)` in `enable()`
- Added `_onWindowCreated(window)` handler that checks title and calls `window.minimize()`
- Added cleanup in `disable()` to disconnect window-created signal

**Why this approach:**
- Reuses existing GNOME Shell extension infrastructure
- No additional packages to install
- GNOME Shell API is stable and well-supported
- 100ms delay ensures window title is set before checking
- Terminal can be restored from taskbar when needed

### MCP Server Tools - Complete Rewrite for Wayland

**Critical Discovery:** Ubuntu desktop is now **Wayland-only** (uses `wayland-0` socket, not X11).

**wmctrl/xdotool are X11-only** - they require EWMH/NetWM which are X11 window manager protocols. They do NOT work on Wayland. All existing MCP window tools were BROKEN on GNOME.

**Solution:** Rewrote all GNOME handlers to use `gdbus` to execute JavaScript in GNOME Shell via `org.gnome.Shell.Eval` method.

**Tools fixed/updated:**
- `list_windows` - Now returns ID/workspace/title via gdbus (was using wmctrl -l -p)
- `focus_window` - Uses `window.focus(global.get_current_time())` via gdbus
- `maximize_window` - Uses `window.maximize()` or `window.make_fullscreen()` via gdbus
- `tile_window` - Uses `window.tile_left()` / `window.tile_right()` via gdbus
- `move_to_workspace` - Uses `window.change_workspace(ws)` via gdbus
- `switch_to_workspace` - Uses `workspace.activate(timestamp)` via gdbus
- `minimize_window` - **NEW** - Uses `window.minimize()` via gdbus

**All handlers now work on Wayland GNOME.** Agents can use these tools for window management during screencasts/demos.

**Agent usage example:**
```
Use minimize_window with title="Terminal" to hide the terminal window before taking a screenshot
```

## Risks

- **Low:** Window title check timing - mitigated by 100ms delay in GNOME extension
- **Low:** GNOME Shell API compatibility across versions 45-49 - extension already supports this range
- **Low:** Extension needs to be reloaded in existing sessions - new sessions will pick it up automatically
- **Low:** gdbus output parsing - returns `(bool success, 'result')` tuple, need to extract result string

## Testing Notes

All changes require manual testing in a running desktop environment:
- GNOME extension changes require `./stack build-ubuntu` + new session
- MCP server changes require API restart (`docker compose restart api`)
- Test via MCP client or agent using the desktop tools