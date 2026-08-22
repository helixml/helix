# 2026-08-18 — Claude subscription: real account identity + probe model fix

## Problem 1: caption showed the Helix user, not the Claude account

Agent settings rendered the subscription caption from the *Helix* owner of the
`claude_subscriptions` row (e.g. `test@helix.local · Max`). The operator expects
the **Claude account the token authenticates as** — "phil@winder.ai max 20x plan".

Ground truth (extracted from the Claude Code CLI v2.1.234 bundle):

- `GET https://api.anthropic.com/api/oauth/profile` with
  `Authorization: Bearer <token>` (no anthropic-beta header) returns
  `{account: {uuid, email, display_name, created_at},
    organization: {uuid, organization_type, rate_limit_tier, seat_tier, ...}}`.
- Works with both OAuth access tokens and `sk-ant-oat` setup tokens (the CLI
  calls it with the same token it probes with; `eie()` does exactly this for
  `CLAUDE_CODE_OAUTH_TOKEN`).
- `organization_type` maps to the plan: `claude_max→max`, `claude_pro→pro`,
  `claude_enterprise→enterprise`, `claude_team→team`. The CLI persists
  `account.email` in its config as `oauthAccount.emailAddress`.

### Confirmed live, 2026-08-19

The endpoint was previously inferred from the CLI bundle. It has now been called
directly with a real Claude Code OAuth access token (`sk-ant-oat01…`, scopes
include `user:profile`) and returns **HTTP 200**:

```json
{
  "account":      { "email": "<redacted>", "display_name": "…",
                    "has_claude_max": true, "has_claude_pro": false },
  "organization": { "name": "…'s Organization", "organization_type": "claude_max",
                    "rate_limit_tier": "default_claude_max_20x",
                    "subscription_status": "active" },
  "application":  { "name": "Claude Code", "slug": "claude-code" }
}
```

Every field name `claudeProfileResponse` parses matches verbatim. So the owner
of an OAuth-connected Claude subscription **is** inspectable — the claim that it
is not holds only for setup tokens (the scope wall in Problem 1b), not for the
oauth credential flow.

Two corroborations on any machine with the CLI logged in:

- `~/.claude/.credentials.json` already carries `subscriptionType` and
  `rateLimitTier` alongside the tokens.
- `~/.claude.json` → `oauthAccount` is the CLI's cache of this exact call
  (`emailAddress`, `organizationType`, `organizationRateLimitTier`,
  `profileFetchedAt`).

Note the tier is an internal slug — `default_claude_max_20x`, not `20x`.
`formatRateLimitTier()` reduces it to the multiplier for display and drops
slugs that carry none (`default_claude_pro` → nothing), so the UI never leaks
Anthropic jargon.

Helix never called this endpoint, so no email/plan/tier ever existed on the row
for setup-token connections (credentials file flow did carry `subscriptionType`
/`rateLimitTier`, but a setup token carries none of that).

### Fix

- `api/pkg/anthropic/subscription_profile.go` — `FetchClaudeProfile(ctx, token)`.
- `ClaudeSubscription.AccountEmail` / `AccountDisplayName` columns (GORM
  AutoMigrate) — the billed identity, distinct from `OwnerID` (who connected it).
- `revalidateClaudeSubscription` fetches the profile on every **valid** probe
  and enriches the row: account email/name, live plan, live rate-limit tier
  (this also backfills plan/tier on setup-token rows that had none). Profile
  failure is best-effort — it never downgrades a just-validated subscription.
- `ValidateSubscription` now also returns the bearer token it probed, so the
  profile fetch reuses the same credential (no double decrypt, no fallback).
- Status endpoint exposes `claude_account_email`, `claude_account_name`,
  `subscription_rate_limit_tier`; the caption renders
  `phil@winder.ai · Max · 20x`, falling back to the Helix owner when a valid
  probe hasn't enriched the row yet.

## Problem 1b: setup tokens can never be profiled (scope wall)

`/api/oauth/profile` requires `any_of(user:profile, user:office)`. Setup
tokens (`claude setup-token`) are minted with inference scopes only — Anthropic
returns 403 "OAuth token does not meet scope requirement" for them, verified
against real Anthropic (2026-08-18, four header combinations, same 403). That
is by design: the Claude Code CLI guards the same call with a scope check and,
for `CLAUDE_CODE_OAUTH_TOKEN`, relies on the user set
`CLAUDE_CODE_SUBSCRIPTION_TYPE` / `CLAUDE_CODE_RATE_LIMIT_TIER` env vars.

