# Implementation Tasks: Make Ghostty Respect System Light/Dark Mode

- [ ] Edit `~/.config/ghostty/config`: append `theme = light:Catppuccin Latte,dark:Catppuccin Mocha` and `window-theme = auto` (preserve all existing lines, especially `gtk-single-instance = false` and its comment)
- [ ] Verify no conflicting `theme`, `background`, or `foreground` lines exist in the config after the edit
- [ ] Reload Ghostty (open a new window or use the in-app config reload) so the new settings take effect
- [ ] Set system to light mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-light'`) and visually confirm Catppuccin Latte is active — prompt, `ls` output colors, selection highlight, window chrome
- [ ] Switch system to dark mode (`gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'`) **without closing the window** and confirm Catppuccin Mocha takes over live
- [ ] Show both modes to the user for sign-off; if they reject Catppuccin, swap to one of the alternates from design.md (`Rose Pine Dawn`/`Rose Pine`, `GitHub Light Default`/`GitHub Dark Default`, or Solarized) and retest
- [ ] Restore the user's preferred system color scheme after testing
