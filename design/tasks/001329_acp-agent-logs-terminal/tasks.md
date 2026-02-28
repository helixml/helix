# Implementation Tasks

## Primary Implementation

- [x] Add window name matching for "ACP Agent Logs" in `helix/desktop/ubuntu-config/devilspie2/helix-tiling.lua`
- [x] Call `minimize()` function for matched ACP Agent Logs window
- [x] Add debug logging for the minimize action

## Testing

- [ ] Test with `SHOW_ACP_DEBUG_LOGS=true` - verify terminal starts minimized
- [ ] Test that terminal can be restored from taskbar
- [ ] Test that logs continue to be captured while minimized
- [ ] Test that other terminal windows (e.g., "Helix Setup") are NOT minimized
- [ ] Test with `SHOW_ACP_DEBUG_LOGS` unset - verify no errors

## Files to Modify

- `helix/desktop/ubuntu-config/devilspie2/helix-tiling.lua` - Add minimize rule for "ACP Agent Logs"