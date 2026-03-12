# Design: Disable Zed Auto-Formatting + Fix Settings Ownership

## Overview

The settings-sync-daemon manages `~/.config/zed/settings.json` inside desktop containers. It has an intended two-layer ownership model (Helix base + user overrides), but the implementation is inconsistent, leading to several bugs. This task fixes three things:

1. Add `format_on_save: "off"` globally with a Go-only override to keep `gofmt`.
2. Fix the settings ownership model so user preferences (theme, etc.) aren't reverted every 30 seconds.
3. Fix pre-existing bugs where hardcoded defaults are lost on poll and the daemon rewrites the file every 30 seconds even when nothing changed.

## Settings Ownership Model

### Current State (Broken)

The daemon has three ad-hoc categories with no clear boundaries:

| Category | Fields | Behavior |
|----------|--------|----------|
| **Security-protected** | `telemetry`, `agent_servers`, `default_agent`, `external_sync` | Stripped from helix→user and user→helix sync. `telemetry` is further special-cased: read from disk and preserved. |
| **Hardcoded daemon defaults** | `text_rendering_mode`, `suggest_dev_container` | Set in `syncFromHelix()` at startup. **Not set in `checkHelixUpdates()`** — silently dropped after first 30s poll. |
| **Everything else** | `theme`, `language_models`, `assistant`, `agent`, `context_servers` | Fully Helix-owned. API value is the base, user changes captured as overrides via fsnotify + `extractUserOverrides()`. |

Problems with this:
- `theme` is treated as Helix-owned, but it's a user preference. The API hardcodes `"Ayu Dark"` in `GenerateZedMCPConfig`, so theme goes through the whole override dance (user changes it → fsnotify captures it → stored as override → reapplied on merge) when it should just be left alone. In practice, users report the theme reverting within 30 seconds — the exact failure path isn't clear from code analysis alone, but the override mechanism is unnecessarily fragile for something that should simply be user-owned.
- Hardcoded defaults vanish after the first poll because `checkHelixUpdates()` rebuilds `d.helixSettings` from just the API response.
- `injectLanguageModelAPIKey()` and `injectAvailableModels()` mutate `d.helixSettings` in place after the `deepEqual` baseline is set, so the comparison always fails → the daemon rewrites the file every 30 seconds even when nothing actually changed.

### Proposed State

Three explicit ownership categories:

