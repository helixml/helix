# Requirements: Fix GitHub OAuth Missing Repo Scope

## Problem

When users connect GitHub via the **Account > OAuth Connections** page, the OAuth authorization request is sent to GitHub *without* any scope parameters. GitHub then grants only default access (public repos, read-only), so private repositories and write operations fail silently.

The project and browse flows work correctly because they explicitly pass `?scopes=repo,read:org,read:user,user:email` to the OAuth start endpoint — but `OAuthConnections.tsx` omits this entirely.

## User Stories

**As a user connecting GitHub from Account Settings,**
I want the OAuth flow to request repo access,
so that I can access my private repositories and the integration works the same as when connecting via the project dialog.

**As a developer,**
I want scope defaults to live in one authoritative place per provider,
so that adding a new OAuth entry point doesn't silently regress permissions.

## Acceptance Criteria

- [ ] Connecting GitHub via Account > OAuth Connections page results in GitHub requesting the `repo`, `read:org`, `read:user`, and `user:email` scopes (visible in the GitHub OAuth consent screen).
- [ ] The existing project flows (`CreateProjectDialog`, `BrowseProvidersDialog`) are unaffected.
- [ ] If a user has a stale GitHub connection (no repo scope), they are prompted to re-authorize when repo access is needed (existing `getMissingScopes` logic handles this — no new work required).
