# Implementation Tasks

- [ ] In `api/pkg/server/git_provider_connection_handlers.go`, update `validateAndFetchUserInfo()` GitHub branch to also require `workflow` scope — check for it alongside `repo` and return a combined error listing all missing scopes
- [ ] In `api/pkg/agent/skill/api_skills/github.yaml`, add `workflow` to `oauth.scopes`
- [ ] In `api/pkg/agent/skill/api_skills/github_issues.yaml`, add `workflow` to `oauth.scopes`
- [ ] In `frontend/src/components/project/BrowseProvidersDialog.tsx`, add `workflow` to the GitHub scopes parameter string (line ~407)
- [ ] In `frontend/src/components/project/BrowseProvidersDialog.tsx`, add an inline scope-upgrade prompt: when an existing GitHub OAuth connection is missing `workflow`, show an alert with a "Reconnect with GitHub" button that starts the OAuth flow with the full required scopes (using the existing `hasRequiredScopes()` utility)
- [ ] In `frontend/src/components/project/forms/ExternalRepoForm.tsx`, update PAT helper text to mention `workflow` scope requirement
- [ ] Build and test: `go build ./...` in `api/`, `cd frontend && yarn build`, verify PAT validation rejects tokens missing `workflow`, verify inline upgrade prompt appears for existing connections
