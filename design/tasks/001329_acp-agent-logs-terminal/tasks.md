# Implementation Tasks

## Primary Implementation

- [x] ~~Add window name matching for "ACP Agent Logs" in devilspie2~~ (devilspie2 not used)
- [x] ~~Call `minimize()` function~~ (devilspie2 not used)
- [x] ~~Add debug logging~~ (devilspie2 not used)
- [x] Remove unused devilspie2 config directory
- [x] Update GNOME Shell extension to minimize "ACP Agent Logs" window
- [x] Add window-created signal handler to extension
- [x] Implement window title matching and minimize logic
- [x] Update extension metadata version number
- [x] Mark 2025-12-08-ubuntu-layout.md as outdated (devilspie2 reference)

## Bonus: MCP Server Window Management

- [x] Add `minimize_window` tool to desktop MCP server
- [x] Add handler implementation for Sway and GNOME (uses gdbus for Wayland)

## Future Cleanup (Out of Scope for This Task)

- [ ] Replace wmctrl/xdotool calls in mcp_server.go with gdbus equivalents (desktop is Wayland-only now)
- [ ] Remove X11-specific code paths in focus_window, maximize_window, tile_window handlers

## Testing

_Manual testing required - needs full desktop environment with `SHOW_ACP_DEBUG_LOGS=true`_

- [ ] Test with `SHOW_ACP_DEBUG_LOGS=true` - verify terminal starts minimized
- [ ] Test that terminal can be restored from taskbar/Activities
- [ ] Test that logs are still written to ~/.local/share/zed/logs/*.log
- [ ] Test that other terminal windows (e.g., "Helix Setup") are NOT minimized
- [ ] Verify extension still tracks cursor shapes correctly
- [ ] Test across GNOME Shell versions if possible (45-49)

## Files Modified

- `helix/desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js` - Add window minimization
- `helix/desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/metadata.json` - Bump version to 2
- `helix/desktop/ubuntu-config/devilspie2/` - DELETED (unused legacy)
- `helix/design/2025-12-08-ubuntu-layout.md` - Add deprecation notice
- `helix/api/pkg/desktop/mcp_server.go` - Add minimize_window tool for agents

## Implementation Notes

- GNOME Shell extension already exists for cursor tracking
- Extension runs in GNOME Shell process, has full window management access
- `global.display.connect('window-created', ...)` fires when windows open
- `window.minimize()` is the standard GNOME API for minimizing windows
- 100ms delay ensures window title is set before checking