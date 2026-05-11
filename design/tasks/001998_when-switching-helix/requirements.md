# Requirements: Reliable Helix ↔ GNOME Theme Sync

## Overview

When the user toggles light/dark mode in the Helix browser UI, the inner GNOME desktop (Ubuntu) is supposed to follow. Today it works *sometimes* and not others, light→dark gets stuck on light, and the light-mode wallpaper changes to a non-Helix image. Make it reliable, symmetric, and brand-consistent.

## User Stories

### US-1: Toggling theme reliably switches the GNOME desktop
**As a** user toggling between light and dark mode in Helix, **I want** the GNOME desktop inside my spec-task session to follow within ~1 second, every time.

**Acceptance Criteria:**
- Toggling light→dark or dark→light in the Helix top bar updates the GNOME `color-scheme` and `gtk-theme` within ~1 s in the common case.
- Switching is symmetric: dark→light and light→dark both work, repeatedly, without ever getting "stuck."
- If the daemon's WebSocket to the API was disconnected at the moment the user toggled (so the live event was missed), the next polling cycle (≤30 s) detects the change and applies it to GNOME — i.e. the desktop converges, never strands.
- The behavior is the same whether the user (a) clicks the toggle in the Helix UI or (b) flips their OS color scheme (the existing OS-driven path).

### US-2: Wallpaper stays Helix in both modes
**As a** user, **I want** the desktop wallpaper to remain the Helix logo in both light and dark mode **so that** my workspace looks like Helix, not stock Ubuntu.

**Acceptance Criteria:**
- In dark mode the wallpaper is `helix-logo.png` (unchanged from today).
- In light mode the wallpaper is **also** `helix-logo.png` — *not* the Ubuntu Questing-Quokka light wallpaper.
- Both `picture-uri` and `picture-uri-dark` GNOME keys point at `helix-logo.png` after every toggle.

## Out of Scope

- Per-session theme overrides (the desktop already follows the *session owner*, not the viewer — that stays).
- A new light-mode Helix wallpaper asset. We're reusing the existing `helix-logo.png` for both modes.
- Touching Zed editor theming — `One Light` / `Ayu Dark` selection in `zed_config_handlers.go` already works; only the GNOME side is broken.
