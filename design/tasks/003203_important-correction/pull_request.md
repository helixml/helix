# fix(api): scope organisation API keys to their organization

## Summary

Organisation API keys were organisation-labelled but not organisation-restricted: the key authenticated as the full user record of whoever created it. A key created by a global admin inherited the admin crown (every organization plus admin endpoints), and a key created by a user belonging to multiple organizations reached their other organizations and personal resources' org surfaces. Organisation API keys are now scoped to their organization no matter who creates them.

Two root-cause changes:

- `api/pkg/server/auth_middleware.go` — in `getUserFromToken`, an org-labelled API key (`apiKey.OrganizationID != ""`) no longer carries the creator's global admin flag. The bearer acts as the key owner inside its own org with normal membership rules; admin power is decided by membership, not by the creator's global status. Personal API keys (no org label) keep the previous behavior and are unaffected.
- `api/pkg/orgstore/authz.go` — new `enforceKeyOrgScope` guard in `AuthorizeOrgOwner` and `AuthorizeOrgMember`: credentials scoped to one organization are refused for any other organization. This is the single choke point shared by the API server and downstream services, so every org-authorization path (org handlers, projects, apps, repositories, sessions, sandboxes, git, filestore) is covered without touching individual handlers.
- List endpoints default to the key's organization for organization API keys (four hunks ported from the closed PR #3228 so the fix is self-contained): unfiltered `listAgents`, `listProjects`, `listGitRepositories` and `listSessions` requests now scope to the bearer key's organization instead of falling through to the key owner's personal and cross-org resources.

Known residual (deliberate): an org-key bearer still authenticates as the creator for org-less *personal* resources via `user.ID == owner` shortcuts. Blocking that would touch dozens of owner checks across handlers and is out of scope for this fix.

## Testing

- New unit tests in `api/pkg/orgstore/authz_test.go`: org-scoped credential denied on another organization (member and owner paths), allowed in its own org with the correct membership role, and unscoped users unaffected. The fake store seeds the other organization's owner membership, so the cross-org denial comes from the guard itself — verified by neutralizing `enforceKeyOrgScope`, which makes the negative tests fail.
- List-fallback behavior ported from PR #3228 carries that PR's reviewed hunks; guard and list paths covered by the full server suite.
- New unit tests in `api/pkg/server/auth_middleware_test.go`: an org API key does not inherit its creator's global admin (owner is admin via `ADMIN_USER_IDS=all` and the DB record), while a personal API key still does; the admin lookup is verified skipped for org keys via mock call counts.
- Regression suites: `go test ./pkg/server/ ./pkg/orgstore/ ./pkg/services/ ./pkg/sandbox/ ./pkg/controller/ -count=1` — all pass.
- `go build ./pkg/server/ ./pkg/orgstore/ ./pkg/types/` clean; repo-wide build failures are limited to pre-existing GStreamer packages needing `pkg-config` (unrelated).
