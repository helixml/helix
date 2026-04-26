# Requirements: Zed window size regression on Ubuntu desktop

## User story

As a Helix Desktop user on the **Ubuntu / GNOME** sandbox, when a session opens and Zed launches, I want it to appear as a reasonably-sized centred window — not full-screen, and not taller than the streamed viewport — so the desktop feels windowed (the way it used to) and so I can see Zed alongside Chrome and the terminal. Chrome should likewise open at a sub-screen size, not at almost the full screen width.

## Problem

Zed currently launches **maximised / "full screen"** in every new spectask on the Ubuntu desktop (every spectask starts with a fresh `~/` and `~/work`, so this is the first-launch behaviour). When the user un-maximises Zed by hand, the window is **taller than the visible streamed viewport** — the bottom of the editor is clipped or scrolls off the user's browser.

The user suspects this regression is connected to the recent Chrome viewport change (task 001532, commit `53715951c`), which added `--viewport 1600x1080` to chrome-devtools-mcp. They are correct that Chrome's `page.resize` does not directly resize Zed, but they have correctly identified a **shared root cause**: any GUI app that opens with a window at or above the GNOME work area is auto-maximised by Mutter (default `org.gnome.mutter auto-maximize=true`). Zed asks the WM for an unconstrained default size and lands on that threshold; the user sees a Maximised editor.

Separately, the `--viewport 1600x1080` chosen in 001532 is itself **too big**: Chrome's window ends up ~1600 wide × ~1160 tall (1080 page + ~80 chrome) on a 1920×1080 monitor, so Chrome covers most of the screen and trips the same Mutter auto-maximise threshold. Chrome only needs ≥ 1280 px wide for sites to render in desktop mode, so the viewport can comfortably shrink.

## Acceptance criteria

- When a new spectask opens, Zed launches as a **centred, windowed (not maximised, not fullscreen) window** sized comfortably smaller than the GNOME virtual monitor (e.g. ≈1600×900 inside a 1920×1080 monitor).
- When the user un-maximises Zed by hand, the window is **strictly smaller than `GAMESCOPE_HEIGHT`** so it cannot be clipped off the bottom of the streamed viewport.
- The user can still maximise / fullscreen Zed manually via the title-bar buttons or normal GNOME shortcuts if they want to.
- The chrome-devtools MCP viewport is reduced from `1600x1080` to a smaller value (≈ `1280x800`) that still triggers desktop-mode rendering on standard sites but leaves room around Chrome on a 1920×1080 monitor and stays below the auto-maximise threshold.
- The Chrome viewport task (001532) is **not reverted** — its underlying fix (passing `--viewport` as a CLI arg) stays; only the dimensions change.
- The fix lives in the Helix repo only — no changes in `/home/retro/work/zed/`.

## Out of scope

- Changing Zed's `build_window_options` (it already returns `window_bounds: None`, deferring to the WM / env override).
- Sway desktop — its window-tiling behaviour is a separate dynamic with a separate fix; this task is Ubuntu only.
- Auditing every other GUI app (ghostty, Nautilus, etc.). They share the auto-maximise dynamic, but the user's complaint is specifically about Zed and Chrome; widening scope further adds risk for no benefit.
