# Helix Sway Desktop - Quick Start

This desktop uses Sway, a tiling window manager. All keyboard shortcuts
use the Alt key as the modifier (not Super/Cmd, which browsers capture).


## COMMON TASKS

### Switch to a different workspace

  Keyboard:  Alt + 2        (or Alt + 1, Alt + 3, Alt + 4)
  Mouse:     Click the workspace number in the top bar


### Move the current window to another workspace

  Keyboard:  Alt + Shift + 2   (moves window to workspace 2)
  Mouse:     Not available - use keyboard


### Open a terminal

  Keyboard:  Alt + Enter
  Mouse:     Click the cat icon in the top bar


### Close a window

  Keyboard:  Alt + Shift + q


### Toggle fullscreen

  Keyboard:  Alt + f


### Move focus between windows

  Keyboard:  Alt + Arrow keys
             Alt + h/j/k/l (vim-style)


### Move/resize windows

  Keyboard:  Alt + Shift + Arrow keys (move window)
  Mouse:     Hold Alt + drag with left mouse (move)
             Hold Alt + drag with right mouse (resize)


## THE TOP BAR (WAYBAR)

Left side:
  - Workspace numbers (1, 2, 3, 4) - click to switch
  - Fox icon - launch Firefox
  - Cat icon - launch terminal
  - Flag icons - switch keyboard layout (US/UK/French)

Right side:
  - System stats and clock


## TYPICAL WORKFLOW

1. Zed opens on workspace 1

2. To put terminals on workspace 2:
   - Press Alt + Enter to open a terminal
   - Press Alt + Shift + 2 to move it to workspace 2
   - Press Alt + 1 to go back to Zed

3. Switch between workspaces:
   - Alt + 1 for Zed
   - Alt + 2 for terminals
   - Or click the numbers in the top bar


## ALL KEYBOARD SHORTCUTS

Workspaces:
  Alt + 1/2/3/4         Switch to workspace
  Alt + Shift + 1/2/3/4 Move window to workspace

Windows:
  Alt + Enter           Open terminal
  Alt + Shift + q       Close window
  Alt + f               Toggle fullscreen
  Alt + Arrow keys      Move focus
  Alt + Shift + Arrows  Move window
  Alt + h/j/k/l         Move focus (vim-style)
  Alt + Shift + h/j/k/l Move window (vim-style)

Apps:
  Alt + d               App launcher
  Alt + Shift + f       Open Firefox
  Alt + Shift + Return  Open terminal (alternative)


## WHY ALT INSTEAD OF SUPER/CMD?

When streaming through a browser, Super/Cmd keys are captured by your OS:
  - macOS: Cmd+Shift+3 takes a screenshot
  - Windows: Win key opens Start menu
  - Browsers: Have their own shortcuts

Alt passes through reliably to the remote desktop.