Consequences:

- The profile fetch is skipped for `credential_type = setup_token` (a
  deterministic 403 is not worth sending every validation); it runs for oauth
  credentials, which may carry `user:profile` — those get the identity
  automatically.
- The connect dialog accepts a self-reported identity (account email, plan,
  rate-limit tier) that is stored on the row and shown in the caption. For
  oauth subs a later successful profile fetch overwrites it with the
  authoritative value; for setup tokens it is the only source.
- One shared frontend formatter (`formatClaudeAccountIdentity` in
  `claudeSubscriptionUtils.ts`) renders the line in both places: the
  agent-settings caption and the account-settings Claude Code Subscription
  pill (which replaces the old plan-only chip) — e.g. `phil@winder.ai ·
  Max · 20x`. The two surfaces cannot drift.

## Problem 1c: asking the user to type their own identity

The setup-token dialog carried a "Which Claude account is this? (optional)"
form — account email, plan select, rate-limit tier. That was wrong on three
counts:

- **Unverifiable.** Free text with no server-side validation, rendered next to
  agents with the same visual authority as a profile fetch. Anyone could type
  any email onto any token.
- **Unanswerable.** "Rate-limit tier" appears on no page a user sees, and the
  placeholder asked for `20x` when Anthropic's actual value is
  `default_claude_max_20x`.
- **Unnecessary.** Anthropic returns `anthropic-organization-id` on *every*
  `/v1/messages` response, with no OAuth scope requirement — including on the
  401 that rejects a revoked token. Verified live 2026-08-19; the uuid matches
  `oauthAccount.organizationUuid` exactly. That is the identity signal setup
  tokens *do* disclose, and the liveness probe already makes the request.

### Fix

- `anthropic.Probe` replaces the `(ProbeResult, string, string)` tuple returned
  by `ProbeClaudeSubscription` / `ValidateSubscription`, carrying
  `OrganizationID` alongside result, detail and token.
- `ClaudeSubscription.ClaudeOrganizationID` persists it. `revalidate` only ever
  widens it — an inconclusive probe that never reached Anthropic must not wipe
  a previously captured id.
- `CreateClaudeSubscriptionRequest` no longer accepts `account_email`,
  `subscription_type` or `rate_limit_tier`; the form is gone.
- When a setup-token row has no email, the status endpoint resolves one from a
  *sibling subscription on the same Claude org* that has been profiled
  (`claudeIdentityForOrg`). Scoped to rows already visible in that context (the
  app owner's, then the app's org) on purpose — a global lookup by org uuid
  would leak an unrelated Helix org's email.
- The UI falls back to `Claude org f2f721d7` (first uuid segment) when no email
  is known: verified, comparable between subscriptions, and honest about what
  we actually know.

**Verified with a real setup token, 2026-08-19.** Revalidating
`csub_01m0djfrap71qq01amyq6z3101` (`credential_type = setup_token`, previously
carrying no plan, tier or email at all) through the live dev stack persisted
`claude_organization_id = f2f721d7-f975-426f-bb19-b0b45a3a9d52` — the same org
uuid the owner's `~/.claude.json` records. So a setup token *does* identify its
Claude organization; only the email is scope-gated. The "you can't tell whose
subscription this is" premise does not hold for either credential type.

## Problem 1d: the dialog only offered the credential that hides the account

Setup tokens were the *only* way the UI let you connect Claude, and they are
precisely the credential that cannot be profiled. Meanwhile the API had always
accepted `credentials.claudeAiOauth` — the shape of `~/.claude/.credentials.json`
— and nothing sent it.

Local tools (t3code and friends) never run an OAuth flow of their own: they read
the credentials `claude login` already wrote on the user's machine. Those tokens
carry `user:profile`, which is exactly why they can name the account. Helix is a
server and cannot read the file, but it can accept a paste of it — the same
affordance Codex already had as "Import auth.json".

### Fix

The connect dialog now offers two methods, defaulting to **Use my Claude
login**: paste the output of `cat ~/.claude/.credentials.json`.
`parseClaudeCredentials` accepts either the whole file or the inner
`claudeAiOauth` object, since people copy both.

