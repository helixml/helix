# Project-level VCS connection lozenge (generic across providers)

**Date:** 2026-07-02
**Status:** Proposed
**Author:** design discussion (Luke + Claude)

## Summary

Spec-task pushes to external repos can fail *silently* when the acting user's
connected VCS account can't reach the repo. The agent reports success, the UI
shows nothing, and the user is left guessing (in the motivating incident, the
user correctly suspected he'd logged in with the wrong GitHub account, but had
no way to see or fix it).

Fix: a **per-provider connection lozenge on the project's Kanban board** that
shows who you're acting as, which VCS account you're pushing as, and whether
that account can actually reach the project's repo(s) — with connect / switch /
disconnect in the one control. Backed by a loud verify-and-prompt on the push
path (never a silent fallback) and a standardized connect scope set. The whole
thing is generic across VCS providers and only appears for providers the
project's repos actually use.

## Motivating incident (evidence)

Task `spt_01kwh8ty8fwgfj9xc5tbtwc7m9` (org `find-ai`, project
`prj_01kvz0e7b401545376fyyfxtta`), repo `code-find-ai-01kvz0e682…`,
external URL `https://github.com/helixml/find-ai.git` (private).

Timeline from the London prod API logs:

1. Agent wrote 3 spec files and pushed `helix-specs`; the internal git server
   returned success — the agent reported *"Design documents pushed and ready
   for review."*
2. Because the repo is external, the server then pushed `helix-specs` up to
   GitHub using the acting user's token and got:
   ```
   [GitPush] FAILED to push to external repository
   error="remote: Repository not found.
   fatal: repository 'https://github.com/helixml/find-ai.git/' not found"
   auth_type=basic:x-access-token
   ```
3. On upstream failure the server **rolled the local refs back** (removed the
   specs from the bare repo the UI reads), but the git client had already
   received its 200 — so nobody was told.

The acting user's connected GitHub accounts:

| GitHub login       | access to `helixml/find-ai` | when connected      |
|--------------------|------------------------------|---------------------|
| `linuxrecruit`     | **none** (→ 404 "not found") | during both failed pushes |
| `tonychapman-prog` | **write** (correct)          | after, once guided  |

The repo-level fallback credential (`o_auth_connection_id = cd7b50f6…`) is
`lukemarsden` (admin) — deliberately *not* used for user-initiated pushes (see
Non-goals). GitHub returns `404 Repository not found` — not `403` — for a
private repo the token can't see, which is why the error was so misleading.

## Root causes

1. **Silent rollback after a reported success.** `receive-pack` returns 200 to
   the git client *before* the external mirror push runs
   (`api/pkg/services/git_http_server.go:636-640`); when the mirror fails, refs
   are rolled back (`:688`) with no signal to the agent, the task, or the UI.
2. **No pre-flight access check.** Nothing verifies the acting user's connected
   account can reach the repo before burning a full planning run and a
   ~285s push-with-retries that then rolls back.
3. **Raw provider error surfaced verbatim.** "Repository not found" implies the
   repo doesn't exist when the real cause is *the connected account can't see
   it*.
4. **Discoverability.** "Connected Services" is a `FullScreenDialog` reached
   from the account menu (`frontend/src/pages/Layout.tsx:124-131`), not from
   where the connection-triggering action lives. The user couldn't find where to
   disconnect.
5. **Account re-selection.** GitHub OAuth reuses the browser's github.com
   session and silently re-attaches the *same* account on "connect" — the user
   reconnected the wrong account (`linuxrecruit`) a second time.
6. **Narrowed scopes.** The connection was stored with `["repo"]` only —
   missing `read:user`/`user:email` (identity) and `read:org` (org repo
   visibility), i.e. exactly the scopes a status lozenge needs to do its job.

## Design

### One lozenge row on the Kanban board, derived from the project's repos

Render **one lozenge per distinct VCS provider present among the project's
external repos**:

```
providers = distinct(repo.ExternalType) for repo in project.repos where repo.IsExternal
```

- Only GitHub repos → one GitHub lozenge.
- GitHub + GitLab repos → two lozenges.
- Local-only (non-external) repos contribute nothing.
- A provider with no repos in the project never appears (no dangling
  "Connect Azure DevOps" when there's no ADO repo).

Presence and state are separate axes: a lozenge shows because a repo of that
type exists; its *state* is one of:

| State            | Example label                                   | Action on click        |
|------------------|-------------------------------------------------|------------------------|
| Verified         | `⎇ helixml/find-ai · @tonychapman-prog ✓`       | menu (switch / disconnect / view) |
| Needs attention  | `⚠ @linuxrecruit · no access to helixml/find-ai`| Switch account / grant access |
| Disconnected     | `Connect GitHub`                                 | Connect (bound to this project) |

When the acting Helix user differs from the pushing VCS account, show both:
`acting as Tony · pushing as @tonychapman-prog`.

### Menu (connect / switch / disconnect in one place)

The lozenge's on-click/hover menu carries the whole lifecycle — putting
disconnect in the same area as the thing that triggers the connection:

- **Switch account** — must force the provider's account picker (don't just
  re-run OAuth against the existing session).
