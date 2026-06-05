# Design: Diagnose & Fix Agent → GitHub OAuth Push Failure

## Architecture Recap

```
Agent (Zed/Qwen, in sandbox)
   │  git push http://api:8080/git/<repo-id>.git refs/heads/<branch>
   ▼
Helix git proxy (api/pkg/services/git_http_server.go)
   │  receive-pack writes to local bare repo
   │  (returns 200 OK to agent here ──────────────────────────► agent thinks success)
   ▼
PushBranchToRemote (git_repository_service_push.go)
   │  getCredentialsForRepo(...) → username, token
   │  buildAuthenticatedCloneURLForRepo(...) → https://x-access-token:TOKEN@github.com/...
   ▼
GitHub
```

The agent never sees an upstream push failure today. That is the architectural
constraint. Everything the user calls "agent can't talk to git via OAuth" actually
manifests as **a server-side failure inside the second-from-last box** — silent.

## Candidate Root Causes (ordered by prior probability)

Until we have the actual logs from User Y's instance, all of these are plausible.
The repro plan below disambiguates them.

1. **No phase-chain user has a GitHub OAuth connection.** `getCredentialsForRepo`
   only walks `OAuthConnectionID` if no acting user matches; if the acting user
   exists but has no GitHub OAuth row, the loop just doesn't hit `return`, and
   the function falls through to the repo-level path. If repo-level OAuth is also
   absent and there's no PAT, the URL has no creds → GitHub responds `401`.
   - Likely if the *project owner* connected GitHub OAuth but the *task starter*
     (a different user, e.g. an admin) did not.

2. **Repo isn't typed as `ExternalRepositoryTypeGitHub`.** The acting-user OAuth
   path is gated on `gitRepo.ExternalType == ExternalRepositoryTypeGitHub`
   (`git_repository_service.go:2653`). If the repo was created without the type
   being set (e.g. via an API path that infers from URL but missed `github.com`,
   or a custom/self-hosted GH Enterprise URL), this branch is skipped. Repo-level
   OAuth path *is* still hit, but only if `OAuthConnectionID` is set on the repo
   row — which is a different code path that may not be wired by the
   "Connect GitHub via OAuth" UI flow.

3. **Phase-chain only walks 3 fields, all empty.** For a brand-new task that the
   agent picks up before any human approves anything (e.g. an "auto-start" path),
   `ImplementationApprovedBy`, `SpecApprovedBy`, and `PlanningStartedBy` could all
   be empty — `pushUserID` ends up `""` and `getCredentialsForRepo` falls through
   to repo/PAT, which may also be empty.

4. **Token expired / scope-stripped.** GitHub OAuth tokens can be revoked or have
   their scopes downgraded out of band. The token sits in `OAuthConnection.AccessToken`
   verbatim with no refresh logic in this code path. GitHub returns `403` "Resource
   not accessible by personal access token" or "Bad credentials".

5. **Wrong username for OAuth flow.** GitHub accepts the OAuth token only with
   `x-access-token` (App tokens) or `oauth2` (some flows). Code uses
   `x-access-token` for both `OAuthProviderTypeGitHub` user-OAuth and the repo-level
   GitHub OAuth — that's correct for GitHub Apps and standard OAuth, but if the
   token is actually a fine-grained PAT stored in the OAuth row by mistake,
   `x-access-token` may still work, but unusual configurations could fail.

6. **Token leak in logs.** Tangential, but worth checking while we're here:
   `pushBranchNative` calls `giteagit.Push` with the token *embedded in the URL*.
   If the underlying gitcmd surfaces the URL on error, we may be writing tokens to
   stderr / log files. (Not the cause, but a finding worth fixing if true.)

## Why the Provided Logs Don't Help

The user's log snippet covers Zed application startup (`Rendered first frame`,
`opening git repository at ...`, `agent_servers::connection_cache`) and edit
prediction errors (`edit_prediction.rs:2216 no credentials provided` — that's
Zed Pro / Anthropic completion, **not** git). There are zero lines from
`api/pkg/services/git_http_server.go` (`Receive-pack completed`,
`Pushing branch to upstream`, `[GitPush] FAILED`, etc.). The first concrete
debugging step is to ask the user for the right log slice (see Plan §1).

## Plan

### Phase 1 — Gather evidence (parallel with phase 2)

Ask User Y for, in this order of usefulness:
1. The spec task id (we can pivot from there to project, repo, task creators).
2. API server logs for the time window when they tried, filtered to:
   - `[GitPush]` — covers `PushBranchToRemote` instrumentation
   - `Receive-pack completed` and surrounding lines
   - `Pushing branch to upstream` / `Failed to push branch to upstream`
3. Whether GitHub OAuth was connected by them or by a different account.

If the user can't pull logs, we point them at:
```
docker compose -f docker-compose.dev.yaml logs api 2>&1 | \
  grep -E "\[GitPush\]|Receive-pack|upstream" | tail -200
```

### Phase 2 — Reproduce locally

In the inner Helix (`http://localhost:8080`), reproduce the OAuth path the user
followed:
1. Register, complete onboarding (per `helix/CLAUDE.md`).
2. Connect a real GitHub OAuth integration via Settings.
3. Create a project pointing at a small test GitHub repo we own.
4. Start a spec task; let the agent push.
5. Watch `docker compose -f docker-compose.dev.yaml logs -f api | grep -E "GitPush|Receive-pack"`.

