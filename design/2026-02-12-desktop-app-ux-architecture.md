# Desktop App UX Architecture

**Date:** 2026-02-12
**Status:** Decided — Option A (Titlebar Status Bar) + System Tray

## Problem

The Helix desktop app wraps the Helix web app in an iframe. Both have left-hand navigation sidebars, creating a "double sidebar" problem. The desktop wrapper's sidebar (Home, Environment, Settings, Console, Storage) sits next to the web app's own sidebar (sessions, agents, projects, etc.). This wastes horizontal space and feels structurally wrong — users see two navigation hierarchies competing for attention.

More fundamentally, the information architecture is muddled. The wrapper exists to manage infrastructure (VM lifecycle, resources, licensing), but the current layout gives it equal visual weight to the actual product the user came to use.

## User Flows

Understanding the two distinct modes of interaction:

**Setup / Maintenance (infrequent)**
1. First launch: download 18GB VM image
2. License activation or trial start
3. Start VM (or configure auto-start)
4. Occasionally: resize disk, change CPU/memory, check console

**Daily use (95%+ of time)**
1. Open app (VM auto-starts)
2. Wait for boot (~30s)
3. Use Helix web UI — this is what the user actually cares about
4. Glance at VM status occasionally

The wrapper should be nearly invisible during daily use. It should be prominent during setup/maintenance, then get out of the way.

## Current State

```
+--sidebar (200px)--+---------- content area ----------+
| [Helix logo]      |                                   |
| Home         *    |  +--helix sidebar--+-- helix ---+ |
| Environment       |  | Sessions       | Chat/Agent | |
| Settings          |  | Agents         |            | |
|                   |  | Projects       |            | |
| v Advanced        |  | Settings       |            | |
|   Console         |  +----------------+------------+ |
|   Storage         |                                   |
|                   |                                   |
| [Running . 2 ses] |                                   |
+-------------------+-----------------------------------+
```

200px of sidebar + ~220px of Helix sidebar = 420px consumed by navigation. On a 1440px-wide laptop, that's 29% of the screen devoted to nav chrome.

## Option A: Titlebar Status Bar

Replace the sidebar with a thin horizontal bar integrated into the macOS titlebar area. The iframe fills the full window width below it.

```
+------ titlebar bar (32-40px tall) ---------------------+
| [Helix logo]    [. Running]         [gear] [terminal]  |
+---------------------------------------------------------+
|  +--helix sidebar--+---------- helix content ---------+ |
|  | Sessions        | Chat / Agent / etc.              | |
|  | Agents          |                                  | |
|  | Projects        |                                  | |
|  | Settings        |                                  | |
|  +------------------+---------------------------------+ |
+---------------------------------------------------------+
```

**How it works:**
- The titlebar doubles as the wrapper's control surface
- Left: Helix logo (draggable region for window movement)
- Center-left: VM status pill (`Running`, `Starting...`, `Stopped`)
- Right: icon buttons — gear (settings), terminal (console)
- Clicking gear opens a settings panel as a slide-over from the right edge (or a modal)
- Clicking the status pill when stopped shows "Start" action
- When VM is stopped / downloading, the titlebar expands into a full-screen setup view (no iframe)

**Setup mode (VM not running):**
```
+------ titlebar bar --------------------------------+
| [Helix logo]    [. Stopped]         [gear]         |
+-----------------------------------------------------+
|                                                     |
|              [Helix logo large]                     |
|                                                     |
|         Welcome to Helix Desktop                    |
|                                                     |
|    Download the environment to get started          |
|                                                     |
|           [ Download (18 GB) ]                      |
|                                                     |
|              or enter license key                   |
|                                                     |
+-----------------------------------------------------+
```

