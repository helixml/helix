# macOS Keyboard Shortcuts for Helix Desktop

**Date:** 2026-02-14
**Branch:** feature/claude-subscription-provider

## Problem

macOS users expect Command+C/V/X/A/Z/etc. for common shortcuts. On the Helix
headless Ubuntu desktop, macOS Command maps to Super/Meta, but:

1. **Super+C/V/X doesn't copy/paste** - GNOME doesn't map these by default
2. **Super+Left/Right tiles windows** instead of being available for apps
3. **Chrome and GTK4 apps** can't be configured to respond to Super+key shortcuts

## Approach: XKB System-Level Remap

Use XKB `altwin:ctrl_win` to remap Super (Command) to Ctrl at the keyboard layout
level. This makes ALL apps (Chrome, GTK3, GTK4, Electron, etc.) see Ctrl+C when
the user presses Command+C, with zero per-app configuration needed.

Also use `caps:ctrl_nocaps` to remap Caps Lock to Ctrl.

### How It Works

```bash
gsettings set org.gnome.desktop.input-sources xkb-options "['altwin:ctrl_win', 'caps:ctrl_nocaps']"
gsettings set org.gnome.mutter overlay-key ''
```

- `altwin:ctrl_win` — Super/Win keys produce Ctrl modifier
- `caps:ctrl_nocaps` — Caps Lock produces Ctrl
- `overlay-key=''` — Disable GNOME Activities overlay on Super tap

The original Ctrl keys still produce Ctrl, so terminal Ctrl+C for SIGINT works.

### Why Not Per-App Configuration?

We investigated configuring each application to respond to Super+key shortcuts:
- **GTK3**: Supports `@binding-set` CSS rules with `<Super>` modifier
- **GTK4**: Removed `@binding-set` entirely — no system-level keybinding config
- **Chrome**: Does not support Super+key shortcuts at all

XKB remapping is the only approach that works universally across all toolkits.

## Files Modified

1. `desktop/ubuntu-config/dconf-settings.ini` — XKB options in `[org/gnome/desktop/input-sources]`
2. `desktop/ubuntu-config/startup-app.sh` — gsettings for XKB options + overlay-key disable

## Not Changed

- `api/pkg/desktop/ws_input.go` — No input-layer remapping. Keys pass through unchanged.
- `desktop/ubuntu-config/ghostty-config` — No Super keybindings needed (XKB handles it)
- `api/cmd/settings-sync-daemon/main.go` — No Zed keymap.json needed (XKB handles it)
- Frontend keyboard.ts — No changes needed, sends MetaLeft/MetaRight keycodes correctly
