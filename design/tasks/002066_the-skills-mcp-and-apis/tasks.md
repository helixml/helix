# Implementation Tasks: Show Session-Restart Notice in Skills/MCP Editor

- [ ] Open `frontend/src/components/app/Skills.tsx` and locate the top of the main rendered output (above the skills grid, near current L1250).
- [ ] Add an MUI `<Alert severity="info">` containing the wording: *"Changes to MCP servers and API skills take effect in new sessions. Restart any active session to pick up updates."*
- [ ] Apply `sx={{ mb: 2 }}` (or matching spacing) so it sits cleanly above the existing grid.
- [ ] Confirm `Alert` is already imported from `@mui/material` in this file; add to the import if missing.
- [ ] Run the frontend (`pnpm dev` or equivalent) and verify the notice appears in **Project Settings → Skills tab**.
- [ ] Verify the notice also appears in **Agent (App) Settings → Skills tab** (same component, so this should be automatic).
- [ ] Verify the notice stays visible when switching between sub-category tabs (Core, MCP Servers, GitHub, etc.).
- [ ] Take a screenshot of each location and attach to the PR.
- [ ] Commit with a message like `frontend: add session-restart notice to Skills editor` and open a PR.
