# Requirements: Zed window size regression on Ubuntu desktop

## User story

As a Helix Desktop user on the **Ubuntu / GNOME** sandbox, when a session opens and Zed launches, I want it to appear as a reasonably-sized centred window — not full-screen, and not taller than the streamed viewport — so the desktop feels windowed (the way it used to) and so I can see Zed alongside Chrome and the terminal. Chrome should likewise open at a sub-screen size, not at almost the full screen width.

## Problem

Zed currently launches **maximised / "full screen"** in every new spectask on the Ubuntu desktop. (Every spectask starts with a fresh `~/` and `~/work`, so this is the first-launch behaviour, not stale persisted state.) When the user un-maximises Zed by hand, the window is **taller than the visible streamed viewport** — the bottom of the editor is clipped or scrolls off the user's browser.

The regression started with the **001864 Zed merge** (2026-04-23/24), which pulled in upstream commit `a0d0195ca9` (2026-04-07, PR #52940). That commit changed Zed's `DEFAULT_WINDOW_SIZE` from `1536×864` to `1536×1095`. On a 1920×1080 virtual monitor the new value clips to `1536×1080` — exactly the screen height — which trips GNOME Mutter's `auto-maximize=true` and the user sees a Maximised editor. Before this Zed bump, `1536×864` opened comfortably as a centred floating window with margin around it (the "used to start nicely in the middle of the screen" behaviour the user remembers).

The user's hunch about the Chrome viewport change (task 001532, commit `53715951c`) is half-right: it's not the cause, but it landed at the same time and shares the same auto-maximise dynamic. Chrome's `--viewport 1600x1080` produces a ~1600×1160 window (1080 page + ~80 chrome decorations), which also trips Mutter's threshold and makes Chrome look fullscreen. Chrome only needs ≥ 1280 px wide for sites to render in desktop mode, so the viewport can comfortably shrink.

The fix needs to **scale with the user's display**: users can pick any virtual monitor (1920×1080, 4K, 5K) and any zoom level. A hardcoded window size would look right at one resolution and wrong everywhere else.

## Acceptance criteria

- When a new spectask opens, Zed launches as a **centred, windowed (not maximised, not fullscreen) window** sized to ≈80% of the logical work area (e.g. 1536×864 on a 1920×1080 monitor at 100% zoom — exactly the dimensions Zed used to default to).
- The same proportions hold on **4K, 5K, and HiDPI sessions**: the formula is `0.8 × (GAMESCOPE_WIDTH/GDK_SCALE) × 0.8 × (GAMESCOPE_HEIGHT/GDK_SCALE)`, centred with a 10% margin on each side.
- When the user un-maximises Zed by hand, the window is **strictly smaller than the work area** in both axes so it cannot be clipped off the streamed viewport.
- The user can still maximise / fullscreen Zed manually via the title-bar buttons or normal GNOME shortcuts if they want to.
- The chrome-devtools MCP viewport is reduced from `1600x1080` to `1280x800` — still triggers desktop-mode rendering on standard sites, but leaves room around Chrome on a 1920×1080 monitor and stays below the auto-maximise threshold. Chrome stays static (not proportional) because the viewport is set in API-side Go code that doesn't see per-session display env vars; this is a deliberate scope decision.
- The Chrome viewport task (001532) is **not reverted** — its underlying fix (passing `--viewport` as a CLI arg) stays; only the dimensions change.
- The fix lives in the Helix repo only — no changes in `/home/retro/work/zed/`.

## Out of scope

- Changing Zed's `build_window_options` (it already returns `window_bounds: None`, deferring to the WM / env override).
- Sway desktop — its window-tiling behaviour is a separate dynamic with a separate fix; this task is Ubuntu only.
- Auditing every other GUI app (ghostty, Nautilus, etc.). They share the auto-maximise dynamic, but the user's complaint is specifically about Zed and Chrome; widening scope further adds risk for no benefit.
