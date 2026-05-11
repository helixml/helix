# Requirements: Reliable Helix ↔ GNOME ↔ Zed Theme Sync

## Overview

When the user toggles light/dark mode in the Helix browser UI, the inner desktop is supposed to follow on **two surfaces**: the GNOME desktop chrome (`color-scheme`, `gtk-theme`, wallpaper) and the Zed editor (`theme` key in `~/.config/zed/settings.json`). Today: GNOME mostly works but is sometimes ~30 s late; **Zed gets stuck in light mode** and won't switch back to dark; and the light-mode wallpaper is the stock Ubuntu Quokka image instead of the Helix logo.

## User Stories

### US-1: Toggling theme reliably switches GNOME *and* Zed
**As a** user toggling light/dark in Helix, **I want** both the GNOME desktop chrome and the Zed editor to follow within ~1 s, every time, in both directions, repeatedly.

**Acceptance Criteria:**
- Toggling dark → light flips the GNOME `color-scheme` to `prefer-light`, the `gtk-theme` to `Yaru`, and the Zed `theme` to `One Light` within ~1 s.
- Toggling light → dark flips them back to `prefer-dark` / `Yaru-dark` / `Ayu Dark` within ~1 s. Specifically: **Zed must switch back to `Ayu Dark` and not stay on `One Light`**, no matter how many times the user toggles.
- If the daemon's WebSocket dropped a `config_changed` event (e.g. mid-reconnect), the next polling cycle (≤30 s) detects the divergence and applies the correct theme to **both** GNOME and Zed — i.e. the desktop converges, never strands.
- The Helix top-bar toggle and the OS-driven `prefers-color-scheme` path produce the same result.

### US-2: Wallpaper stays Helix in both modes
**As a** user, **I want** the desktop wallpaper to remain the Helix logo in both light and dark mode.

**Acceptance Criteria:**
- Both `picture-uri` and `picture-uri-dark` are `helix-logo.png` after every toggle, in both modes.
- The Quokka wallpaper (`Questing_Quokka_Full_Light_3840x2160.png`) is no longer referenced by the daemon.

## Out of Scope

- Per-session theme overrides — the desktop already follows the *session owner* (`zed_config_handlers.go:300-303`), and that stays.
- A new light-mode Helix wallpaper asset — we're reusing the existing `helix-logo.png`.
- The user's ability to change the Zed theme manually inside Zed's UI to something other than `One Light`/`Ayu Dark` while leaving the Helix toggle alone. Today that's nominally what `USER_PREFERENCE_FIELDS` was protecting — see the design doc for why we're removing the protection and what we're doing instead.