Verified live 2026-08-19 by connecting a real credentials file through the API:
create returns `credential_type=oauth`, and because create already revalidates,
the profile fetch lands immediately — `account_email=karolis.rusenas@gmail.com`,
`subscription_type=max`, `rate_limit_tier=default_claude_max_20x`. The org
providers row went from `Claude org f2f721d7` to
`karolis.rusenas@gmail.com · Max · 20x`.

So the identity ceiling is a property of the *credential*, not of Helix: setup
token -> organization uuid only; credentials file -> full account identity.
The dialog now says so at the point of choice.

## Correction: refreshing does not extend the login window

Earlier notes in this document and in the refresher's comments claimed that
because Anthropic rotates the refresh token on every use, refreshing "rolls the
9-day window forward indefinitely". That is wrong.

Measured on a live credential: two readings 9.2 hours apart, with a real token
refresh in between (the credentials file was rewritten, and the access token was
4.4h into its 8h life), showed the refresh window shrink from 9.25 to 8.82 days
— it declines with wall clock. Anthropic's own docs say the same thing about the
login behind it: "The login lifetime itself is unchanged."

So the actual lifecycle is:

| Credential | Access token | Overall life |
|---|---|---|
| OAuth (sign in / paste credentials) | 8h, refreshed automatically | **~9 days**, then sign in again |
| Setup token | n/a | **1 year** |

The background refresher is still worth having — without it the access token
lapses after 8h unless a session happens to refresh it, which is what made
subscriptions go red while idle. It just cannot make one immortal, and the
connect dialog now says so rather than promising "stays connected".

This also makes the setup token the right choice for genuinely unattended use,
despite being the credential that cannot name its own account.

## Removed: the desktop-session login

Helix already had a second Claude login: `POST /claude-subscriptions/start-login`
provisioned a **full GNOME desktop** (`DesktopType: "ubuntu"`), ran
`claude auth login` in it via `helix-claude-auth-wrapper.sh`, and
`poll-login/{sessionId}` scraped either the OAuth URL from the wrapper's stdout
or `~/.claude/.credentials.json` once the login finished. Giving the container a
real browser is how it avoided having to feed a pasted code back into the CLI's
stdin.

It was never wired into the frontend — only the generated client had the
methods, with no importers — which is why the connect dialog offered nothing but
setup tokens until this branch.

Server-side PKCE replaces it and is strictly cheaper: no container at all, so it
still works when sandbox capacity is exhausted, and it yields the same
credential (verified end-to-end — a real sign-in produced an `oauth` row with
full identity, which the background refresher then kept alive).

Removed: both handlers and their routes, `ClaudeLoginSessionResponse` /
`ClaudePollLoginResponse`, the login-session constants, the wrapper script, its
`Dockerfile.ubuntu-helix` install and the `helix-claude-auth-wrapper` entry in
the desktop exec allowlist.

Kept, because Codex still provisions a desktop for its device flow:
`cleanupSubscriptionLoginSession*`, `isTemporarySubscriptionLoginSession`,
`execInContainer`, and `npm` in the exec allowlist. The shared cleanup test now
exercises the Codex constants rather than the deleted Claude ones.

## Problem 2: the liveness probe used a retired model and 404'd

`ProbeClaudeSubscription` pinned `claude-3-5-haiku-latest` (a 2024 model).
Anthropic returns **404** (not 401) for deprecated model ids on `/v1/messages`,
which the probe classifies as *inconclusive* — so since the model was retired
(evidence: 404s already in the api logs 2026-08-18 17:26 with the pre-change
binary, while a fresh dummy token 401s normally) **no subscription had ever
actually been probed valid**; `status="active"` was only the create-time
default and `valid=true` in the status API was a stale inference.

Fix: probe `claude-haiku-4-5` (current generation, cheapest), and include a
512-byte response-body preview in the detail on unexpected statuses so the next
model retirement is diagnosable from the log in one line.

## Incident note (2026-08-18, dev inner stack)

While testing the negative path, a `setup_token` was posted through the live
`POST /claude-subscriptions` endpoint as the dev test user. The endpoint
deletes that owner's existing subscriptions on re-auth (existing behaviour),
which destroyed the operator's real setup-token row in the inner dev DB. The
row is unrecoverable; the operator re-entered the token. Keep negative tests
off live user rows (the unit tests with httptest cover these paths).
