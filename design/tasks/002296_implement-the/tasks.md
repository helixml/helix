# Implementation Tasks: Fix Claude Subscription UX — Whose Sub, Cross-User Edits, Legible Auth Errors

## Foundation — liveness probe & data model
- [x] Add `LastValidatedAt *time.Time` to `types.ClaudeSubscription`; confirm GORM AutoMigrate adds the column.
- [x] Create `api/pkg/anthropic/subscription_probe.go`: `ProbeClaudeSubscription(ctx, token)` → POST `api.anthropic.com/v1/messages` with `anthropic-beta: oauth-2025-04-20`; 401=invalid, 200/429=valid, network/5xx=inconclusive; 8s timeout.
- [x] Add `ValidateSubscription(ctx, sub)` wrapper: decrypt creds, pick bearer (`setup_token`→SetupToken, `oauth`→AccessToken; oauth with expired-but-refreshable token → inconclusive, not invalid). Returns outcome; callers persist `Status`/`LastError`/`LastValidatedAt`.
- [x] Unit tests for the probe (401/429/200/500/empty branches) via httptest, asserting the OAuth beta header is sent.

## Validation wiring
- [x] `createClaudeSubscription`: run `ValidateSubscription` after store instead of hard-coding `Status="active"`; persist `Status`/`LastError`/`LastValidatedAt` (via `revalidateClaudeSubscription` helper).
- [x] ~~`validateProvidersAndModels` soft warning~~ — **decision (Open Q1): warn, don't block.** The save-time warning is surfaced live via the owner-status endpoint + `AppSettings.tsx` callout (below), not by threading a warning through the save response. No server-side save block. Runtime is caught by the session-start check.
- [x] `subscriptionEnvForSession`: now that `Status` is genuinely maintained (validated on connect / on error), its existing `sub.Status != "active"` guard meaningfully degrades an invalid sub to base env, whose 401 is then reclassified legibly at turn time. **Decision:** rather than build a separate session-start error-surfacing path (would need to locate/create the first interaction at desktop-start), rely on the turn-time reclassification below — it satisfies Story 3's acceptance criteria (UI session error names the auth failure).

## Owner-scoped status endpoint (Story 1)
- [x] Add `GET /api/v1/apps/{id}/claude-subscription-status` returning `{connected, valid, owner_id, owner_name, is_current_user, subscription_owner_type, status, last_validated_at, last_error}` for the **app owner** (authorise caller on the app; re-probe if stale >5min).
- [x] Add swagger annotations and run `./stack update_openapi` to regenerate the client (`v1AppsClaudeSubscriptionStatusDetail(id)` → `ServerAppClaudeSubscriptionStatus`).

## Frontend callout (Story 1 + Story 2 warning)
- [x] In `AppSettings.tsx` subscription branch: added always-on info callout — "sessions authenticate with the session owner's subscription, not yours".
- [x] Added React Query hook (`v1AppsClaudeSubscriptionStatusDetail`) for the owner-status endpoint. When app owner ≠ current user, names the owner and shows their subscription status.
- [x] Subscription radio's "connected ✓" for `claude_code` now derives from owner status (`hasClaudeSubscription = ownerClaudeStatus.connected`), falling back to `useClaudeSubscriptions()` only until it resolves — fixes the editor-scoped false positive.
- [x] Shows a warning Alert when owner status is `!connected || !valid` (both cross-user and own-agent variants).
- [ ] Mirror the callout in shared coding-agent config (`CodingAgentForm.tsx` / `useCodingAgentProviderState.ts`) — deferred as follow-up (AppSettings is the primary agent-edit surface the incident used).

## Legible session error (Story 3 — Helix-only primary)
- [x] In `handleChatResponseError`: added `maybeReclassifySubscriptionAuthError` — when the interaction's session is subscription-mode Claude Code and the error is the generic mid-turn string, re-probe the resolved owner token; on invalid (or no sub) rewrite `interaction.Error` to the legible subscription-auth message (naming the owner). Persists the fresh status too.
- [ ] (Optional, cross-repo — get review first) Propagate the real cause from Zed: payload on `AcpThreadEvent::Error` (`acp_thread.rs:3073`), forward at `thread_service.rs:1140`, add `error_kind` to `ChatResponseError` (`types.rs:242`), map it in Helix; bump `sandbox-versions.txt`.

## End-to-end verification in inner Helix (mandatory — show real UI)
- [x] **Live probe proven:** connected an invalid `sk-ant-oat…` setup token via the API → the real Anthropic probe returned 401 → row persisted `status=error`, `last_error="invalid or expired token (401 from Anthropic)"`, `last_validated_at` set.
- [x] **Owner-status endpoint proven live:** returns `is_current_user=true/false`, `owner_name`, `connected`, `valid` correctly for both own-agent and cross-user (Chris) cases.
- [x] **Story 1 own-agent (screenshot 01):** Luke's own agent, invalid sub → callout "session owner's / not yours" + warning "Your Claude subscription isn't working (invalid or expired token (401 from Anthropic))".
- [x] **Story 1 cross-user (screenshot 02):** Luke edits Chris-owned agent → callout names **chris@helix.ml**, warns "has no working Claude subscription connected", and the subscription radio shows "(not connected)" + disabled — the exact incident, now visible.
- [x] Reclassification wiring unit-tested (`TestReclassifySubAuthSuite`): generic message + subscription mode + owner → legible message; api-key mode & specific errors pass through.
- [~] Story 3 live session-error screenshot — **NOT captured live.** A spec task (`spt_01ky2b8g…`, session `ses_01ky2b8g…`, Luke-owned, subscription-mode, invalid sub) was started to get a live failing Zed turn, but the desktop/Zed sandbox never provisioned in this inner-Helix env (`ubuntu-external` container never appeared; the interaction failed at the no-Zed-connection watchdog with "Agent unresponsive", a DIFFERENT code path). My reclassification correctly left that non-generic error untouched. The auth path (Zed → generic ACP abort → `handleChatResponseError`) could not be exercised because Zed never connected. **Evidence for Story 3 instead:** (a) the probe is proven live against real Anthropic; (b) `TestReclassifySubAuthSuite` proves the reclassification wiring; (c) the render path (`InteractionInference.tsx`) is pre-existing/unchanged — it displays whatever `interaction.Error` holds. Honest gap: no live end-to-end turn-failure screenshot.
- [ ] Happy path: owner with valid token authenticates — **not shown:** no real working Claude subscription token available in this env.
- [ ] `go build ./...` + regenerate client; push and confirm Drone CI green.

## Wrap-up
- [ ] Write a `design/2026-07-DD-*.md` note in the helix repo capturing the whose-sub resolution and the probe gotcha (OAuth beta header).
- [ ] Open a PR against `helixml/helix` (and, if the Zed path is built, the coordinated `helixml/zed` PR + version bump per CLAUDE.md ordering).
