# Design: Disable Zed Auto-Formatting (Except Go)

## Overview

One-line change in the settings-sync-daemon's `syncFromHelix()` function to inject `format_on_save` and a `languages` block into the Zed settings it writes.

## Where the Change Lives

**File:** `api/cmd/settings-sync-daemon/main.go`
**Function:** `syncFromHelix()` (line ~722, where `d.helixSettings` map is constructed)

The settings-sync-daemon runs inside each desktop container and writes `~/.config/zed/settings.json`. It already sets hardcoded Helix-specific defaults here (`text_rendering_mode`, `suggest_dev_container`). Adding `format_on_save` follows the same pattern.

## What Gets Added to `d.helixSettings`

```json
{
  "format_on_save": "off",
  "languages": {
    "Go": {
      "format_on_save": "on"
    }
  }
}
```

This is two new keys in the existing `d.helixSettings` map literal:

```go
d.helixSettings = map[string]interface{}{
    "context_servers":        config.ContextServers,
    "text_rendering_mode":    "grayscale",
    "suggest_dev_container":  false,
    // Disable auto-formatting globally — it mangles JS/TS/TSX in our codebases.
    // Go keeps format_on_save via per-language override (gofmt is expected).
    "format_on_save": "off",
    "languages": map[string]interface{}{
        "Go": map[string]interface{}{
            "format_on_save": "on",
        },
    },
}
```

## Why Settings-Sync-Daemon (Not `zed_config.go`)

- `zed_config.go` (`GetZedConfigForSession`) generates MCP/assistant/agent config from the API side. It returns a `ZedMCPConfig` struct that doesn't have fields for `format_on_save` or `languages`.
- The settings-sync-daemon is where other Helix-specific Zed defaults live (`text_rendering_mode`, `suggest_dev_container`). Formatting policy belongs here too.
- Adding new fields to `ZedMCPConfig` + the API response + the daemon's `helixConfigResponse` struct is unnecessary overhead for a static default.

## Interaction with User Overrides

The daemon's `mergeSettings()` applies user overrides on top of Helix settings. If a user sets `"format_on_save": "on"` in their overrides, it will win. The `languages` key from user overrides will also merge on top (standard Go map overwrite). This is the correct behavior — we set a safe default, users can opt back in.

**One subtlety:** the current `mergeSettings` does a shallow merge for non-`context_servers` keys. If a user sets `"languages": {"TypeScript": {"tab_size": 4}}`, it will replace the entire `languages` map (losing the Go override). This is acceptable for now — users who customize `languages` can include their own Go format_on_save setting. A deep merge of `languages` would be a nice future improvement but is not needed for this task.

## Deployment

Per CLAUDE.md: settings-sync-daemon changes require `./stack build-ubuntu` + starting a new session. Existing sessions keep the old settings.

## Risks

None. This is a static config default. If it breaks anything, users can override it. The only behavioral change is Zed stops auto-formatting non-Go files, which is the explicit goal.