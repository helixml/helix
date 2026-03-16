# Implementation Tasks

- [ ] In `frontend/src/components/account/OAuthConnections.tsx` (~line 282), update the `GET /api/v1/oauth/flow/start/{provider}` call to append `?scopes=repo,read:org,read:user,user:email` for GitHub providers (and equivalent scopes for GitLab), mirroring the pattern in `BrowseProvidersDialog.tsx:400-408`
- [ ] (Optional) Extract the provider→scope mapping into `frontend/src/utils/oauthScopes.ts` and update `OAuthConnections.tsx`, `CreateProjectDialog.tsx`, and `BrowseProvidersDialog.tsx` to import from it, eliminating the three duplicate hardcoded scope strings
- [ ] Manually verify: connect GitHub from Account > OAuth Connections — the GitHub consent screen must show repo/org/user scopes