**Settings panel (slide-over):**
```
+------ titlebar bar -----------------+
| [Helix logo]  [. Running]    [gear] |
+------+------------------------------+
|      |            Settings        X |
| helix|  Resources                   |
| web  |    CPU cores: [4]            |
| app  |    Memory: [8192] MB         |
| (dim)|                              |
|      |  Network                     |
|      |    [ ] Share on network      |
|      |    SSH Port: [41222]         |
|      |    API Port: [41080]         |
|      |                              |
|      |  Storage                     |
|      |    Disk: 256 GB [Resize]     |
|      |    ZFS dedup: 1.23x          |
|      |                              |
|      |        [Save]  [Reset]       |
+------+------------------------------+
```

**Pros:**
- Maximum horizontal space for the Helix web app
- Only one sidebar (the web app's)
- Titlebar is macOS-native territory — users expect controls there
- Clean separation: titlebar = infrastructure, below = application
- Setup flow gets the full screen when it needs it

**Cons:**
- Less room for controls in a 32px bar — may feel cramped with many features
- Settings panel as slide-over adds a new interaction pattern
- Console (xterm.js) needs a reasonable amount of space — would need its own full-screen mode or a bottom drawer

**Console approach:** Open console as a bottom drawer (like browser DevTools) that splits the iframe vertically, or as a separate window.

---

## Option B: Disappearing Sidebar

Keep the sidebar but make it context-aware. When the iframe is showing (VM running, daily use mode), the sidebar collapses to a thin strip or disappears entirely. It reappears for setup and settings.

**Daily use mode (VM running, Home view):**
```
+--+------------------------------------------------+
|  |                                                 |
|  |  +--helix sidebar--+---- helix content -------+ |
|[H]  | Sessions        | Chat / Agent / etc.      | |
|  |  | Agents          |                           | |
|[.]  | Projects        |                           | |
|  |  | Settings        |                           | |
|[g]  +------------------+--------------------------+ |
|  |                                                 |
+--+------------------------------------------------+
```

The collapsed strip is ~44px wide: just enough for icon buttons. H = Helix logo (home), . = status dot, g = gear. Hovering or clicking expands to show labels and additional controls.

**Settings / Environment view (sidebar expanded):**
```
+--- sidebar (200px)--+---------- content ----------+
| [Helix logo]        |                              |
|                     |  Settings                    |
| < Back to Helix     |                              |
|                     |  CPU cores: [4]              |
| Resources           |  Memory: [8192] MB           |
| Network             |  ...                         |
| Storage             |                              |
| Console             |                              |
|                     |                              |
| [. Running]         |                              |
+---------------------+------------------------------+
```

**How it works:**
- `currentView === 'home' && vmStatus.state === 'running'` → sidebar collapses to icon strip
- Any other view → sidebar expands with full navigation
- "Back to Helix" link at top of expanded sidebar returns to the iframe
- The icon strip has: logo (click → home), status indicator, settings gear
- Transition is animated (width animation, icons fade in/out)

**Pros:**
- Sidebar disappears when not needed (daily use)
- Sidebar appears when you need it (settings, debugging)
- Familiar pattern (VS Code's activity bar, Slack's workspace switcher)
- Console gets a proper full-height view when selected
- No new interaction patterns — still a sidebar, just responsive

**Cons:**
- Still two sidebars when expanded (though you'd only see settings content, not the iframe)
- The 44px strip + Helix sidebar is still two columns, though barely noticeable
- Animation between states can feel janky if not tuned carefully
- Users might not discover the collapsed icons

---

## Option C: Chromeless Wrapper with System Tray

The most radical option. The desktop app becomes essentially invisible — just a window frame around the Helix web app. All infrastructure controls move to the macOS menu bar (system tray) and a menu bar popover.

**Daily use (VM running):**
```
+------- window frame (just titlebar) ---------------+
|  [traffic lights]                                   |
+-----------------------------------------------------+
|  +--helix sidebar--+---------- helix content -----+ |
|  | Sessions        | Chat / Agent / etc.           | |
|  | Agents          |                               | |
|  | Projects        |                               | |
|  | Settings        |                               | |
|  +------------------+------------------------------+ |
+-----------------------------------------------------+
```

No wrapper UI at all inside the window. The iframe is the entire window content.

**Menu bar (always visible in macOS menu bar):**
```
                                    [Helix icon .]
                                         |
                              +----------v-----------+
                              | Helix Desktop        |
                              | Status: Running      |
                              | Sessions: 2          |
                              | CPU: 4 cores (23%)   |
                              | Memory: 8 GB         |
                              +----------------------+
                              | Open Helix        ^O |
                              | Start / Stop VM      |
                              +----------------------+
                              | Settings...       ^, |
                              | Console...           |
                              | Storage Stats        |
                              +----------------------+
                              | Quit Helix        ^Q |
                              +----------------------+
```

**Settings opens as a separate native-feeling window or sheet:**
```
+---------- Settings (separate window) ----------+
|  Resources                                      |
|    CPU cores: [4]    Memory: [8192] MB          |
|                                                 |
|  Network                                        |
|    [ ] Share on network                         |
|    SSH Port: [41222]  API Port: [41080]         |
|                                                 |
|  Storage                                        |
|    Data disk: 256 GB  [Resize]                  |
|    ZFS dedup: 1.23x   Compression: 2.45x       |
|                                                 |
|               [Save]  [Reset to Defaults]       |
+-------------------------------------------------+
```

**Setup flow (first launch / VM stopped):**
The main window shows the setup flow (download, license, start) since there's no iframe to show yet. Once the VM is running and API is ready, the setup UI is replaced by the iframe.

**How it works:**
- Wails supports system tray via `runtime.Menu` and tray APIs
- Main window = iframe only (when VM running)
- Main window = setup wizard (when VM not running)
- Settings = separate window or macOS sheet attached to main window
- Console = separate window (xterm.js in its own Wails window)
- Menu bar icon shows VM status via icon color (green dot = running, grey = stopped)

**Pros:**
- Zero wrapper UI overhead — the web app gets 100% of the window
- Feels truly native to macOS (menu bar apps are idiomatic)
- Clean separation: app window = product, menu bar = infrastructure
- Settings as a separate window is standard macOS convention (Preferences window)
- Console in its own window means it can be positioned independently

**Cons:**
- Requires Wails multi-window support or system tray APIs — need to verify Wails capabilities
- Menu bar apps are less discoverable for new users
- Setup flow in the main window → iframe swap is a jarring transition
- Users might close the main window expecting it to quit, but the menu bar app keeps running
- More complex implementation: multiple windows, tray management, window lifecycle

---

## Recommendation

**Option A (Titlebar Status Bar)** is the strongest balance of simplicity, discoverability, and space efficiency.

Reasoning:
- It solves the core problem completely — only one sidebar
- The implementation is straightforward (remove sidebar, add a thin bar, add a slide-over panel)
- It works within Wails' single-window model without needing multi-window or tray APIs
- The setup flow naturally fills the content area when there's no iframe
- It's the pattern Docker Desktop converged on after multiple redesigns
- The titlebar area is already used for the drag region, so this is additive, not disruptive

Option B is a solid fallback if Option A feels too constrained (e.g., if we need more controls visible at a glance). Option C is worth considering for v2 once the product is mature and users are familiar with it.

**Suggested implementation for Option A:**

1. Replace `<nav className="sidebar">` with `<header className="titlebar-controls">`
2. Move VM status, settings gear, and console button into the header
3. Convert SettingsView into a slide-over panel (position: fixed, right: 0, width: 380px)
4. Merge StorageView content into the settings panel under a "Storage" section
5. Console: bottom drawer (like Chrome DevTools) — 40% height, resizable, with a toggle button in the titlebar
6. Setup flow (download, license, start): full content area with centered card layout (already exists in HomeView)
7. Remove the sidebar entirely from daily use mode

The titlebar height would be ~40px to accommodate the traffic lights (which sit at ~14px from top on macOS) plus the controls. This is the same height the sidebar header already uses for its drag region.
