# Implementation Tasks

## Primary Implementation

- [x] ~~Add window name matching for "ACP Agent Logs" in devilspie2~~ (devilspie2 not used)
- [x] ~~Call `minimize()` function~~ (devilspie2 not used)
- [x] ~~Add debug logging~~ (devilspie2 not used)
- [~] Remove unused devilspie2 config directory
- [~] Mark 2025-12-08-ubuntu-layout.md as outdated (devilspie2 reference)
- [~] Comment out `launch_acp_log_viewer` call in start-zed-core.sh
- [ ] Add comment explaining logs are still written to ~/.local/share/zed/logs/*.log

## Testing

_Manual testing required - needs full desktop environment with `SHOW_ACP_DEBUG_LOGS=true`_

- [ ] Test with `SHOW_ACP_DEBUG_LOGS=true` - verify NO terminal window appears
- [ ] Test that logs are still written to ~/.local/share/zed/logs/*.log
- [ ] Test that Zed still starts normally without the log viewer
- [ ] Test that users can manually tail logs if needed

## Files to Modify

- `helix/desktop/ubuntu-config/devilspie2/` - DELETE (unused legacy config)
- `helix/desktop/shared/start-zed-core.sh` - Comment out launch_acp_log_viewer call
- `helix/design/2025-12-08-ubuntu-layout.md` - Add deprecation notice about devilspie2