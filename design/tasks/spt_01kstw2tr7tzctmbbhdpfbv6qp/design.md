# Design: Review PR 2484 Open-Redirect Fix in Logout Endpoint

## Approach

This is a code-review task, not a code-change task. The output is a posted GitHub review with a verdict and evidence. Three review passes feed that verdict:

1. **Claim verification** — does the vulnerability actually exist on `main` as described?
2. **Fix verification** — does the patch close every redirect sink in `logout()`?
3. **Bypass hunt** — can `isSameOriginRedirect()` be tricked by any URL form the contributor didn't think of?

Then two additional considerations shape the final recommendation:

4. **Consistency** — how does this compare to the existing `redirect_uri` validation in `login()`?
5. **Test coverage** — the PR ships no unit tests; is that acceptable here?

## Key Findings From Discovery (already gathered)

These observations are baked in so future agents don't re-discover them:

- **Vulnerable surface (current `main`, auth.go:930–944, 947–981)**: `postLogoutRedirect := r.URL.Query().Get("redirect_uri")` is passed unmodified to `http.Redirect` in two places, written to the `{"url": ...}` JSON body in two more places (one inside the `get_url=true` branch of each auth provider), and URL-escaped into `logoutURL` as `post_logout_redirect_uri` for OIDC RP-Initiated Logout. All five sinks reach attacker-controlled bytes.
- **Endpoint exposure (server.go:1118)**: `insecureRouter.HandleFunc("/auth/logout", ...).Methods("POST")` — no auth required. Combined with the CSRF-exempt list in `auth_middleware.go:439`, a victim can be made to hit this endpoint with a single `<form>` autosubmit or a `fetch(... , {method:'POST'})` from any origin (no preflight needed for a simple POST with no custom headers).
- **Parallel pattern in `login()` (auth.go:499–517)**: same-origin validation is already done there, but uses `r.Host` (request `Host` header) rather than `s.Cfg.WebServer.URL`. The PR diverges from this pattern. Worth a short comment on the PR — either ratify the divergence (logout fallback to a known-safe config value is reasonable when no host is yet established) or ask the contributor to align.
- **Test pattern (auth_test.go)**: `AuthSuite` using `testify/suite` + `gomock`. `testServerURL = "http://localhost:8080"` is the canonical fixture. Any review-recommended unit test for `isSameOriginRedirect` should follow this shape — pure function, table test, no mocks needed.
- **No existing tests for `logout()`** in `auth_test.go` (grep confirmed). Adding one is greenfield, not extending an existing test.

## Bypass-Hunt Strategy

`isSameOriginRedirect(candidate, serverURL)` is 18 lines. Read it once, then attack it as a pure function — no need to spin the server. The contributor's test plan only covered three happy-path inputs; the bypass list in `requirements.md` AC-4 is the minimum. Run each case as a table test mentally (or as a throwaway `go test` if a dev environment is up) against the function as written:

```go
// inside the diff at auth.go:147-164
cu.Scheme == "" && cu.Host == ""   // relative path branch
cu.Scheme == su.Scheme && cu.Host == su.Host   // absolute branch
```

Things to specifically probe:

- **`url.Parse` quirks**: `url.Parse("//evil.com")` returns `Host: "evil.com"`, `Scheme: ""` — the relative-path branch (`Scheme == "" && Host == ""`) correctly rejects this because `Host != ""`. Confirm.
- **Userinfo (`https://evil.com@trusted.com`)**: `url.Parse` puts `evil.com` in `User`, `trusted.com` in `Host` — Go's `http.Redirect` and browsers see different things. Document what Go does, then what the browser does.
- **Trailing-slash / case / port differences** between `candidate` and `serverURL`: `cu.Host` is lowercased by `url.Parse` for `Host`, and includes the port. So `https://Helix.com:443` vs `https://helix.com` — does it match? (`url.Parse` keeps `:443` as part of the host string; this is a real edge case.)
- **Backslash trick `https:/\evil.com`** — some HTTP clients normalize backslashes to slashes. Go's `url.Parse` is strict; document.

## Consistency-vs-`login()` Question

`login()` rejects with `400 Bad Request` on origin mismatch. `logout()` (post-fix) falls back silently to `WebServer.URL`. Different behavior is defensible because:

- `logout()` already cleared the session before reaching the redirect — the user-visible action (logout) succeeded; only the post-action navigation needs to be safe.
- A 400 on logout would leave the user looking at an error page after their session is already gone — worse UX than a fallback redirect.

But it's worth a one-line review comment so the maintainer can ratify or push back.

## Decision Tree for Verdict

- **APPROVE** if: vulnerability reproduces, all sinks closed, bypass-hunt finds no real bypass, missing tests acceptable as a follow-up issue.
- **REQUEST_CHANGES** if: bypass-hunt finds a real bypass, OR a sink is missed, OR tests are deemed mandatory (small PRs with no tests for a security helper are reasonable to push back on).
- **COMMENT** if: bypass-hunt finds a maybe-bypass that needs the contributor's clarification before a verdict.

## Posting the Review

Use `mcp__github__create_pull_request_review` against `helixml/helix#2484` with the chosen `event` and a body that contains:
1. Verdict + one-line rationale.
2. Bypass-hunt table (case → expected → actual).
3. Consistency note re: `login()`.
4. Test-coverage recommendation (file an issue or block on it).
5. CI status snapshot.

## Implementation Notes (added during review)

- **Bypass hunt was run as a standalone Go program** (`/tmp/bypass_hunt.go`) that imports `net/url` and re-implements `isSameOriginRedirect` verbatim from the patch. 24 adversarial inputs exercised. Full table → `findings.md` §3. No security bypass.
- **Two minor robustness notes** surfaced — case-sensitive host comparison (RFC 3986 says hosts are case-insensitive; suggest `strings.EqualFold`) and default-port strictness (`:443` doesn't match implicit `:443`). Both err toward rejection (safe); flagged as non-blocking comments.
- **CI status**: `get_pull_request_status` returned `state: pending, total_count: 0`. Helix Drone CI likely gates external-contributor PRs from secret-bearing builds. Mention in the review.
- **Verdict: APPROVE** — vulnerability real, fix correct, all sinks closed, no bypass. Missing tests recommended as follow-up but not a blocker for a 30-line security fix that the maintainer can validate by reading.
