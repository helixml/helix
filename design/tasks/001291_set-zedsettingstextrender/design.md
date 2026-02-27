# Design: Set Zed Text Rendering Mode to Grayscale

## Overview

Add `text_rendering_mode: "grayscale"` to the Helix-managed settings that the settings sync daemon writes to Zed's settings.json.

## Architecture

The settings sync daemon (`api/cmd/settings-sync-daemon/main.go`) manages Zed's settings.json by:
1. Fetching configuration from the Helix API
2. Merging with user overrides
3. Writing to `~/.config/zed/settings.json`

The `text_rendering_mode` setting is a top-level key in settings.json (not nested).

## Implementation

### Change Location

In `syncFromHelix()` function, add `text_rendering_mode` to `d.helixSettings`:

```go
d.helixSettings = map[string]interface{}{
    "context_servers": config.ContextServers,
    "text_rendering_mode": "grayscale",  // <-- Add this line
    "remote": map[string]interface{}{
        "suggest_dev_container": false,
    },
}
```

### GNOME Settings

The dconf-settings.ini already has the correct setting:
```ini
[org/gnome/desktop/interface]
font-antialiasing='grayscale'
font-hinting='slight'
```

No changes needed for GNOME.

## Key Decisions

1. **Hardcoded value**: We hardcode `grayscale` rather than making it configurable because:
   - Grayscale is the correct choice for remote streaming
   - Reduces API complexity
   - Can be made configurable later if needed

2. **Top-level setting**: Zed expects `text_rendering_mode` at the root of settings.json, not nested under a section.

## Testing

1. Start a new session
2. Check `~/.config/zed/settings.json` contains `"text_rendering_mode": "grayscale"`
3. Verify text in Zed uses grayscale antialiasing (no color fringing on text edges)