# Implementation Tasks

- [~] In `frontend/src/contexts/theme.tsx`, simplify `getInitialMode()` to read only `window.matchMedia('(prefers-color-scheme: light)').matches` (no localStorage).
- [~] In `frontend/src/contexts/theme.tsx`, remove the `if (localStorage.getItem(THEME_MODE_KEY)) return` early-out from the `prefers-color-scheme` `change` listener so OS changes always update the mode and POST to the API.
- [~] In `frontend/src/contexts/theme.tsx`, remove the `localStorage.setItem(THEME_MODE_KEY, …)` call from the manual toggle handler so a manual click no longer locks out future OS changes.
- [~] Remove the `THEME_MODE_KEY` constant once nothing references it; add a one-line cleanup that deletes any leftover `themeMode` key from prior versions on load.
- [ ] Verify in the browser: first load matches OS theme; toggling OS appearance live updates the UI; clicking the in-app toggle flips the UI; a subsequent OS change re-syncs the UI to the OS.
- [ ] Verify the inner desktop: flipping macOS appearance updates GNOME color-scheme, GTK theme, wallpaper, and Zed theme inside the container within a few seconds.
