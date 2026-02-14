# macOS Keyboard Shortcuts for Helix Desktop

**Date:** 2026-02-14
**Branch:** feature/claude-subscription-provider

## Problem

macOS users expect Command+C/V/X/A/Z/etc. for common shortcuts. On the Helix
headless Ubuntu desktop, macOS Command maps to Super/Meta, but:

1. **Super+C/V/X doesn't copy/paste** - GNOME doesn't map these by default
2. **Super+Left/Right tiles windows** instead of moving the cursor to line
   start/end (Home/End), which macOS users expect from Command+Left/Right
3. **Ghostty and Zed** need their own Super-key bindings configured
4. **Chrome** has no app-level config for this - needs system-level handling

## Architecture

Input flows: **Browser** -> **WebSocket** -> **desktop-bridge (Go)** -> **GNOME D-Bus RemoteDesktop**

The browser sends `event.metaKey` as the `ModifierMeta` bit (0x08) and the
physical key as an evdev keycode. Modifier keys (MetaLeft/MetaRight) are sent
as their own evdev keycodes (125/126).

Since this is a headless GNOME container with no physical keyboard, system-level
tools like `keyd` don't work (they filter out virtual input devices). The
remapping must happen either:

- **At the input injection layer** (desktop-bridge Go code) - works for ALL apps
- **Per-application** (Zed keymap, Ghostty config) - defense in depth

## Approach: Layered Solution

### Layer 1: Input Injection Remapping (Go - desktop-bridge)

In `api/pkg/desktop/ws_input.go`, remap Super+key combinations before injecting
into GNOME via D-Bus. This catches everything including Chrome.

**Evdev keycode path** (`handleWSKeyboardKeycode`):
- When evdev keycode for a shortcut key (C, V, X, A, Z, S, F, etc.) arrives
  with `ModifierMeta` set, replace Super modifier press/release with Ctrl

**Keysym path** (`handleWSKeyboardKeysym`, `handleWSKeyboardKeysymTap`):
- When keysym arrives with `ModifierMeta`, replace `XK_Super_L` with `XK_Control_L`

**Navigation keys** (Super+Left/Right/Up/Down):
- Super+Left -> Home
- Super+Right -> End
- Super+Up -> Ctrl+Home (document start)
- Super+Down -> Ctrl+End (document end)
- Super+Backspace -> Ctrl+Backspace (delete word, or select-all + delete for line)

### Layer 2: GNOME Keybinding Overrides (dconf)

Disable GNOME's default Super+Left/Right window tiling shortcuts that conflict:

```ini
[org/gnome/desktop/wm/keybindings]
move-to-side-e=['']
move-to-side-w=['']
```

Also disable the Super key overlay (Activities view):

```ini
[org/gnome/mutter]
overlay-key=''
```

### Layer 3: Application Keybindings (Zed + Ghostty)

**Zed** (`~/.config/zed/keymap.json`): Add `super-` bindings for common actions.
Written by the settings-sync-daemon alongside settings.json.

**Ghostty** (`~/.config/ghostty/config`): Add `super+c`/`super+v` copy/paste
bindings.

## What Gets Remapped

| macOS Shortcut | Linux Equivalent | Context |
|----------------|------------------|---------|
| Cmd+C | Ctrl+C | Copy (all apps) |
| Cmd+V | Ctrl+V | Paste (all apps) |
| Cmd+X | Ctrl+X | Cut (all apps) |
| Cmd+A | Ctrl+A | Select All |
| Cmd+Z | Ctrl+Z | Undo |
| Cmd+Shift+Z | Ctrl+Shift+Z | Redo |
| Cmd+S | Ctrl+S | Save |
| Cmd+F | Ctrl+F | Find |
| Cmd+W | Ctrl+W | Close tab |
| Cmd+T | Ctrl+T | New tab |
| Cmd+N | Ctrl+N | New window |
| Cmd+L | Ctrl+L | Address bar / Go to line |
| Cmd+R | Ctrl+R | Reload |
| Cmd+P | Ctrl+P | Print / Quick open |
| Cmd+Left | Home | Line start |
| Cmd+Right | End | Line end |
| Cmd+Up | Ctrl+Home | Document start |
| Cmd+Down | Ctrl+End | Document end |
| Cmd+Backspace | Select-line+Delete | Delete line (macOS behavior) |

## Files Modified

1. `api/pkg/desktop/ws_input.go` - Core remapping logic in input handlers
2. `desktop/ubuntu-config/dconf-settings.ini` - Disable conflicting GNOME shortcuts
3. `desktop/ubuntu-config/ghostty-config` - Add Super+C/V copy/paste
4. `desktop/ubuntu-config/startup-app.sh` - gsettings for overlay-key disable
5. `api/cmd/settings-sync-daemon/main.go` - Write Zed keymap.json

## Not Changed

- Frontend keyboard.ts - no changes needed, it already sends MetaLeft/MetaRight
  keycodes and the ModifierMeta bit correctly
- The remapping is always-on since we control the desktop environment and know
  that macOS users are the primary audience
