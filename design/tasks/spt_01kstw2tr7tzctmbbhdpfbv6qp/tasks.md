# Implementation Tasks: Review PR 2484 Open-Redirect Fix in Logout Endpoint

- [~] Read the PR diff at https://github.com/helixml/helix/pull/2484 end-to-end (one file, +30/-2)
- [~] Read current `main` `auth.go:910-982` (the `logout()` function) and confirm the five redirect sinks: two `http.Redirect`, two `get_url=true` JSON responses, one OIDC `post_logout_redirect_uri` substring in `logoutURL`
- [~] Verify the endpoint exposure: `server.go:1118` registers `/auth/logout` on `insecureRouter`; `auth_middleware.go:439` lists it in `csrfExemptPaths`
- [ ] Reproduce the open redirect against current `main` by hitting `POST /api/v1/auth/logout?redirect_uri=https://evil.example` (use inner Helix at `http://localhost:8080` if dev stack is up; otherwise note that reproduction was code-only)
- [ ] Confirm each of the five sinks is gated by the new `isSameOriginRedirect()` check — i.e. `postLogoutRedirect` is reset to `s.Cfg.WebServer.URL` before reaching every sink
- [ ] Bypass-hunt: table-test `isSameOriginRedirect()` against the inputs in `requirements.md` AC-4 (empty, relative, same-origin, cross-origin, protocol-relative `//evil.com`, userinfo `https://evil@trusted`, `javascript:`, suffix `<host>.evil.com`, scheme mismatch, port mismatch). Record case → expected → actual.
- [ ] Compare to the existing `redirect_uri` validation in `login()` (auth.go:499–517). Note that login uses `r.Host` (request) while logout uses `s.Cfg.WebServer.URL` (config). Decide whether to flag this as a follow-up.
- [ ] Note the missing unit test for `isSameOriginRedirect`. Decide whether to block the PR on tests or accept as a follow-up (file an issue if accepting).
- [ ] Check CI status on the PR (`mcp__github__get_pull_request_status` or Drone MCP if applicable)
- [ ] Compose review body with: verdict, bypass-hunt table, consistency note, test-coverage recommendation, CI snapshot
- [ ] Post the review via `mcp__github__create_pull_request_review` with the chosen `event` (APPROVE / REQUEST_CHANGES / COMMENT)
