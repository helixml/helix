# Helix Sway Desktop User Guide

This guide explains how to use the Sway tiling window manager in Helix Desktop sessions.

## The Problem: Modifier Keys in Browser Streaming

Sway uses the **Super** (Windows/Command) key as its modifier, but when streaming
through the browser, this key is usually captured by your browser or OS and doesn't
reach the remote desktop.

## Solution: Use the Waybar and Terminal Commands

### Waybar (Top Panel)

The waybar at the top provides clickable controls:

**Left side:**
- **Workspace numbers** - Click to switch between workspaces (1, 2, 3...)
- **🦊** - Launch Firefox
- **🐱** - Launch Kitty terminal
- **📄** - Launch OnlyOffice
- **🇺🇸 🇬🇧 🇫🇷** - Switch keyboard layout

**Right side:**
- System stats (CPU, memory, etc.)
- Clock

### Using Workspaces

Workspaces let you organize windows. By default, Zed opens on workspace 1.

**To create and switch workspaces:**

1. Open a terminal by clicking 🐱 in waybar
2. Use these commands:

```bash
# Switch to workspace 2
swaymsg workspace 2

# Move the focused window to workspace 2
swaymsg move container to workspace 2

# Switch back to workspace 1
swaymsg workspace 1
```

**Workflow example - Zed on workspace 1, terminals on workspace 2:**

```bash
# 1. Open terminal (click 🐱)
# 2. Move this terminal to workspace 2:
swaymsg move container to workspace 2

# 3. The terminal is now on workspace 2
# 4. Click "1" in waybar to go back to Zed
# 5. Click "2" in waybar when you need your terminal
```

### Opening Applications

Click the icons in waybar, or from a terminal:

```bash
kitty          # Terminal
firefox        # Browser
zed            # Already running
```

### Window Layout Commands

Sway is a tiling window manager - windows automatically tile to fill the screen.

```bash
# Split next window horizontally (side by side)
swaymsg splith

# Split next window vertically (stacked)
swaymsg splitv

# Toggle fullscreen for focused window
swaymsg fullscreen toggle

# Close focused window
swaymsg kill
```

### If Keyboard Shortcuts Work

If your browser does pass through the Super key (try pressing it), these shortcuts work:

| Shortcut | Action |
|----------|--------|
| `Super + 1/2/3...` | Switch to workspace |
| `Super + Shift + 1/2/3...` | Move window to workspace |
| `Super + Enter` | Open terminal |
| `Super + d` | App launcher |
| `Super + Shift + q` | Close window |
| `Super + f` | Toggle fullscreen |
| `Super + Arrow keys` | Move focus |

## Quick Reference Card

| Task | How to do it |
|------|--------------|
| Open terminal | Click 🐱 in waybar |
| Switch workspace | Click workspace number in waybar |
| Move window to workspace 2 | `swaymsg move container to workspace 2` |
| Go to workspace 1 | `swaymsg workspace 1` or click "1" in waybar |
| Open Firefox | Click 🦊 or run `firefox` |
| Change keyboard layout | Click flag icon (🇺🇸 🇬🇧 🇫🇷) |
| Fullscreen | `swaymsg fullscreen toggle` |
| Close window | `swaymsg kill` |
