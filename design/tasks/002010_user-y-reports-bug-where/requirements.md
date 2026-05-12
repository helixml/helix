# Requirements: Diagnose & Fix Agent Git/OAuth Push Failure

## Problem Statement

User Y reports that an agent attached to a spec task **cannot talk to git** when the
project's external repository (GitHub) is configured via **OAuth**. The agent appears
to operate normally inside the sandbox, but commits never reach the upstream GitHub
repo (or do not appear, or the user perceives the integration as broken).

The log snippet the user attached is **not** evidence of the failure — it is Zed
startup noise (`edit_prediction: no credentials provided` is Zed Pro completion
auth, websocket warnings are the IDE↔Helix sync channel). It contains zero git or
push lines. So the first job is to **collect the right evidence**, then fix the
root cause.

## Background — How the Push Actually Works

The agent does **not** push directly to GitHub. It pushes to Helix's internal git
proxy (`http://api:8080/git/{repo-id}.git`), and Helix then pushes from its bare
mirror to GitHub using a credential resolved server-side. Credential resolution
lives in `api/pkg/services/git_repository_service.go::getCredentialsForRepo`, called
from `PushBranchToRemote` (`git_repository_service_push.go`). For agent-initiated
pushes the acting user is found by walking the spec-task phase chain in
`git_http_server.go:629-642`:

```
ImplementationApprovedBy → SpecApprovedBy → PlanningStartedBy
```

The first non-empty user wins, and **their** OAuth connection is used. If none have
a GitHub OAuth connection, the code falls through to repo-level `OAuthConnectionID`
or a PAT (`GitHub.PersonalAccessToken`). If none of those exist either, the push
URL has no credentials and GitHub rejects it.

Critically, `git_http_server.go:610-666` already documents that **if the upstream
push fails, the agent will not know** — receive-pack has already returned 200 by
then. Errors are logged on the server only. (Spec task `001434_propagate-upstream-git`
proposed surfacing these errors via `LastPushError` / `LastUpstreamPushError`, but
**`grep` shows neither field exists in the codebase** — that work was not landed.)

## User Stories

### US1 — Reproduce and identify root cause
**As** a Helix engineer triaging this bug,
**I want** a deterministic way to reproduce User Y's failure on the inner-Helix dev
environment,
**So that** I can identify *which* of the failure modes in §"Candidate Root Causes"
(design.md) is actually firing for them, instead of guessing.

### US2 — Make the failure visible
**As** an agent / a user watching the agent,
**I want** OAuth-related upstream push failures to appear *somewhere I can see them*
(API response, task UI, server log searchable by repo id, or all three),
**So that** the user does not silently lose work and the operator can diagnose
without raw log diving.

### US3 — Fix the actual failure
**As** User Y,
**I want** my agent to push to my OAuth-configured GitHub repo successfully, end to
end, without me touching credentials manually,
**So that** the integration the UI advertised actually works.

## Acceptance Criteria

### AC1 — Reproduction documented
- [ ] A short repro (one-paragraph script) is checked in to this task folder under
  `repro.md` (or appended to `design.md`) that triggers the same failure on the
  inner Helix.
- [ ] The repro identifies which failure mode applies: missing OAuth connection on
  the phase-chain user, expired/revoked token, missing scope, wrong external type,
  or other.

### AC2 — Visibility for upstream push failures
- [ ] When `PushBranchToRemote` fails for a repo, the failure is recorded on
  `GitRepository` (or equivalent) and surfaced via the existing
  `GET /api/v1/git/repositories/{id}` endpoint **at minimum as a single string +
  timestamp**. (Reuse the field names from spec `001434` — `last_push_error`,
  `last_push_error_at` — if reviving that scope here, otherwise keep this minimal.)
- [ ] When the failure is OAuth/auth-related (HTTP 401/403, "Invalid username or
  password", "Bad credentials"), the message shown to the user is actionable and
  names the **likely** cause: "no GitHub OAuth token found for any of the users on
  this task", "GitHub returned 403 — token may have lost `repo` scope", etc.
- [ ] Successful push clears the error.

### AC3 — Fix the root cause
- [ ] Whatever root cause AC1 identified is fixed (not papered over).
- [ ] Manual end-to-end test: a user logs in to inner Helix, connects GitHub via
  OAuth, creates a project pointing at a real GitHub repo of theirs, runs a spec
  task to completion. The agent's commits arrive on GitHub on the feature branch
  with no manual intervention.
- [ ] Existing PAT-based and Helix-internal repos are unaffected (regression
  guard).

### AC4 — Don't regress the silent-success path
- [ ] The architectural note in `git_http_server.go:610-616` (receive-pack returns
  200 before upstream push happens) is preserved or explicitly addressed. We do
  not want to *break* the agent's `git push` exit code while we're in here.

## Out of Scope

- Full real-time error propagation to the agent's stdout (would require git wire
  protocol changes — same architectural boundary `001434` already noted).
- OAuth token **refresh** logic — if tokens expire, surface clearly and prompt the
  user to re-connect. Auto-refresh is a separate task.
- Frontend UI work beyond what naturally falls out of surfacing the new fields on
  the existing endpoints.
- GitLab / Azure DevOps / Bitbucket — verify they still work (regression), but
  scope of this bug is GitHub OAuth specifically.

## Open Questions

The user has not yet provided:
- The actual repo id, project id, or task id involved.
- The user account they connected GitHub OAuth on (vs. the user who started the
  task — could be different people, which is itself a candidate root cause).
- API logs filtered to `repo_id=...` and `[GitPush]` — these are the lines that
  would actually show the failure.

The first action under this task is to ask the user for those three things in
parallel with running the dev-environment repro.
