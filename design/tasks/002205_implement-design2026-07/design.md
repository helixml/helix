# Design: Project VCS Connection Lozenge & Loud Push-Failure Surfacing

## Overview

Two coupled but independently-shippable workstreams:

- **A. Loud push-failure surfacing** (backend + a task error state) — removes the
  silent-success footgun. Ships first, on its own.
- **B. Project VCS connection lozenge** (generic across providers) — a Kanban
  board control backed by a per-provider access-verify probe, forced account
  selection, standardized scopes, and a project↔connection binding surface.

## Current-state facts (verified in code)

| Concern | Location | Today |
|---|---|---|
| 200 before mirror push | `git_http_server.go:614-617` | Client gets 200 pre-mirror |
| External push + rollback | `git_http_server.go:635-691`, `rollbackBranchRefs` `:775-805` | Refs rolled back on failure, no client/task/UI signal |
| Push credential use | `git_repository_service_push.go:50-114` | `getCredentialsForRepo` → `pushBranchNative`; error only logged |
| Credential resolution | `git_repository_service.go:2659-2717` | Acting-user OAuth first (correct), repo-level `OAuthConnectionID` fallback |
| Provider interface | `oauth/provider.go:13-33` | `GetAuthorizationURL(...scopes []string)` already supports scope override |
| Requested scopes (sample) | `simple_sample_projects.go:731` | `["repo","read:user","user:email"]` — missing `read:org` |
| Validation scopes | `sample_project_access_handlers.go:257` | `GitHubRepoScopes = ["repo"]` |
| read:org degradation | `git_repository_handlers.go:1782-1791` | Warns, continues (incomplete org listing) |
| Connected Services UI | `Layout.tsx:123-131` | `FullScreenDialog` → `<OAuthConnections/>` |
| Repo type/fields | `types/git_repositories.go:57-137` | `IsExternal`, `ExternalURL`, `ExternalType`, `OAuthConnectionID` |
| OAuth connection | `types/oauth.go:56-79` | Has `Scopes`, `ProviderUsername`, `ProviderUserID`, `Profile` |
| Task push fields | `types/simple_spec_task.go:195-196` | `LastPushCommitHash`, `LastPushAt` — **no error field** |
| Kanban board | `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Primary board component |

## Workstream A — Loud push-failure surfacing

### A1. Persist a structured push error on the task
Add to `SpecTask` (`types/simple_spec_task.go`, GORM AutoMigrate):

```go
LastPushError *PushError `json:"last_push_error" gorm:"type:text;serializer:json"`
```
```go
type PushError struct {
    Provider     ExternalRepositoryType `json:"provider"`
    Account      string                 `json:"account"`      // e.g. "@linuxrecruit"
    Repo         string                 `json:"repo"`         // e.g. "helixml/find-ai"
    RawMessage   string                 `json:"raw_message"`  // provider verbatim
    Cause        string                 `json:"cause"`        // translated
    NextStep     string                 `json:"next_step"`    // translated action
    FailedAt     time.Time              `json:"failed_at"`
}
```
Clear it (`nil`) on the next successful push.

### A2. Write the error on mirror-push failure
At the rollback site (`git_http_server.go:687-691`), before/after
`rollbackBranchRefs`, populate `PushError` from the acting user's connection
(account handle from `OAuthConnection.ProviderUsername`) and the repo, and
persist it on the task via the store. The acting user is already resolved in
`git_http_server.go:644-668` (`ImplementationApprovedBy` → `SpecApprovedBy` →
`PlanningStartedBy`).

### A3. Don't confabulate success
The agent's completion signal must be tied to specs actually persisting on the
branch the UI reads — not to the pre-mirror 200. Where the task is marked
pushed/ready, gate on the absence of `LastPushError` (and a verified ref). If a
`PushError` was written, the task moves to an explicit error state, not
"ready for review".

### A4. Translate the provider error
Small mapper: raw `remote: Repository not found` for a private repo →
`Cause` + `NextStep` referencing the account, the repo, and the switch-account
action. Keep provider-specific mapping alongside the provider capability entry
(B4) so it is generic.

### A5. Surface on the board/task
The task card/detail shows the error state and the translated cause + next step,
with the action wired to the lozenge switch-account flow. New API field flows
through the existing spec-task serialization (regenerate OpenAPI, use the
generated client per CLAUDE.md).

## Workstream B — The lozenge

### B1. Provider capability abstraction (generic core)
Introduce a small per-provider capability set so the shared component holds
behaviour, not GitHub-shaped code:

```go
type VCSProviderCapability struct {
    Type           types.ExternalRepositoryType
    AuthMechanism  string   // "x-access-token" | "oauth2" | "PAT" | ...
    RequiredScopes []string // GitHub: repo, read:user, user:email, read:org
    // VerifyAccess probes whether a connection can reach a repo.
    VerifyAccess func(ctx, conn *types.OAuthConnection, owner, repo string) (bool, error)
    // IdentityHandle extracts the display handle ("@login").
    IdentityHandle func(conn *types.OAuthConnection) string
    TranslateError func(raw string, account, repo string) (cause, nextStep string)
}
```
Registry keyed by `ExternalRepositoryType`. Adding a provider = adding an entry;
no change to the shared lozenge or push path. Verify probes: GitHub
`GET /repos/{o}/{r}`, GitLab `GET /projects/:id`, ADO/Bitbucket equivalents —
issued via `Provider.MakeAuthorizedRequest` (`oauth/provider.go`).

### B2. Board data API — "acting as" vs "pushing as" + access state
Add an endpoint the board consumes, e.g.
`GET /api/v1/projects/{id}/vcs-connections`, returning one entry per distinct
provider among the project's external repos:

```json
[{
  "provider": "github",
  "state": "verified|needs_attention|disconnected",
  "acting_user": {"id":"...","name":"Tony"},
  "pushing_as": {"username":"@tonychapman-prog","connection_id":"..."},
  "repos": [{"repo":"helixml/find-ai","has_access":true}],
  "missing_scopes": []
}]
```
Presence axis = `distinct(repo.ExternalType) where repo.IsExternal`. State axis =
result of the verify probe (B1) against the acting user's connection.

### B3. Lozenge component (frontend, generic)
New component under `frontend/src/components/tasks/` (or a `vcs/` folder),
rendered in the board chrome (top-right) of `SpecTaskKanbanBoard.tsx`. One
lozenge per provider entry from B2. Uses the generated API client + React Query
(CLAUDE.md). States/labels per the requirements table. Menu items: Switch
account, Reconnect, Disconnect (unbind project), Remove from my account, View on
provider. Reuse existing OAuth connect flow from `OAuthConnections.tsx`.

### B4. Standardize scopes + force account picker
- Store the full per-provider scope set from the capability entry (B1). For
  GitHub: `repo, read:user, user:email, read:org`. Update the requested set at
  `simple_sample_projects.go:731` and any other connect entry points to source
  from the registry; keep `GitHubRepoScopes` validation aligned or widened.
- Force account re-selection: pass the provider's account-picker parameter in
  `GetAuthorizationURL` (GitHub: `prompt=select_account` / re-consent equivalent)
  for Switch/Reconnect so OAuth can't silently re-attach the same session
  account.

### B5. Pre-flight verify
Run the B1 verify probe before kicking off planning (and/or at repo-link time —
open question). On failure, set the lozenge to `needs_attention` and block the
run with the switch-account prompt instead of burning a planning run + ~285s
push that rolls back. **Never** fall back to the repo-level/admin credential for
user-initiated actions (`getCredentialsForRepo` acting-user-first stays; the
repo-level `OAuthConnectionID` remains for system/agent-initiated only).

## Workstream C — Readable dev-startup pull output

The pull output looks half-rendered/hung because `docker pull … 2>&1 | grep -v "^$"`
runs against a non-TTY (plaintext per-layer lines) and **grep block-buffers**
stdout, so lines arrive in bursts and snapshots land mid-line. Not a hang — the
7.67 GB `helix-ubuntu` image is genuinely transferring. Full evidence:
`investigation-helix-in-helix-boot.md` §1.

**Implemented:** added `--line-buffered` to the `grep -v "^$"` pipes in `stack`
(`:1098/:1139/:1176` for `docker pull`, plus `:1089` for `docker push` — same
issue). This flushes each line immediately so the output no longer arrives in
truncated bursts.

**Finding — `sandbox/04-start-dockerd.sh:266/285` left unchanged.** Those
`docker pull … 2>&1` calls have **no** `grep` pipe: docker writes directly and
already line-flushes to a non-TTY. Adding a `grep` pipe there would be actively
harmful — the surrounding `if docker pull …; then` tests the pipeline's exit
status, so piping through `grep` (which returns non-zero when it filters all
lines) would corrupt the boot-critical success check that hard-fails the sandbox.
So the grep-buffering bug only ever existed in the `stack` transfer path.

## Workstream D — Warm desktop image via golden snapshot

The build cache already persists — this session's `helix-ubuntu` build was 100 %
`CACHED`. The cost is the **transfer** (`push → registry:5000 → pull-back`, ~411 s)
because the **sandbox container's own docker store** (`helix_sandbox-docker-storage`,
a named volume, distinct from the golden-cloned desktop inner dockerd) comes up
empty each fresh session. Per `design/2026-02-14-sandbox-docker-storage-split.md`
the two stores are deliberately split and the inner sandbox "pulls from the
registry". Full evidence: `investigation-helix-in-helix-boot.md` §2.

Fix: have the **golden build run through the desktop-image transfer before
promotion**, so the golden snapshot carries `helix-ubuntu:<tag>` inside the
sandbox store. A fresh session then clones a golden where the image is already
present, and the existing skip-checks short-circuit:
- `sandbox/04-start-dockerd.sh:220-224` (skip pull when exact tag present)
- the `./stack` transfer path (`stack:1074-1145`) only re-pushes/pulls when absent

Confirm scope first by reading `api/pkg/services/golden_build_service.go` — how
far the golden build currently runs for this project type, and whether the
transfer can be folded in before `PromoteSessionToGolden(Zvol)` (`api/pkg/hydra/golden.go`).
Persisting `sandbox-docker-storage` independently would fight the split
architecture, so the golden route is the aligned fix.

## Key decisions

- **Two workstreams, A first.** A is independent and removes the footgun even
  before B ships. Do not couple them.
- **Fail loud, never silent fallback** for user-initiated pushes — a fallback
  breaks attribution and is an authorization bypass.
- **Generic by capability registry**, not per-provider branching in the UI or
  push path. GitHub is the first entry; GitLab/ADO/Bitbucket follow.
- **Per-project binding, not per-project credentials** — connections stay
  per-user/global; the project gets a binding + verified-access surface.
- Follow CLAUDE.md: generated API client only, `SimpleTable`/menu patterns,
  `./stack update_openapi` after adding API fields, GORM AutoMigrate,
  structs not maps, no fallbacks/dead code.

## Testing

- **A:** end-to-end in inner Helix — create a spec task against an external repo
  the acting account can't reach; confirm the task shows an error state and the
  translated message, and the agent does NOT report success. Add Go unit tests
  for the `PushError` write + translate mapper.
- **B:** end-to-end — project with a GitHub external repo shows one verified
  lozenge; simulate no-access → `needs_attention`; menu switch forces the account
  picker. Verify no second GitHub repo of a different provider adds a stray
  lozenge; local-only repo adds none.
- Verify pre-flight blocks a planning run when access is missing.
