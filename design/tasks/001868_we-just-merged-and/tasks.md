# Implementation Tasks

- [ ] Add `read:org` to `GITHUB_REQUIRED_SCOPES` in `frontend/src/components/project/BrowseProvidersDialog.tsx:74`
- [ ] Update GitHub PAT helper text (~line 1065) to mention `read:org` scope alongside `repo`
- [ ] Add `read:org` missing warning log in `api/pkg/server/git_repository_handlers.go` after `repo` scope check (~line 1719)
- [ ] Add warning log in `api/pkg/agent/skill/github/client.go:136` when `listUserOrganizations` fails (replace silent return)
- [ ] `cd frontend && yarn build` — verify no TypeScript errors
- [ ] `cd api && go build ./...` — verify no Go build errors
- [ ] Test OAuth scope upgrade flow with existing connection (verify banner appears, re-auth works)
- [ ] Test PAT flow with `repo`+`read:org` scopes (verify public org repos appear)
