# Fix Claude-subscription UX: whose sub is used, cross-user agent edits, and legible auth errors

## Summary
Fixes a real incident where User A switched User B's agent to Claude **subscription**
mode, expecting A's own subscription to be used. Sessions actually authenticate with
the **session owner's** subscription (B's), whose token was invalid — every turn
failed with a useless generic error (`the ACP agent process exited mid-turn or hit
max tokens`). There was no UI affordance for whose subscription is used, no
validation that it works, and no legible surfacing of the auth failure.

This PR makes "whose subscription" explicit, validates the token with a real
liveness probe, and reclassifies the generic turn-failure into a legible
subscription-auth error — all in the helix repo, no Zed change required.

## Changes
- **Liveness probe** (`api/pkg/anthropic/subscription_probe.go`): probes a Claude
  subscription token against `api.anthropic.com/v1/messages` with the mandatory
  `anthropic-beta: oauth-2025-04-20` header. 401 = invalid, 200/429 = valid,
  network/5xx = inconclusive (never downgrades status on an ambiguous signal).
- **Validate on connect** (`claude_subscription_handlers.go`): `Status` now reflects
  a real probe instead of an unconditional `"active"`; persists `Status`,
  `LastError`, and a new `LastValidatedAt` (`types.ClaudeSubscription`).
- **Owner-status endpoint** `GET /api/v1/apps/{id}/claude-subscription-status`:
  resolves the **app owner's** effective subscription (the likely session owner) and
  reports `connected/valid/owner_name/is_current_user/last_error`. Re-probes when
  stale (>5min).
- **Legible session error** (`websocket_external_agent_sync.go`):
  `maybeReclassifySubscriptionAuthError` rewrites the generic ACP mid-turn abort into
  *"Claude subscription authentication failed for <owner> (invalid or expired token).
  Reconnect the subscription in Settings."* — only for subscription-mode Claude Code
  sessions whose owner sub is missing/invalid; all other errors pass through.
- **Agent settings UI** (`frontend/src/components/app/AppSettings.tsx`): an always-on
  callout ("sessions use the session owner's subscription, not yours"); the
  subscription option's connected state now derives from the **owner's** status
  (fixing a false "connected ✓" that showed the editor's own subscription); a warning
  Alert naming the owner when their subscription is missing/invalid.
- Regenerated OpenAPI TS client; unit tests for the probe and the reclassification wiring.

## Verification
- **Live, against real Anthropic:** connecting an invalid `sk-ant-oat…` token → probe
  returned 401 → row persisted `status=error`,
  `last_error="invalid or expired token (401 from Anthropic)"`, `last_validated_at`.
- **Live UI** (see screenshots): own-agent invalid-sub warning, and the cross-user
  incident (User A editing User B's agent → "chris@helix.ml has no working Claude
  subscription connected", subscription option disabled/"(not connected)").
- **Unit:** `TestProbeClaudeSubscription`, `TestReclassifySubAuthSuite`.
- **Not shown live:** the end-to-end turn-failure screenshot — the inner-Helix
  desktop/Zed sandbox would not provision, so a real failing Zed turn couldn't be
  produced. The reclassification is unit-proven and the render path is unchanged.

## Screenshots
![Own agent, invalid subscription warning](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002296_implement-the/screenshots/01-own-agent-invalid-subscription-warning.png)
![Cross-user: editing another user's agent](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002296_implement-the/screenshots/02-cross-user-agent-owner-no-subscription-warning.png)

## Notes / follow-ups
- Warn (not hard-block) at save time, per design Open Q1.
- Optional future work: propagate a real `error_kind` from Zed
  (`acp_thread`/`external_websocket_sync`) so non-subscription auth failures are also
  legible generically — deferred (needs a `helixml/zed` PR + `sandbox-versions.txt` bump).
- Mirroring the callout into the shared coding-agent config surfaces is a small follow-up.
