# Helix Sway Desktop User Guide

This guide explains how to use the Sway tiling window manager in Helix Desktop sessions.

## Keyboard Modifier Key: Alt

Helix configures Sway to use **Alt** as the modifier key (not Super/Cmd).

This is because Super/Cmd is captured by your OS:
- macOS: Cmd+Shift+2/3 = screenshots
- Windows: Win key opens Start menu
- Browsers: Often capture these keys for their own shortcuts

**Alt passes through browser streaming reliably.**

## Using the Waybar (Top Panel)

The waybar at the top provides clickable controls:

**Left side:**
- **Workspace numbers** - Click to switch between workspaces (1, 2, 3...)
- **🦊** - Launch Firefox
- **🐱** - Launch Kitty terminal
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

## Keyboard Shortcuts

All shortcuts use **Alt** as the modifier key.

| Shortcut | Action |
|----------|--------|
| `Alt + 1/2/3...` | Switch to workspace |
| `Alt + Shift + 1/2/3...` | Move window to workspace |
| `Alt + Enter` | Open terminal |
| `Alt + d` | App launcher (dmenu) |
| `Alt + Shift + q` | Close window |
| `Alt + f` | Toggle fullscreen |
| `Alt + Arrow keys` | Move focus between windows |
| `Alt + Shift + Arrow keys` | Move window |
| `Alt + h/j/k/l` | Move focus (vim-style) |
| `Alt + Shift + h/j/k/l` | Move window (vim-style) |

## Quick Reference Card

| Task | Keyboard | Mouse/Terminal |
|------|----------|----------------|
| Open terminal | `Alt + Enter` | Click 🐱 in waybar |
| Switch to workspace 2 | `Alt + 2` | Click "2" in waybar |
| Move window to workspace 2 | `Alt + Shift + 2` | `swaymsg move container to workspace 2` |
| Close window | `Alt + Shift + q` | `swaymsg kill` |
| Fullscreen | `Alt + f` | `swaymsg fullscreen toggle` |
| Open Firefox | `Alt + Shift + f` | Click 🦊 in waybar |
| Change keyboard layout | - | Click flag icon (🇺🇸 🇬🇧 🇫🇷) |
| App launcher | `Alt + d` | - |
