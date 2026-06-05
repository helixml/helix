# Implementation Tasks: Make Ghostty Respect System Light/Dark Mode

- [x] Edit repo source `desktop/ubuntu-config/ghostty-config` in helix: append `theme = light:Catppuccin Latte,dark:Catppuccin Mocha` and `window-theme = auto` (preserves existing lines incl. `gtk-single-instance = false`)
- [x] Mirror the same two lines into the live `~/.config/ghostty/config` so the user can test interactively without rebuilding the container image
- [x] Reload Ghostty so the new settings take effect (`pkill -USR2 ghostty.real`)
- [x] Set system to light mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-light'`) and visually confirm Catppuccin Latte is active — screenshots/02-light-mode-latte.png
- [x] Switch system to dark mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'`) **without closing the window** and confirm Catppuccin Mocha takes over live — screenshots/03-dark-mode-mocha.png
- [x] Rebuild helix-ubuntu image so a fresh inner sandbox picks up the new ghostty config (`./stack build-ubuntu` — produced image `ac9115`)
- [x] Spawn a new inner Helix spec-task session via the Chrome MCP browser, toggle the in-UI sunshine button, confirm Ghostty inside the embedded desktop flips — user confirmed (screenshots/07-inner-helix-light-mode-works.png)
- [x] Restore the user's preferred system color scheme after testing (now `prefer-light` per their toggle; state owned by user)
- [x] Write PR description (`pull_request_helix.md`) and push code change to the feature branch
