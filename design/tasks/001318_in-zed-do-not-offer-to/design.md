# Design: Disable Dev Container Suggestions in Zed

## Overview

Add a setting to disable the automatic dev container suggestion notification that appears when Zed detects a `.devcontainer` directory.

## Current Behavior

When a project with a `.devcontainer` directory is opened:
1. `suggest_on_worktree_updated()` in `recent_projects/src/dev_container_suggest.rs` detects the directory
2. A notification is shown asking to re-open in a container
3. Users can dismiss per-project via "Don't Show Again" (stored in KVP store)

## Solution

Add a global setting `remote.suggest_dev_container` that disables this notification entirely when set to `false`.

### Files to Modify

1. **`crates/settings_content/src/settings_content.rs`**
   - Add `suggest_dev_container: Option<bool>` to `RemoteSettingsContent`

2. **`crates/recent_projects/src/dev_container_suggest.rs`**
   - Import settings and check `suggest_dev_container` early in `suggest_on_worktree_updated()`
   - Return early if setting is `false`

3. **`assets/settings/default.json`**
   - Add default value (or rely on `None` = `true` behavior)

### Settings Schema

```json
{
  "remote": {
    "suggest_dev_container": true
  }
}
```

### Code Flow

```
suggest_on_worktree_updated()
  → Check remote.suggest_dev_container setting
  → If false, return early (no notification)
  → Otherwise, continue with existing logic
```

## Design Decisions

1. **Global setting, not per-project**: Helix wants this disabled globally, not per-repo. The existing per-project "Don't Show Again" remains for users who want granular control.

2. **Opt-out, not opt-in**: Default to `true` to preserve existing behavior for upstream Zed users.

3. **Minimal changes**: Only touch the suggestion code path, not the dev container functionality itself.

## Helix Integration

Helix will set this in its Zed configuration:
```json
{
  "remote": {
    "suggest_dev_container": false
  }
}
```

## Implementation Notes

### Files Modified
1. `crates/settings_content/src/settings_content.rs` - Added `suggest_dev_container: Option<bool>` field to `RemoteSettingsContent` struct with doc comment
2. `crates/recent_projects/src/remote_connections.rs` - Added `suggest_dev_container: bool` field to `RemoteSettings` struct and updated `from_settings()` to read it (defaults to `true` when unset)
3. `crates/recent_projects/src/dev_container_suggest.rs` - Added check at the start of `suggest_on_worktree_updated()` that returns early if setting is `false`
4. `assets/settings/default.json` - Added `"suggest_dev_container": true` with comment

### Pattern Used
The codebase has a two-layer settings architecture:
- `RemoteSettingsContent` (in settings_content crate) - Schema definition with `Option<T>` fields
- `RemoteSettings` (in recent_projects crate) - Runtime settings with concrete types, populated via `impl Settings for RemoteSettings`

The setting is accessed via `RemoteSettings::get_global(cx).suggest_dev_container` in the suggestion code.

### Gotcha
The default.json also had a trailing comma issue - line 1088 needed a comma added after `"auto_open_panel": false` since it's no longer the last field in its object.
