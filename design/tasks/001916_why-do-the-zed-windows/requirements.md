# Requirements: Zed window size regression

## User story

As a Helix Desktop user, when I open a session and Zed launches, I want it to appear as a reasonably-sized window (not full screen, not taller than the visible streaming viewport), so the desktop matches the windowed feel I had previously and so I can see other apps (Chrome, terminals) alongside Zed.

## Problem

Zed currently launches **full screen** in the Sway desktop. When the user un-fullscreens it, the window is **taller than the visible streamed viewport** (i.e., the bottom of the window is clipped or scrolls off the user's browser). The user expects a floating, sub-screen-sized Zed window like they had before.

The user suspects this regression coincides with the recent Chrome viewport change (task 001532, commit `53715951c`) which added `--viewport 1600x1080` to chrome-devtools-mcp. They believe it should be unrelated, and they are correct that Chrome's CDP `page.resize` does not directly resize Zed — but the underlying cause is **the Sway desktop has no floating rules for any GUI app**, so all apps (Zed, Chrome, terminals) tile to fill their workspace by default. The Chrome viewport change just made this newly-noticeable for Chrome; Zed has the same root cause.

## Acceptance criteria

- When a session opens, the Zed window starts as a **floating, centred window** sized smaller than the Sway output (target ~1600×900, leaving margin on all sides of the 1920×1080 inner desktop).
- When un-fullscreened, the Zed window is **strictly smaller** than `GAMESCOPE_HEIGHT` (i.e., not taller than the streamed viewport).
- Chrome continues to work for the chrome-devtools MCP — viewport `--viewport 1600x1080` is still honoured for the rendered page, and Chrome's window is not larger than the desktop.
- The user can still tile, maximize, or fullscreen Zed manually using normal Sway keybindings if they want to.
- Behavior persists across Sway crash/restart (the fix lives in `desktop/sway-config/`, which is reapplied on every session start).

## Out of scope

- Zed-side initial window size code (`build_window_options` in `crates/zed/src/zed.rs`) — Zed sets `window_bounds: None`, deferring to the WM. The fix belongs in Sway config, not Zed.
- Reverting the Chrome viewport task — it's a real fix and should stay.
- Investigating whether Zed's persisted window state (`~/.config/zed`, symlinked to `$WORK_DIR/.zed-state`) was ever floating in the past — Sway tiling has been the default since at least the `desktop/` rename in January, so the user's "previously" memory is likely from an earlier desktop (e.g. Ubuntu/GNOME).
