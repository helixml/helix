# Implementation Tasks: Make Ghostty Respect System Light/Dark Mode

- [x] Edit repo source `desktop/ubuntu-config/ghostty-config` in helix: append `theme = light:Catppuccin Latte,dark:Catppuccin Mocha` and `window-theme = auto` (preserves existing lines incl. `gtk-single-instance = false`)
- [x] Mirror the same two lines into the live `~/.config/ghostty/config` so the user can test interactively without rebuilding the container image
- [x] Reload Ghostty so the new settings take effect (`pkill -USR2 ghostty.real`)
- [x] Set system to light mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-light'`) and visually confirm Catppuccin Latte is active — screenshots/02-light-mode-latte.png
- [x] Switch system to dark mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'`) **without closing the window** and confirm Catppuccin Mocha takes over live — screenshots/03-dark-mode-mocha.png
- [~] Show both modes to the user for sign-off; if rejected, swap to an alternate pair from design.md and retest
- [ ] Restore the user's preferred system color scheme after testing (currently `prefer-dark`, matches state before testing)
- [ ] Write PR description (`pull_request_helix.md`) and commit code change to the feature branch
