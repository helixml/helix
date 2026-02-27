# Design: Claude Code Permission Bypass Regression Fix

## Overview

This is a one-line fix to revert an incorrect field rename in the settings-sync-daemon.

## Technical Details

### The Bug

In `api/cmd/settings-sync-daemon/main.go`, line ~189, the code generates agent_servers config:

```go
return map[string]interface{}{
    "claude": map[string]interface{}{
        "default": "bypassPermissions",  // ❌ Wrong field name
        "env":     env,
    },
}
```

Should be:

```go
return map[string]interface{}{
    "claude": map[string]interface{}{
        "default_mode": "bypassPermissions",  // ✅ Correct field name
        "env":          env,
    },
}
```

### Why This Happened

The commit message mentioned "Replace default_mode with default for agent server config" but this conflated two different settings:

1. **`agent.tool_permissions.default_mode` → `agent.tool_permissions.default`**: This was a real Zed migration (upstream PR #48553, commit `de213ad4e7`). Correct to change.

2. **`agent_servers.claude.default_mode`**: This is NOT deprecated. It's the field name in `BuiltinAgentServerSettings` struct (see `zed/crates/settings_content/src/agent.rs:366`). Should NOT have been changed.

### Verification from Zed Codebase

The `BuiltinAgentServerSettings` struct in Zed confirms the field is `default_mode`:

```rust
// zed/crates/settings_content/src/agent.rs
pub struct BuiltinAgentServerSettings {
    // ... other fields ...
    /// The default mode to use for this agent.
    ///
    /// Note: Not only all agents support modes.
    ///
    /// Default: None
    pub default_mode: Option<String>,  // <-- This is the correct field name
    // ...
}
```

And `claude.rs` reads this setting:

```rust
// zed/crates/agent_servers/src/claude.rs
fn default_mode(&self, cx: &App) -> Option<acp::SessionModeId> {
    let settings = cx.read_global(|settings: &SettingsStore, _| {
        settings.get::<AllAgentServersSettings>(None).claude.clone()
    });
    settings
        .as_ref()
        .and_then(|s| s.default_mode.clone().map(acp::SessionModeId::new))  // reads default_mode
}
```

### Two Layers of Permission Control

1. **Zed-level** (`agent.tool_permissions.default="allow"`): Tells Zed's UI to auto-approve tool permission prompts from any agent. This was correctly updated and is working.

2. **Claude Code CLI-level** (`agent_servers.claude.default_mode="bypassPermissions"`): Tells Claude Code to start in bypass mode, so it doesn't even send permission prompts. This is broken due to the incorrect rename.

Both are needed for a seamless no-prompt experience.

## Files Changed

| File | Change |
|------|--------|
| `api/cmd/settings-sync-daemon/main.go` | Change `"default"` back to `"default_mode"` on line ~189 |

## Testing

1. Start new Helix session with Claude Code
2. Ask it to create a file
3. Verify no permission prompt appears
4. Check `~/.config/zed/settings.json` in container has `"default_mode": "bypassPermissions"` under `agent_servers.claude`
