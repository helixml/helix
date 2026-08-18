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
