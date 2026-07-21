# Implementation Tasks: Fix Claude Subscription UX — Whose Sub, Cross-User Edits, Legible Auth Errors

## Foundation — liveness probe & data model
- [ ] Add `LastValidatedAt *time.Time` to `types.ClaudeSubscription`; confirm GORM AutoMigrate adds the column.
- [ ] Create `api/pkg/anthropic/subscription_probe.go`: `ProbeClaudeSubscription(ctx, token)` → POST `api.anthropic.com/v1/messages` with `anthropic-beta: oauth-2025-04-20`; 401=invalid, 200/429=valid, network/5xx=inconclusive; ~5s timeout.
- [ ] Add `ValidateSubscription(ctx, sub)` wrapper: decrypt creds, pick bearer (`setup_token`→SetupToken, `oauth`→AccessToken with refresh-if-expired), probe, persist `Status`/`LastError`/`LastValidatedAt` via `UpdateClaudeSubscription`.
- [ ] Unit tests for the probe (401/429/200/network branches) using gomock + suite pattern.

## Validation wiring
- [ ] `createClaudeSubscription`: run `ValidateSubscription` after store instead of hard-coding `Status="active"`.
- [ ] `validateProvidersAndModels` (subscription branch): resolve the **app owner's** effective subscription, validate it, return a **soft warning** (non-blocking) in the save response.
- [ ] `subscriptionEnvForSession` / `addUserAPITokenToAgent`: on subscription mode with no valid owner subscription, surface the legible error at session start instead of silently degrading.

## Owner-scoped status endpoint (Story 1)
- [ ] Add `GET /api/v1/apps/{id}/claude-subscription-status` returning `{connected, valid, owner_id, owner_name, owner_type, status, last_validated_at, last_error}` for the **app owner** (authorise caller on the app).
- [ ] Add swagger annotations and run `./stack update_openapi` to regenerate the client.

## Frontend callout (Story 1 + Story 2 warning)
- [ ] In `AppSettings.tsx` subscription branch (lines 703-775): add always-on info callout — "sessions authenticate with the session owner's subscription, not yours".
- [ ] Add React Query hook for the owner-status endpoint (generated API client). When app owner ≠ current user, name the owner and show their subscription status.
- [ ] Drive the subscription radio's "connected ✓" / "(not connected)" from owner status, not `useClaudeSubscriptions()` (fix the editor-scoped false positive).
- [ ] Show the "<owner> has no working Claude subscription…" warning when owner status is `connected && !valid`.
- [ ] Mirror the callout in shared coding-agent config (`CodingAgentForm.tsx` / `useCodingAgentProviderState.ts`) if low-cost; else note as follow-up.

## Legible session error (Story 3 — Helix-only primary)
- [ ] In `handleChatResponseError` (`websocket_external_agent_sync.go:3853`): when the interaction's session is subscription-mode and the error is the generic mid-turn string, re-probe the resolved owner token; on 401 rewrite `interaction.Error` to the legible subscription-auth message.
- [ ] (Optional, cross-repo — get review first) Propagate the real cause from Zed: payload on `AcpThreadEvent::Error` (`acp_thread.rs:3073`), forward at `thread_service.rs:1140`, add `error_kind` to `ChatResponseError` (`types.rs:242`), map it in Helix; bump `sandbox-versions.txt`.

## End-to-end verification in inner Helix (mandatory — show real UI)
- [ ] Register two users; user A edits user B's agent → subscription mode → screenshot callout naming B + B's sub status; verify warning when B has no/invalid sub.
- [ ] Owner with invalid token: run a turn (via a **spec task** for a live Zed) → screenshot session error showing subscription-auth message (not "process exited").
- [ ] Happy path: owner with valid token authenticates and a turn succeeds.
- [ ] `go build ./pkg/...` + `cd frontend && yarn build`; push and confirm Drone CI green.

## Wrap-up
- [ ] Write a `design/2026-07-DD-*.md` note in the helix repo capturing the whose-sub resolution and the probe gotcha (OAuth beta header).
- [ ] Open a PR against `helixml/helix` (and, if the Zed path is built, the coordinated `helixml/zed` PR + version bump per CLAUDE.md ordering).
