# Implementation Tasks: Review PR 2484 Open-Redirect Fix in Logout Endpoint

- [x] Read the PR diff at https://github.com/helixml/helix/pull/2484 end-to-end (one file, +30/-2)
- [x] Read current `main` `auth.go:910-982` (the `logout()` function) and confirm the five redirect sinks: two `http.Redirect`, two `get_url=true` JSON responses, one OIDC `post_logout_redirect_uri` substring in `logoutURL`
- [x] Verify the endpoint exposure: `server.go:1118` registers `/auth/logout` on `insecureRouter`; `auth_middleware.go:441` lists it in `csrfExemptPaths`
- [x] Reproduce the open redirect against current `main` — code-only reproduction (dev stack not exercised); the unmodified handler passes `r.URL.Query().Get("redirect_uri")` straight to `http.Redirect`
- [x] Confirm each of the five sinks is gated by the new `isSameOriginRedirect()` check — `postLogoutRedirect` is reset to `s.Cfg.WebServer.URL` before reaching every sink (verified line-by-line)
- [x] Bypass-hunt: table-test `isSameOriginRedirect()` against 24 adversarial inputs (`/tmp/bypass_hunt.go`). Full table in `findings.md` §3. **No security bypass found.**
- [x] Compare to existing `redirect_uri` validation in `login()` — divergence is defensible (login hard-fails 400, logout falls back silently because session is already cleared). Flagged in review as a one-line note.
- [x] Note the missing unit test for `isSameOriginRedirect`. **Accept as follow-up** — 30-line security fix with correct logic; tests recommended but not a merge blocker.
- [x] Check CI status — `state: pending, total_count: 0`. No checks ran (likely Drone gates external PRs from secrets). Mentioned in review.
- [x] Compose review body — see `findings.md` and the posted PR review.
- [x] Post the review via `mcp__github__create_pull_request_review` — **APPROVED** at https://github.com/helixml/helix/pull/2484#pullrequestreview-4399392694
- [x] Write PR description file (review-only task, no code change; `pull_request.md` documents the verdict + link to the posted review)
