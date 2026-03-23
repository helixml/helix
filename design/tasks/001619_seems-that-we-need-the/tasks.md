# Implementation Tasks

- [x] In `api/pkg/server/git_provider_connection_handlers.go`, update `validateAndFetchUserInfo()` GitHub branch to also require `workflow` scope — check for it alongside `repo` and return a combined error listing all missing scopes
- [x] In `api/pkg/agent/skill/api_skills/github.yaml`, add `workflow` to `oauth.scopes`
- [x] In `api/pkg/agent/skill/api_skills/github_issues.yaml`, add `workflow` to `oauth.scopes`
- [x] In `frontend/src/components/project/BrowseProvidersDialog.tsx`, add `workflow` to the GitHub scopes parameter string (line ~407)
- [x] In `frontend/src/components/project/BrowseProvidersDialog.tsx`, add an inline scope-upgrade prompt: when an existing GitHub OAuth connection is missing `workflow`, show an alert with a "Reconnect with GitHub" button that starts the OAuth flow with the full required scopes (using the existing `hasRequiredScopes()` utility)
- [x] In `frontend/src/components/project/forms/ExternalRepoForm.tsx`, update PAT helper text to mention `workflow` scope requirement
- [x] Build and test: `go build ./...` in `api/`, TypeScript check clean in our files, pushed to feature branch
