# Requirements: Review PR 2484 Open-Redirect Fix in Logout Endpoint

## Context

External contributor `@Joshua-Medvinsky` submitted https://github.com/helixml/helix/pull/2484, a security fix that adds same-origin validation to the `redirect_uri` query parameter on `POST /api/v1/auth/logout`. Without validation, the endpoint (on `insecureRouter`, in `csrfExemptPaths`) was usable as a CWE-601 open-redirect — an attacker-supplied URL would 302 the victim from the trusted Helix origin to a phishing site.

The PR changes one file (`api/pkg/server/auth.go`, +30/-2), introducing `isSameOriginRedirect()` and gating the four redirect sinks in `logout()` (two `http.Redirect` + two `get_url=true` JSON paths, plus the OIDC `post_logout_redirect_uri` value).

## Goal

Produce a review verdict on this PR: approve, request-changes, or close. The verdict must be backed by:

- Confirmation the security claim is real (the vulnerability exists on `main` as described).
- Confirmation the fix actually closes every redirect sink in `logout()`.
- A bypass-hunt against `isSameOriginRedirect()` covering edge cases the contributor's test plan didn't enumerate.
- A consistency check against the parallel `redirect_uri` validation already present in `login()` (auth.go:499–517) — same approach? Different? Why does that matter?
- A judgement on whether the missing unit tests block merge or are acceptable as a follow-up.

## User Stories

**US-1 (Maintainer).** As a Helix maintainer I want a documented review of PR 2484 so I can decide whether to merge, request changes, or close, without re-doing the contributor's threat model myself.

**US-2 (Security reviewer).** As a security reviewer I want the bypass-hunt evidence (what URL forms were tested against `isSameOriginRedirect`) recorded so a future incident can re-run the same cases without rediscovering them.

**US-3 (Helix operator).** As an operator who runs Helix behind a reverse proxy with multiple hostnames I want to know whether this fix breaks my existing logout flow (it pins the allowed origin to `s.Cfg.WebServer.URL`, not the request `Host` header).

## Acceptance Criteria

- **AC-1** A review comment is posted on the PR with the verdict (APPROVE / REQUEST_CHANGES / COMMENT) and the supporting evidence inline.
- **AC-2** The vulnerability is reproduced against `main` (or an explicit note explains why reproduction wasn't possible, e.g. dev stack not bootable).
- **AC-3** All four redirect sinks in `logout()` are enumerated and each is verified to be gated by `postLogoutRedirect` being reset to the safe fallback when validation fails. Specifically: regular-auth `Redirect`, regular-auth `get_url=true` JSON, OIDC fallback `Redirect`, OIDC fallback `get_url=true` JSON, and the OIDC `post_logout_redirect_uri` value embedded in `logoutURL`.
- **AC-4** `isSameOriginRedirect()` is challenged with at least these inputs and the result documented:
  - `""` (empty) — must accept
  - `"/dashboard"` (relative) — must accept
  - `"https://<webserver-host>/x"` (same origin) — must accept
  - `"https://evil.com"` (cross-origin absolute) — must reject
  - `"//evil.com"` (protocol-relative) — must reject
  - `"https://evil.com@<webserver-host>"` (userinfo trick) — document behavior
  - `"javascript:alert(1)"` (non-http scheme) — must reject
  - `"https://<webserver-host>.evil.com"` (suffix trick) — must reject
  - `"http://<webserver-host>"` when `WebServer.URL` is `https://<webserver-host>` (scheme mismatch) — must reject
  - URL with a different port than `WebServer.URL` — document behavior
- **AC-5** The consistency gap with `login()`'s validation (which uses `r.Host` instead of `s.Cfg.WebServer.URL`) is called out: either the PR is fine because the two endpoints have different threat models, or a follow-up is recommended.
- **AC-6** The test-coverage gap is acknowledged: the PR adds no unit test for `isSameOriginRedirect`. The review decides whether this blocks merge.
- **AC-7** CI status on the PR is reported (pass/fail/none).

## Out of Scope

- Implementing the fix differently (the PR's approach is acceptable if it passes the bypass-hunt).
- Adding the `Auth.AllowedPostLogoutRedirects` config knob the contributor mentions as a follow-up — that's a separate ticket if operators ask for it.
- Auditing other `http.Redirect` call sites in the codebase for similar open-redirect bugs — out of scope for this PR review, but flag if obvious cousins are spotted.
