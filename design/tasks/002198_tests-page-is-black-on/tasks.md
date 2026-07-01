# Implementation Tasks: Fix Tests Tab Dark-on-Dark Colors in Light Mode (Agent Settings)

- [x] Import and call `useLightTheme()` in `frontend/src/components/app/TestsEditor.tsx`
- [x] Replace per-test card background `#2a2d3e` with theme-aware `panelColor`
- [x] Replace per-step card background `#1e1e2f` with theme-aware inner background
- [x] Replace "Running Tests with CLI" panel background `#2a2d3e` with `panelColor`
- [x] Replace CLI command snippet box background `#1e1e2f` with theme-aware inner background
- [x] Replace GitHub & GitLab accordion backgrounds `#1e1e2f` with theme-aware value
- [x] Replace GitHub & GitLab code block backgrounds `#0d1117` with an `isLight`-conditional light/dark code background
- [x] Replace `color: 'white'` on the three copy IconButtons with theme-aware `icon` color
- [x] Build the frontend (typecheck via `helix-frontend-1` container) and confirm it compiles
- [x] Verify in the inner Helix: Tests tab is readable in light mode (no dark-on-dark)
- [x] Verify in the inner Helix: Tests tab is unchanged in dark mode; capture before/after screenshots