If it works locally, the candidate root causes narrow to user-specific (#1, #3,
#4). If it fails locally, we have a deterministic repro for #2 / #5 / wiring
bugs.

### Phase 3 — Implement visibility (the AC2 work)

Even before the root cause is fixed, **make the failure observable**. Smallest
viable change:

- Add `LastPushError string` and `LastPushErrorAt *time.Time` fields to
  `types.GitRepository`.
- In `git_http_server.go::handleReceivePack`, after the `PushBranchToRemote` for
  loop completes, write the captured error (or clear on success) via
  `gitRepoService.UpdatePushStatus(repoID, err)`.
- Surface the fields in the existing GET repository handler's response (no new
  endpoint — they're on the row already).
- Classify auth errors: if the error string contains `401`, `403`,
  `Bad credentials`, `Invalid username or password`, or
  `Resource not accessible`, prepend a short hint to the stored message:
  `"OAuth: <hint>: <original error>"`. Hints:
  - `403` + `permission` → "token lacks `repo` scope or no access to this repo"
  - `401` / `Bad credentials` → "token rejected — likely expired or revoked"
  - empty creds (detected at `getCredentialsForRepo` returning `"", ""`) →
    "no OAuth/PAT/user-credential found — connect GitHub OAuth on the user
    who started this task"

This piece is the unfinished body of spec `001434_propagate-upstream-git`. We
intentionally keep the scope tiny: one repo column, one error string, one
classifier function. No frontend redesign, no per-task field, no real-time
push to the agent.

### Phase 4 — Fix the root cause

Driven by what phases 1–3 reveal. Likely shape:

- **If root cause is #1 or #3 (no OAuth on phase-chain user):** broaden the
  fallback. After the phase-chain walk, if no acting user is found *or* the
  found user has no GitHub OAuth, also try the **project owner** and the
  **organization owner** before falling through to repo PAT. This matches
  user mental model — "I connected GitHub on my org account, why does the
  agent not use it".
- **If root cause is #2 (wrong external type):** the bug is in the
  project-creation handler, not the push code. Make sure
  `ExternalRepositoryType` is correctly inferred / set when a GitHub URL is
  provided, and that "Connect via OAuth" UI flow writes
  `OAuthConnectionID` on the repo row (not just on the user).
- **If root cause is #4 (expired token):** scope this task to surfacing it
  clearly and instructing the user to reconnect. Implementing token refresh is
  a separate task.

### Phase 5 — End-to-end verification

Per `helix/CLAUDE.md` "Never give up on testing": run the full repro from
phase 2 again after the fix, on the inner Helix, with the agent actually
pushing. Confirm:
- Commits land on the GitHub feature branch.
- `GET /api/v1/git/repositories/{id}` returns `last_push_error: ""` (or the
  field is unset).
- Force a known-failing case (revoke the token mid-flow) and confirm
  `last_push_error` populates with an actionable message.

## Files Likely Touched

| File | Change |
|---|---|
| `api/pkg/types/git_repository.go` | Add `LastPushError`, `LastPushErrorAt` fields + GORM tags |
| `api/pkg/services/git_repository_service.go` | Add `UpdatePushStatus(repoID, err)`; classify auth errors |
| `api/pkg/services/git_http_server.go` | Call `UpdatePushStatus` after each upstream push attempt; potentially extend phase-chain fallback |
| `api/pkg/services/git_repository_service_push.go` | Return classified errors so caller can store them |
| `api/pkg/server/git_handlers.go` (whichever serves `GET /api/v1/git/repositories/{id}`) | Include the new fields (auto if struct serialised) |
| (Optionally) frontend repo card / project settings | Surface the error if present — out of scope unless trivial |

## Risks & Non-Risks

- **Risk:** Storing the raw error message could include the token if the gitcmd
  layer leaks the URL. Mitigate by stripping `://[^@]+@` from any error string
  before persisting (`stripCredentialsFromURL` already exists in the same package).
- **Risk:** Adding fields to `GitRepository` requires a GORM migration. AutoMigrate
  handles new columns, no data migration needed.
- **Non-risk:** The agent's git push exit code is unchanged — receive-pack still
  returns 200 before upstream is touched. We are purely adding observability +
  fixing the upstream auth path.

## Notes for Future Agents

- The pattern "check `git_http_server.go:610-666` for the receive-pack →
  upstream-push handoff" comes up repeatedly across spec tasks 001062,
  001434, and now this one. That handoff is the architectural seam where most
  agent-perceives-success-but-nothing-happened bugs live. If you see the
  symptom, start there.
- `helix/CLAUDE.md` mandates real end-to-end testing in the inner Helix for any
  UI-touching change. This bug is mostly server-side, but the verification step
  (phase 5) still needs a real OAuth connect, project create, and agent run —
  no shortcut via `curl` / unit test alone.
- `getCredentialsForRepo` is the central choke point for *all* outbound git
  auth — touch it carefully; force-push protection (001062), upstream error
  reporting (001434), and this bug all flow through it.
