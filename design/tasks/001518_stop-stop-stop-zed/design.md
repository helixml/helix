# Design: Disable Zed Auto-Formatting (Except Go)

## Overview

Two changes in the settings-sync-daemon:

1. Add `format_on_save: "off"` globally with a Go-only override to keep `gofmt`.
2. Fix a pre-existing bug where hardcoded Helix defaults (`text_rendering_mode`, `suggest_dev_container`, and now `format_on_save`/`languages`) are lost after the first 30-second poll cycle, causing user settings to get overwritten.

## Where the Change Lives

**File:** `api/cmd/settings-sync-daemon/main.go`

The settings-sync-daemon runs inside each desktop container and writes `~/.config/zed/settings.json`. It already sets hardcoded Helix-specific defaults (`text_rendering_mode`, `suggest_dev_container`) in `syncFromHelix()`. Adding `format_on_save` follows the same pattern — but we also need to fix the poll path so these defaults stick.

## Bug: Hardcoded Defaults Lost on Poll

`syncFromHelix()` (called once at startup) sets hardcoded defaults in `d.helixSettings`:

```go
d.helixSettings = map[string]interface{}{
    "context_servers":       config.ContextServers,
    "text_rendering_mode":   "grayscale",
    "suggest_dev_container": false,
    // ... plus agent.tool_permissions injection below
}
```

But `checkHelixUpdates()` (called every 30s) rebuilds `d.helixSettings` from just the API response — it only sets `context_servers`, `language_models`, `assistant`, `agent`, `theme`. It's missing all the hardcoded defaults.

The consequence: `deepEqual(newHelixSettings, d.helixSettings)` always returns false on the first poll (because the old map has the extras), triggering a rewrite. `d.helixSettings` then gets replaced with the incomplete version. From that point on, the hardcoded defaults are gone from the settings file. **This is the bug the user observes — settings get overridden within 30 seconds.**

### Fix: Extract Hardcoded Defaults to a Helper

Create a `helixDefaults()` function that returns the static defaults map. Both `syncFromHelix()` and `checkHelixUpdates()` call it when constructing `d.helixSettings`:

```go
func helixDefaults() map[string]interface{} {
    return map[string]interface{}{
        "text_rendering_mode":   "grayscale",
        "suggest_dev_container": false,
        "format_on_save":        "off",
        "languages": map[string]interface{}{
            "Go": map[string]interface{}{
                "format_on_save": "on",
            },
        },
    }
}
```

Both `syncFromHelix()` and `checkHelixUpdates()` start from `helixDefaults()` then layer on the API response fields. The `agent.tool_permissions` injection (currently only in `syncFromHelix`) also needs to happen in `checkHelixUpdates` — same pattern.

## Deep Merge for `languages`

Currently `mergeSettings()` deep-merges `context_servers` but does a flat overwrite for everything else. If a user sets `"languages": {"TypeScript": {"tab_size": 4}}`, it would replace the entire `languages` map and lose our `"Go": {"format_on_save": "on"}` override.

Same problem in `extractUserOverrides()` — it needs to diff `languages` per-language (like it does `context_servers` per-server) so that only the user's actual language customizations are captured, not the whole merged map.

### Fix

Add the same deep-merge pattern used for `context_servers` to `languages` in both functions:

**`mergeSettings()`** — after applying Helix settings, deep-merge user `languages` on top instead of replacing:

```go
if userLangs, ok := user["languages"].(map[string]interface{}); ok {
    if helixLangs, ok := merged["languages"].(map[string]interface{}); ok {
        for lang, config := range userLangs {
            helixLangs[lang] = config
        }
    } else {
        merged["languages"] = userLangs
    }
}
```

And skip `languages` from the flat user-override loop (same as `context_servers` is skipped today).

**`extractUserOverrides()`** — diff `languages` per-language key, same pattern as the existing `context_servers` diff block.

## Why Settings-Sync-Daemon (Not `zed_config.go`)

- `zed_config.go` generates MCP/assistant/agent config from the API side. Its `ZedMCPConfig` struct doesn't have fields for `format_on_save` or `languages`.
- The settings-sync-daemon is where other Helix-specific Zed defaults live. Formatting policy belongs here too.
- Adding new fields to `ZedMCPConfig` + the API response + `helixConfigResponse` is unnecessary overhead for a static default.

## Deployment

Settings-sync-daemon changes require `./stack build-ubuntu` + starting a new session. Existing sessions keep the old settings (including the poll bug behavior).

## Risks

Low. The formatting change is a static default that users can override. The poll-cycle fix is a bug fix that makes the daemon behave as originally intended — hardcoded defaults should persist across poll cycles.