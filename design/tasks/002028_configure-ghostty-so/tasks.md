# Implementation Tasks: Make Ghostty Respect System Light/Dark Mode

- [~] Edit repo source `desktop/ubuntu-config/ghostty-config` in helix: append `theme = light:Catppuccin Latte,dark:Catppuccin Mocha` and `window-theme = auto` (preserves existing lines incl. `gtk-single-instance = false`)
- [ ] Mirror the same two lines into the live `~/.config/ghostty/config` so the user can test interactively without rebuilding the container image
- [ ] Reload Ghostty so the new settings take effect (`pkill -USR2 ghostty` or open a new window)
- [ ] Set system to light mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-light'`) and visually confirm Catppuccin Latte is active
- [ ] Switch system to dark mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'`) **without closing the window** and confirm Catppuccin Mocha takes over live
- [ ] Show both modes to the user for sign-off; if rejected, swap to an alternate pair from design.md and retest
- [ ] Restore the user's preferred system color scheme after testing
- [ ] Write PR description (`pull_request_helix.md`) and commit code change to the feature branch
