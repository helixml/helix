# Implementation Tasks: Fix Tests Tab Dark-on-Dark Colors in Light Mode (Agent Settings)

- [~] Import and call `useLightTheme()` in `frontend/src/components/app/TestsEditor.tsx`
- [~] Replace per-test card background `#2a2d3e` with theme-aware `panelColor`
- [~] Replace per-step card background `#1e1e2f` with theme-aware inner background
- [~] Replace "Running Tests with CLI" panel background `#2a2d3e` with `panelColor`
- [~] Replace CLI command snippet box background `#1e1e2f` with theme-aware inner background
- [~] Replace GitHub & GitLab accordion backgrounds `#1e1e2f` with theme-aware value
- [~] Replace GitHub & GitLab code block backgrounds `#0d1117` with an `isLight`-conditional light/dark code background
- [~] Replace `color: 'white'` on the three copy IconButtons with theme-aware `icon` color
- [ ] Build the frontend (`cd frontend && yarn build`) and confirm it compiles
- [ ] Verify in the inner Helix: Tests tab is readable in light mode (no dark-on-dark)
- [ ] Verify in the inner Helix: Tests tab is unchanged in dark mode; capture before/after screenshots
