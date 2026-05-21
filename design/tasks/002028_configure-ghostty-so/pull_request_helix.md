# feat(desktop): make Ghostty respect system light/dark mode

## Summary

Adds a light/dark theme pair to the Ghostty terminal config used by the
Helix desktop image. When a user toggles the sun/moon button in the Helix
UI (which the settings-sync-daemon mirrors into the sandbox's GNOME via
`gsettings set ... color-scheme`), Ghostty now flips its palette to match
instead of staying on its default dark theme.

Uses Ghostty 1.3.1's native `theme = light:<name>,dark:<name>` syntax —
no scripts, no daemons, no reload signals. Ghostty subscribes to the
freedesktop `color-scheme` signal itself.

## Changes

- `desktop/ubuntu-config/ghostty-config`: append two lines
  - `theme = light:Catppuccin Latte,dark:Catppuccin Mocha`
  - `window-theme = auto` (also re-styles the GTK window chrome)
- All existing config (translucent background, font, padding, scrollback,
  keybindings, `gtk-single-instance = false`) is preserved unchanged.

## Testing

Built a new helix-ubuntu image (`./stack build-ubuntu` → tag `ac9115`),
registered an inner Helix account, spawned a fresh spec-task session,
and toggled the sun/moon button in the UI. Ghostty inside the embedded
desktop stream flipped from Mocha to Latte live, with no restart. User
confirmed visually.

## Screenshots

![Inner Helix light mode — sandbox Ghostty flipped](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002028_configure-ghostty-so/screenshots/07-inner-helix-light-mode-works.png)

## Notes

- This is the desktop *image* config, so the change takes effect for
  sandboxes started against the next built image. Existing running
  sandboxes keep their old config.
- Theme pair (Catppuccin Latte / Mocha) was picked for matched accent
  hues across modes and broad familiarity. Alternates considered:
  Rose Pine Dawn/Rose Pine, GitHub Light/Dark, Solarized.
