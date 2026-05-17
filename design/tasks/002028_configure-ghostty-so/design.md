# Design: Make Ghostty Respect System Light/Dark Mode

## Approach

Ghostty 1.3.1 natively supports per-mode themes via the special
`theme = light:<name>,dark:<name>` syntax (documented in
`ghostty +show-config --default --docs`). When this form is used, Ghostty
listens for the desktop's color-scheme signal (on Linux via the
`org.freedesktop.appearance` portal / GTK's `color-scheme` setting) and swaps
the active palette live, without restart.

So the entire change is **two config lines**: pick a light theme, pick a dark
theme, plus make sure `window-theme = auto` (which becomes equivalent to
`system` when light/dark themes are specified, and also re-styles the GTK
window chrome).

No scripts, no daemons, no per-session reload logic — Ghostty does it.

## Theme Choice

Ghostty ships a large library of themes (`ghostty +list-themes`). The pair
needs to (a) come from the same visual family so the switch feels coherent,
(b) be widely considered tasteful, and (c) have good contrast for both
syntax-highlighted code and plain shell output.

**Recommended default:** `light:Catppuccin Latte,dark:Catppuccin Mocha`

Why Catppuccin:
- Most popular community-maintained theme family across editors/terminals;
  the user is very likely to already recognize it.
- Latte (light) uses soft pastel accents on a warm off-white background —
  much easier on the eyes than a stark white like GitHub Light.
- Mocha (dark) is a well-balanced muted dark that's a clear upgrade over
  Ghostty's plain default.
- Both palettes use the same accent hue assignments, so colored output
  (e.g. `ls`, `git status`) keeps the same semantic colors across modes.

**Alternates to offer during interactive review** if the user dislikes
Catppuccin:
- `light:Rose Pine Dawn,dark:Rose Pine` — softer, lower contrast, very muted.
- `light:GitHub Light Default,dark:GitHub Dark Default` — clean, professional,
  matches GitHub.com.
- `light:iTerm2 Solarized Light,dark:Solarized Dark` — the classic Ethan
  Schoonover palette; warm beige light mode.

The spec recommends Catppuccin but the final pick is whatever the user
approves after seeing both modes live.

## Final config additions

Two lines appended to `~/.config/ghostty/config`:

```
theme = light:Catppuccin Latte,dark:Catppuccin Mocha
window-theme = auto
```

`window-theme = auto` is the default, but we set it explicitly so the
intent is documented and accidental future overrides are obvious.

## Key Notes & Gotchas

- **Don't set a single `theme = <name>`** alongside the light/dark form —
  only one `theme` line should exist. Replacing, not adding.
- **Don't set `background` or `foreground` overrides** in the same config:
  per the Ghostty docs, those override the theme's colors and would defeat
  the auto-switching. The current config doesn't set them, so we're fine.
- **`background-opacity = 0.9` is preserved.** Opacity is independent of
  theme; both light and dark backgrounds will be 90% opaque.
- **Live switch on Linux** requires the desktop to publish color-scheme
  changes via the freedesktop portal. GNOME (Yaru, the current setup)
  does this; if a user is on a minimal WM it may require a manual Ghostty
  reload (Ctrl+Shift+, → reload config), but that's an edge case and not
  in scope.
- **Verifying current state:**
  `gsettings get org.gnome.desktop.interface color-scheme` returns
  `'prefer-light'` or `'prefer-dark'`; toggling it should flip Ghostty
  immediately during interactive testing.

## Interactive Test Plan

1. Apply the new config lines, reload Ghostty (or open a new window).
2. With system in light mode (`gsettings set ... color-scheme
   'prefer-light'`), confirm Latte is showing — check prompt, `ls` colors,
   selection highlight.
3. Switch system to dark mode (`gsettings set ... color-scheme
   'prefer-dark'`) **without closing Ghostty** and confirm Mocha takes
   over live in the existing window.
4. Show both modes to the user; if approved, done. If not, swap the theme
   pair to one of the alternates and re-test.

## Discovery Notes (for future agents)

- Ghostty config lives at `~/.config/ghostty/config` and uses a simple
  `key = value` plaintext format; no JSON/TOML, no includes needed for this
  task.
- The Helix Desktop ghostty config has a load-bearing comment about
  `gtk-single-instance = false` — don't remove it; it's needed for
  `launch_terminal()` in `start-zed-helix.sh` to work via `-e`.
- `ghostty +list-themes` enumerates everything available; `ghostty
  +show-config --default --docs` prints inline docs for every option,
  which is the authoritative reference (works offline, beats the web docs
  for spec work).
