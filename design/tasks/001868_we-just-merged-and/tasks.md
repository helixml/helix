# Implementation Tasks

- [x] Add `read:org` to `GITHUB_REQUIRED_SCOPES` in `frontend/src/components/project/BrowseProvidersDialog.tsx:74`
- [x] Update GitHub PAT helper text (~line 1065) to mention `read:org` scope alongside `repo`
- [x] Add `read:org` missing warning log in `api/pkg/server/git_repository_handlers.go` after `repo` scope check (~line 1719)
- [x] Add warning log in `api/pkg/agent/skill/github/client.go:136` when `listUserOrganizations` fails (replace silent return)
- [x] Update scope upgrade warning message to be generic (mentions all required scopes)
- [x] `cd frontend && npx tsc --noEmit` — verify no TypeScript errors
- [x] `cd api && go build ./pkg/agent/skill/github/ ./pkg/server/` — verify no Go build errors
- [ ] Test OAuth scope upgrade flow with existing connection (verify banner appears, re-auth works)
- [ ] Test PAT flow with `repo`+`read:org` scopes (verify public org repos appear)
