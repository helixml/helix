# Review Findings: PR 2484

## Verdict: APPROVE (with test-coverage follow-up)

The vulnerability is real, the fix closes every sink, and the bypass hunt found no security issue. Two minor robustness notes worth surfacing on the PR; one test-coverage gap worth flagging.

## 1. Vulnerability Confirmed on `main`

- **Endpoint exposure** — `POST /api/v1/auth/logout` is registered on `insecureRouter` (`server.go:1118`) and is in `csrfExemptPaths` (`auth_middleware.go:441`). No authentication, no CSRF token required, simple POST body → cross-origin exploitable from any attacker page.
- **Five reachable redirect sinks** (current `main`, `auth.go:932-981`):
  1. Regular-auth `get_url=true` JSON response (`auth.go:939-941`)
  2. Regular-auth `http.Redirect` (`auth.go:943`)
  3. OIDC fallback `get_url=true` JSON response (`auth.go:953-955`)
  4. OIDC fallback `http.Redirect` (`auth.go:957`)
  5. OIDC main-path `post_logout_redirect_uri` URL-escaped into `logoutURL` (`auth.go:964-969`) — IdP then 302s the user with this value

All five reach attacker-controlled bytes via the `redirect_uri` query parameter.

## 2. Fix Closes All Five Sinks

The patch sets `postLogoutRedirect = s.Cfg.WebServer.URL` whenever `isSameOriginRedirect()` returns false, **before** any of the five sinks reads the variable. Verified by tracing the variable from line 956 (post-validation block) through to each sink. ✓

## 3. Bypass Hunt — No Security Bypass Found

Ran `isSameOriginRedirect("...", "https://helix.example.com")` against 24 adversarial inputs:

| Case | Candidate | Expected | Got | Verdict |
|---|---|---|---|---|
| empty | `""` | accept | accept | OK |
| relative path | `/dashboard` | accept | accept | OK |
| same-origin absolute | `https://helix.example.com/welcome` | accept | accept | OK |
| cross-origin absolute | `https://evil.com` | reject | reject | OK |
| protocol-relative | `//evil.com` | reject | reject | OK (Host parses to `evil.com`, not the relative-path branch) |
| userinfo trick (host in userinfo) | `https://evil.com@helix.example.com` | accept | accept | OK — `url.Parse` puts `evil.com` in `User`, real Host is `helix.example.com`; safe because the browser also resolves it to `helix.example.com` |
| reverse userinfo | `https://helix.example.com@evil.com` | reject | reject | OK |
| `javascript:` URI | `javascript:alert(1)` | reject | reject | OK (Scheme=`javascript`, no match) |
| `data:` URI | `data:text/html,…` | reject | reject | OK |
| suffix trick | `https://helix.example.com.evil.com` | reject | reject | OK |
| prefix trick | `https://evil.helix.example.com` | reject | reject | OK |
| scheme mismatch http→https | `http://helix.example.com` | reject | reject | OK |
| port mismatch | `https://helix.example.com:8443` | reject | reject | OK |
| backslash trick (with scheme) | `https:/\evil.com` | reject | reject | OK |
| backslash relative path | `/\evil.com` | accept | accept | OK — relative; `http.Redirect` will normalize |
| triple slash | `https:///evil.com` | reject | reject | OK |
| CRLF / newline injection | `/dashboard\nhttps://evil.com` | reject (parser errors) | reject | OK |
| unicode lookalike | `https://helıx.example.com` (Turkish dotless i) | reject | reject | OK |
| trailing slash | `https://helix.example.com/` | accept | accept | OK |
| with fragment | `https://helix.example.com/#evil` | accept | accept | OK |

## 4. Robustness Notes (Non-Blocking)

These are not security issues — they err on the safe side. Worth a comment on the PR for the contributor's awareness.

### 4a. Case-Sensitive Host Comparison
`url.Parse` does NOT lowercase `Host`. So `https://HELIX.example.com` does not match `https://helix.example.com` per the current function — it falls back silently. RFC 3986 §3.2.2 says hosts are case-insensitive.

This errs toward rejection (safe), but could surprise an operator whose `WebServer.URL` is `https://Helix.acme.com` and whose frontend submits `window.location.origin` lowercased by Chrome.

**Suggestion:** `strings.EqualFold(cu.Host, su.Host)` instead of `==`.

### 4b. Default-Port Strictness
`https://helix.example.com:443` does NOT match `https://helix.example.com` — `url.Parse` keeps `:443` in `Host`. Operators whose `WebServer.URL` includes an explicit `:443`/`:80` (or whose frontend submits one) will see redirects silently fall back.

Acceptable trade-off; flag in the comment.

### 4c. Empty-`serverURL` Edge Case
If `s.Cfg.WebServer.URL` is empty/misconfigured, the function returns false for every non-empty candidate, including the empty string (which is treated specially and returns true). The handler then sets `postLogoutRedirect = s.Cfg.WebServer.URL` (also empty), and `http.Redirect(w, r, "", 302)` produces `Location: /` — harmless, no open-redirect. ✓

## 5. Consistency vs `login()` (auth.go:499-517)

`login()` validates `redirect_uri` against `r.Host` (the request `Host` header) and returns **400 Bad Request** on mismatch. The PR validates against `s.Cfg.WebServer.URL` and falls back silently.

Defensible difference:
- `logout()` has already cleared the session by the time it reaches the redirect — failing hard with a 400 would leave the user on an error page after a successful logout (bad UX). Silent fallback is the right call.
- `login()` has not yet created a session, so failing hard prevents an attacker from completing a tainted login flow.
- `r.Host` is mutable by upstream proxies; `WebServer.URL` is operator-configured. For the logout sink, the operator-configured value is the safer anchor.

Worth a one-line note on the PR ratifying the divergence; no change required.

## 6. Missing Unit Test

The PR ships zero test for `isSameOriginRedirect`. The function is pure (no I/O, no mocks), trivially table-testable. `auth_test.go` already exists with a `testify/suite` + `gomock` pattern (and a `testServerURL = "http://localhost:8080"` constant).

**Recommendation:** request a follow-up PR (or accept this PR and file a `test:` task) that adds a table test covering at least the cases in §3. Not a merge blocker — the implementation is correct — but the function will rot without tests if a future refactor changes behavior.

## 7. CI Status

`get_pull_request_status` returned `state: pending, total_count: 0` for SHA `d8667e1`. No checks have run. Helix CI is on Drone, which typically gates secret-bearing builds on external-contributor PRs. The maintainer should kick a build manually before merging.

## 8. Out of Scope (Flagged for Awareness)

The same `redirect_uri` query-param pattern exists on the `login()` flow (auth.go:499-517) and is already validated there. No other obvious `http.Redirect(..., user-controlled, ...)` sites were spotted during this review, but a sweep was not exhaustive — out of scope for this PR.
