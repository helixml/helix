# Project VCS connection lozenge & loud push-failure surfacing

## Summary

Spec-task pushes to external repos could fail **silently**: the internal git
server returns 200 to the client before mirroring to the provider, and when the
mirror push fails the refs are rolled back with no signal — so the agent reported
"pushed and ready for review" while the UI showed nothing. This PR makes those
failures loud and adds a per-provider connection lozenge on the project board so
users can see which account they're pushing as and whether it can actually reach
the repo. Also bundles two Helix-in-Helix dev-startup fixes found during the work.

## Changes

**Loud push-failure surfacing (backend)**
- New `types.PushError` + `SpecTask.LastPushError` (jsonb); persisted on external
  push failure and cleared on the next success (`git_http_server.go`
  `recordPushError`/`clearPushError`).
- `types.NewPushError` translates raw provider errors (the misleading GitHub 404
  for private repos, 403, 401) into a human cause + next step. Unit-tested.
- Helpers in `git_repository_service.go`: `OAuthProviderTypeForRepo`,
  `RepoOwnerName`, `GetActingAccountHandle`.

**VCS connection lozenge (generic across providers)**
- New `api/pkg/vcs` capability registry (per-provider auth mechanism, required
  scopes, access-probe path). GitHub fully specified; GitLab/ADO/Bitbucket stubbed.
- New endpoint `GET /api/v1/projects/{id}/vcs-connections` — one entry per distinct
  provider among the project's external repos, with acting-user, pushing-as,
  per-repo verified access (via `oauth.Provider.MakeAuthorizedRequest`), and
  missing scopes.
- Frontend `VCSConnectionLozenges` rendered in the board header (verified /
  needs-attention / disconnected states; acting-as vs pushing-as + missing scopes
  in the tooltip; menu links to Connected Services + View on provider).

**Scopes**
- GitHub connect now requests the full `repo, read:user, user:email, read:org`
  set from the registry (was `repo, read:user, user:email`). Hard validation kept
  at `repo` so existing connections aren't rejected.

**Dev-environment fixes**
- `stack`: `docker push/pull` output piped through `grep --line-buffered` so the
  desktop-image transfer no longer renders as truncated/hung bursts.

## Testing
- `go build ./pkg/...` green; `types` unit test for the error mapper passes.
- Frontend `tsc --noEmit` clean.
- E2E (inner Helix): board loads, calls the new endpoint → `200 []` for a
  non-external repo (no lozenge), zero console errors.
- NOT tested: verified/needs-attention lozenge states (needs a real connected
  GitHub account + external repo).

## Screenshots
![Board loads clean](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002205_implement-design2026-07/screenshots/05-board-loads-clean.png)

## Follow-ups (not in this PR)
- Force the provider account picker on switch/reconnect.
- Per-project "disconnect" (unbind without revoking the global token).
- Pre-flight verify before "Start planning".