- **Reconnect** — re-auth, requesting the full scope set.
- **Disconnect** — *unbind this project* (safe default; connections are
  per-user/global today, so this must NOT revoke the user's token everywhere).
- **Remove from my account** — the revoke-entirely variant, clearly labelled,
  with a light confirm; or link out to the global Connected Services page.
- **View on \<provider\>** — open the account/repo in the provider UI.

### Acting-as-user, verify, fail loud — never silent fallback

Acting as the user on the provider for **user-initiated** actions is
intentional (attribution), so the credential resolver trying the acting user's
connection first (`api/pkg/services/git_repository_service.go:2659`) is correct
by design. The corollary:

- When the acting user's connected account **cannot reach the repo**, the push
  must **fail loudly and prompt to switch accounts** — it must *not* fall back
  to the repo-level/admin credential. A fallback would break attribution *and*
  be an authorization bypass (pushing to a repo the user's own account can't
  access).
- The repo-level connection (`o_auth_connection_id`) remains the credential for
  **system/agent-initiated** (non-user) actions only.

### Generic across providers

Everything provider-specific comes from the provider abstraction
(`types.ExternalRepositoryType` {GitHub, GitLab, ADO, Bitbucket}, the
`o_auth_providers` registry, `api/pkg/oauth/provider.go` incl. per-connect
scopes via `GetAuthorizationURL(..., scopes)`), not from GitHub-shaped code in
the shared lozenge. Each provider supplies a small capability set:

| Capability        | GitHub                               | GitLab                | ADO / Bitbucket |
|-------------------|--------------------------------------|-----------------------|-----------------|
| auth mechanism    | `x-access-token`                     | `oauth2`              | PAT / app-password (`getCredentialsForRepo:2676`) |
| required scopes   | `repo`, `read:user`, `user:email`, `read:org` | `api`, `read_repository`, `write_repository` | provider-specific |
| identity handle   | login (`@x`)                         | username              | org/user, workspace |
| access-verify probe | `GET /repos/{o}/{r}`               | `GET /projects/:id`   | provider equivalent |

The generic layer holds the behavior (act as user → verify → fail-loud/prompt →
connect/switch/disconnect) and renders one lozenge per provider the project
uses. Adding a provider = adding a capability entry, no changes to the shared
component.

### Scopes (GitHub)

`repo` alone is insufficient and sets users up for failure. The connect flow
must request `repo` + `read:user` + `user:email` + `read:org`:

- `repo` — clone/push/PR (already validated at
  `sample_project_access_handlers.go:257`, but that's the *validation* set, too
  narrow to store as the *requested* set).
- `read:user` + `user:email` — identity for the "logged in as @x" display
  (already the intended set at `simple_sample_projects.go:731`).
- `read:org` — org repo listing / access verification; degraded without it
  (`git_repository_handlers.go:1790`) and needed to tell the user *"your
  @linuxrecruit account isn't in the helixml org."*

## Backend work required (the UI alone won't fix it)

1. **Surface the silent rollback.** On external-push failure
   (`git_http_server.go:680-690`), persist the error on the task (e.g.
   `last_push_error`), reflect it as a task/board error state, and feed it back
   to the agent so it stops claiming success it can't verify.
2. **Pre-flight + on-failure verify-and-prompt.** Add a provider access-verify
   probe (interface method per provider). Check before kicking off planning and
   translate a push auth failure into an actionable "switch account" prompt on
   the lozenge — never a silent fallback for user-initiated actions.
3. **Standardize connect scopes** to the full per-provider set; stop storing
   `["repo"]`.
4. **Force account re-selection** in the connect/switch flow (provider account
   picker), so reconnect can't silently re-attach the wrong account.
5. **Project↔provider-connection binding surfaced** for the lozenge (the repo
   already carries `o_auth_connection_id`); expose "acting as" vs "pushing as"
   and the verified-access state via an API the board consumes.

## Non-goals / decided

- **Do not** make a repo/project-level credential authoritative over the acting
  user for user-initiated pushes — acting-as-user is intentional.
- **Do not** silently fall back to another credential when the user's account
  lacks access — fail loud.
- Keep OAuth connections per-user/global (don't duplicate tokens per project);
  "project-level" = a per-project binding + verified-access surface, not a
  separate credential store.

## Open questions

- Multi-repo project with several repos of the *same* provider but *different
  access*: one lozenge with an aggregated/worst-case state, or per-repo detail
  in the menu?
- Where exactly on the board chrome the lozenge row sits (top-right), and how it
  coexists with existing board controls.
- Whether the pre-flight check runs at repo-link time, at "Start planning", or
  both.

## Key references

- `api/pkg/services/git_http_server.go:636-640` (200 before mirror push),
  `:641-692` (external push + rollback), `:688` (rollback)
- `api/pkg/services/git_repository_service_push.go:60,85,102` (credential use,
  push, failure)
- `api/pkg/services/git_repository_service.go:2653-2719` (`getCredentialsForRepo`;
  `:2659` acting-user-first, `:2673` repo-level, `:2676` per-provider auth)
- `api/pkg/server/simple_sample_projects.go:731` (intended scope set)
- `api/pkg/server/sample_project_access_handlers.go:257` (`GitHubRepoScopes`)
- `api/pkg/server/git_repository_handlers.go:1790` (`read:org` needed)
- `api/pkg/oauth/provider.go:22` (`GetAuthorizationURL(..., scopes)`)
- `frontend/src/pages/Layout.tsx:124-131` (Connected Services dialog — current
  discoverability)
