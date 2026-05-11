# Implementation Tasks

- [ ] In `frontend/src/contexts/theme.tsx`, replace binary `mode` state with `themePreference: 'light' | 'dark' | 'system'` plus a derived `resolvedMode: 'light' | 'dark'`.
- [ ] In `frontend/src/contexts/theme.tsx`, default new users to `'system'` and one-time-migrate the legacy `themeMode` localStorage key into the new `themePreference` key.
- [ ] In `frontend/src/contexts/theme.tsx`, make the `prefers-color-scheme` `change` listener update `resolvedMode` whenever `themePreference === 'system'` (not only when localStorage is empty), and POST the resolved color to `/api/v1/users/me/color-scheme` on every change.
- [ ] Update `frontend/src/hooks/useLightTheme.tsx` (and any other consumers) to read `resolvedMode` instead of the old `mode`.
- [ ] In `frontend/src/components/system/Page.tsx`, replace the binary toggle button with a Light/Dark/System control (dropdown menu preferred; a 3-state cycling icon with tooltip is acceptable). Show the current selection.
- [ ] Verify in the browser: first load with no preference uses OS theme; toggling OS appearance live updates the UI; selecting Light or Dark pins it; selecting System resumes OS following.
- [ ] Verify the inner desktop: with the Helix UI in System mode, flipping macOS appearance updates GNOME color-scheme, GTK theme, wallpaper, and Zed theme inside the container within a few seconds.
- [ ] Verify cross-device behavior: explicit Light/Dark choice persists after sign-out/sign-in; System choice does not pin a fixed color server-side.
