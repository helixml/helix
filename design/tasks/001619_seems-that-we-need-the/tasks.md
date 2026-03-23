# Implementation Tasks

- [ ] In `api/pkg/server/git_provider_connection_handlers.go`, update `validateAndFetchUserInfo()` GitHub branch to also require `workflow` scope — check for it alongside `repo` and return a combined error listing all missing scopes
- [ ] In `api/pkg/agent/skill/api_skills/github.yaml`, add `workflow` to `oauth.scopes`
- [ ] In `api/pkg/agent/skill/api_skills/github_issues.yaml`, add `workflow` to `oauth.scopes`
- [ ] In `frontend/src/components/project/forms/ExternalRepoForm.tsx`, update PAT helper text to mention `workflow` scope requirement (e.g. "needs repo and workflow scopes")
- [ ] Build and test: `go build ./...` in `api/`, `cd frontend && yarn build`, verify PAT validation rejects tokens missing `workflow`