| Category | Fields | Behavior |
|----------|--------|----------|
| **Helix-owned** | `context_servers`, `language_models`, `assistant`, `agent`, `text_rendering_mode`, `suggest_dev_container`, `format_on_save`, `languages` | Daemon controls these. Set on startup and every poll. User can override (captured in `d.userOverrides`, applied via `mergeSettings`). |
| **User-owned** (with initial default) | `theme` | Daemon writes a default on first startup (e.g. `"Ayu Dark"`). After that, the on-disk value is always preserved — the daemon never overwrites it. Not synced as a "user override" to the API; just read from disk. |
| **Security-protected** | `telemetry`, `agent_servers` | Never synced in either direction. Read from disk and preserved. (Remove deprecated `default_agent` and `external_sync` from this list — they're cleaned up by `DEPRECATED_FIELDS`.) |

This model is enforced by three maps:

```go
var HELIX_OWNED_DEFAULTS = map[string]interface{}{
    "text_rendering_mode":   "grayscale",
    "suggest_dev_container": false,
    "format_on_save":        "off",
    "languages": map[string]interface{}{
        "Go": map[string]interface{}{
            "format_on_save": "on",
        },
    },
}

var USER_PREFERENCE_FIELDS = map[string]bool{
    "theme": true,
}

var SECURITY_PROTECTED_FIELDS = map[string]bool{
    "telemetry":     true,
    "agent_servers": true,
}
```

## Where the Change Lives

**File:** `api/cmd/settings-sync-daemon/main.go`

All changes are in this one file. No changes to `zed_config.go` or the API — the daemon-side defaults are the right place for formatting policy and user-preference protection, same as the existing `text_rendering_mode` and `suggest_dev_container`.

## Change 1: Format-on-Save Defaults

Add `format_on_save` and `languages` to the Helix-owned defaults. Included in `HELIX_OWNED_DEFAULTS` above and applied via the `helixDefaults()` helper (see Change 2).

Zed settings format:
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

Global `"off"` stops JS/TS/TSX reformatting. Per-language Go `"on"` preserves `gofmt`. Users can still override per-language via Zed UI (captured as user overrides, deep-merged — see Change 4).

## Change 2: Extract Hardcoded Defaults to Shared Helper

Create a `helixDefaults()` function returning `HELIX_OWNED_DEFAULTS`. Both `syncFromHelix()` and `checkHelixUpdates()` use it as the base for `d.helixSettings`, then layer on API response fields.

This fixes the bug where `checkHelixUpdates()` builds `newHelixSettings` from just the API response (missing `text_rendering_mode`, `suggest_dev_container`, etc.), causing them to vanish after the first 30-second poll.

The `agent.tool_permissions` injection (currently only in `syncFromHelix`) also needs to be applied in `checkHelixUpdates` — either extract to a shared helper or include in the defaults-layering step.

## Change 3: User-Preference Fields (Theme)

For fields in `USER_PREFERENCE_FIELDS`:

**`syncFromHelix()` (startup):** Set the initial default from the API response (e.g. `theme: "Ayu Dark"`) into `d.helixSettings`. This gets written to the fresh `settings.json`. On a brand new session, the user gets "Ayu Dark".

**`checkHelixUpdates()` (30s poll):** Skip these fields when building `newHelixSettings` so they don't trigger spurious diffs and don't overwrite `d.helixSettings`.

**`mergeSettings()`:** For each user-preference field, read the on-disk value and use it instead of the Helix value. This is the same pattern already used for `telemetry` — just generalized:

```go
if existingData, err := os.ReadFile(SettingsPath); err == nil {
    var existing map[string]interface{}
    if err := json.Unmarshal(existingData, &existing); err == nil {
        for field := range USER_PREFERENCE_FIELDS {
            if value, exists := existing[field]; exists {
                merged[field] = value
            }
        }
    }
}
```

**`extractUserOverrides()`:** Skip these fields — they don't need to be synced back to the API since the daemon reads them from disk.

Net effect: "Ayu Dark" is the default for new sessions, but the user's choice is never reverted.

## Change 4: Fix Spurious Rewrite Every 30 Seconds

`injectLanguageModelAPIKey()` and `injectAvailableModels()` mutate `d.helixSettings` in place (adding `api_key`, `available_models`). On the next poll, the fresh API response (without those injected keys) differs from the mutated `d.helixSettings` → `deepEqual` fails → rewrite → inject mutates → repeat forever.

**Fix:** Store a `d.helixSettingsBaseline` that holds the pre-injection version. Compare against that instead of the post-injection `d.helixSettings`:

```go
if !deepEqual(newHelixSettings, d.helixSettingsBaseline) || codeAgentChanged {
    d.helixSettingsBaseline = newHelixSettings
    d.helixSettings = copyMap(newHelixSettings) // fresh copy for inject to mutate
    d.codeAgentConfig = config.CodeAgentConfig

    d.injectKoditAuth()
    d.injectLanguageModelAPIKey()
    d.injectAvailableModels()

    merged := d.mergeSettings(d.helixSettings, d.userOverrides)
    if err := d.writeSettings(merged); err != nil {
        return err
    }
}
```

This stops the every-30-second rewrite. The daemon should only rewrite `settings.json` when the Helix API actually returns different config (e.g. MCP servers changed, model changed).

## Change 5: Deep Merge for `languages`

Currently `mergeSettings()` deep-merges `context_servers` but does a flat overwrite for everything else. If a user sets `"languages": {"TypeScript": {"tab_size": 4}}`, it replaces the entire `languages` map and loses our `"Go": {"format_on_save": "on"}`.

**`mergeSettings()`** — add the same deep-merge pattern for `languages`:

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

Skip `languages` from the flat user-override loop (same as `context_servers` is skipped today).

**`extractUserOverrides()`** — diff `languages` per-language key, same pattern as the existing `context_servers` diff block. Only capture languages the user actually customized.

## Deployment

Settings-sync-daemon changes require `./stack build-ubuntu` + starting a new session. Existing sessions keep the old behavior.

## Risks

Low. The formatting change is a static default that users can override. The ownership model fixes are bug fixes that make the daemon behave as intended — hardcoded defaults persist, user preferences aren't reverted, and the daemon doesn't rewrite the file every 30 seconds when nothing changed.