# Implementation Tasks

- [ ] In `frontend/src/components/account/OAuthConnections.tsx`, remove the "Available Integrations" section and all connect-initiation UI (the section around lines 782-816 that lists providers with Connect buttons)
- [ ] Delete the `openConnectDialog()`, `startOAuthFlow()`, and `connectProvider` functions from `OAuthConnections.tsx` (~lines 222-282), and any state/hooks they depend on that are no longer used
- [ ] Verify the Connected Services list (existing connections with disconnect/refresh) still renders correctly after removal
- [ ] Check `frontend/src/pages/OAuthConnectionsPage.tsx` for any references to the removed connect functionality and clean up if needed
- [ ] Manually verify: Account > Connected Services page shows existing connections only, with no way to initiate a new connection; existing contextual flows (BrowseProvidersDialog, CreateProjectDialog) still work
