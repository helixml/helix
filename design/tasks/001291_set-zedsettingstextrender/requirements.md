# Requirements: Set Zed Text Rendering Mode to Grayscale

## User Stories

1. As a Helix platform user, I want text to render consistently across all sessions using grayscale antialiasing, so that text appears sharp and predictable regardless of the underlying display.

## Background

Text rendering on Linux can use subpixel antialiasing (which assumes a specific LCD pixel layout) or grayscale antialiasing (which works universally). For remote desktop streaming scenarios like Helix, grayscale is preferred because:
- The client display's subpixel layout is unknown
- Video compression artifacts can interfere with subpixel rendering
- Grayscale provides consistent results across all client devices

## Acceptance Criteria

1. **Zed Settings**
   - [ ] The settings sync daemon sets `text_rendering_mode` to `"grayscale"` in Zed's settings.json
   - [ ] The setting persists across session restarts
   - [ ] The setting is applied to new sessions automatically

2. **GNOME Settings**
   - [ ] The dconf-settings.ini already has `font-antialiasing='grayscale'` (verified - no change needed)
   - [ ] GNOME applications use grayscale font rendering

## Out of Scope

- User-configurable text rendering mode (hardcoded to grayscale for now)
- macOS desktop containers (different text rendering system)