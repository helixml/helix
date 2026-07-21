# Design: Fix Claude Subscription UX — Whose Sub, Cross-User Edits, Legible Auth Errors

## Overview

Three coordinated changes on top of the existing Claude-subscription plumbing:

1. **UI callout** in agent settings naming whose subscription authenticates the
   agent, backed by the **agent owner's** resolved subscription status.
2. **A liveness probe** (`api.anthropic.com/v1/messages` with the OAuth beta
   header) reused at three points: subscription connect, agent save, and session
   start. Results persist on the `claude_subscriptions` row.
3. **Legible auth errors** in the session UI, via a Helix-side reclassification of
   the failure (primary) and, optionally, real error propagation from Zed
   (thorough follow-up).

The work is data-driven and reuses existing code paths — no new subsystems.

## Key Code Facts (grounding)

| Concern | Location |
|---|---|
| Session-owner token resolution | `api/pkg/server/external_agent_handlers.go:130` `subscriptionEnvForSession` → `GetEffectiveClaudeSubscription(session.Owner, session.OrganizationID)` (line 145) |
| Effective-sub resolution (user→org) | `api/pkg/store/store_claude_subscription.go:104` |
| Subscription model | `api/pkg/types/claude_subscription.go:12` — has `Status`, `LastError` (defined, **never written**), `LastRefreshedAt`; **no `LastValidatedAt`** |
| Connect handler (no validation) | `api/pkg/server/claude_subscription_handlers.go:35` — `Status` hard-set `"active"` (line ~140) |
| List endpoint (caller's subs) | `GET /api/v1/claude-subscriptions` → `claude_subscription_handlers.go:168` |
| Credential-type enum | `api/pkg/types/task_management.go:168` (`api_key` / `subscription`, `IsSubscription()`) |
| Assistant save + validation hook | `PUT /api/v1/apps/{id}` → `app_handlers.go:1081 updateApp` → `validateProvidersAndModels:651` (credential-type branch at 739-750) |
| Credential-mode UI | `frontend/src/components/app/AppSettings.tsx:703-775` (radio group; precedent warning callout at 771-775) |
| Editor-scoped sub check (the false-positive) | `AppSettings.tsx:288-293` `useClaudeSubscriptions()` — lists the **caller's** subs |
| Error chain (Zed→Helix→UI) | `zed/crates/acp_thread/src/acp_thread.rs:3073` (discards cause) → `zed/crates/external_websocket_sync/src/thread_service.rs:1140` (hardcoded string) → `types.rs:242` `ChatResponseError{request_id,error}` → `helix .../websocket_external_agent_sync.go:3853 handleChatResponseError` → `interaction.Error` → `InteractionInference.tsx:551` |

## Design Decisions

### 1. Liveness probe helper (foundation — build first)

New helper, e.g. `api/pkg/anthropic/subscription_probe.go`:

```
ProbeClaudeSubscription(ctx, token string) (valid bool, detail string)
```

- `POST https://api.anthropic.com/v1/messages`
- Headers: `Authorization: Bearer <token>`, `anthropic-beta: oauth-2025-04-20`,
  `anthropic-version: 2023-06-01`, `content-type: application/json`.
- Minimal body: `{"model":"claude-3-5-haiku-latest","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`.
- **401 → invalid** (`authentication_failed`); **200 / 429 → valid**; network/5xx →
  inconclusive (treat as "unknown", do not mark invalid).
- **Gotcha:** the `anthropic-beta: oauth-2025-04-20` header is mandatory — without
  it Anthropic rejects OAuth tokens with "only authorized for Claude Code" (see
  `design/2026-02-14-claude-subscription-provider.md`). Short timeout (~5s).

A `ValidateSubscription(ctx, sub)` wrapper decrypts the stored credentials
(`crypto.DecryptAES256GCM`), selects the bearer token (`setup_token` → SetupToken;
`oauth` → AccessToken, **refreshing first if expired** — see Open Question 3),
probes, then writes back `Status` (`active`/`error`), `LastError`, and new
`LastValidatedAt` via `UpdateClaudeSubscription`.

### 2. Data model — add `LastValidatedAt`, actually use `Status`/`LastError`

Add one field to `types.ClaudeSubscription` (GORM AutoMigrate handles the column):

```go
LastValidatedAt *time.Time `json:"last_validated_at,omitempty"`
```

Reuse existing `Status` (`active`/`error`) and `LastError` — currently dead. The
list/get endpoints already serialise the row, so the frontend gets these for free.

### 3. Where validation runs

- **On connect** (`createClaudeSubscription`): after storing, run
  `ValidateSubscription` and persist the result. Replaces the always-`"active"`
  assumption with a real status.
- **On agent save** (`validateProvidersAndModels`, subscription branch): resolve
  the **app owner's** effective subscription and validate it. Return a **soft
  warning** (non-blocking — see Open Question 1) surfaced in the save response so
  the UI can show it. Do not hard-fail the save.
- **On session start** (`subscriptionEnvForSession` / caller in
  `addUserAPITokenToAgent`): when subscription mode resolves no valid subscription
  for `session.Owner`, do not silently return base env. Instead mark the
  session/first interaction with the legible error (Story 3) so the doomed turn
  isn't started blind. Reuse `ValidateSubscription` (cheap; cache within the
  request).

### 4. "Whose subscription" status endpoint (Story 1 + save warning)

The existing `/api/v1/claude-subscriptions` returns the **caller's** subs — wrong
audience for a cross-user edit. Add a small read endpoint scoped to the app owner:

```
GET /api/v1/apps/{id}/claude-subscription-status
→ { connected: bool, valid: bool, owner_id, owner_name,
    owner_type, status, last_validated_at, last_error }
```

It authorises the caller on the app, resolves `GetEffectiveClaudeSubscription`
for the **app's** owner/org, optionally runs a fresh probe, and returns the
owner-scoped status. This backs both the callout's per-owner claim and the
"connected ✓" indicator, fixing the editor-scoped false positive.

### 5. Frontend callout (Story 1)

In `AppSettings.tsx`, within the subscription branch of the credential radio
(lines 703-775):
- Add an always-on info callout for subscription mode with the plain-language
  "session owner's subscription" explanation.
- Fetch the new owner-status endpoint (React Query hook, generated API client per
  CLAUDE.md). When `app.owner !== currentUser`, name the owner and show
  active/invalid state. Drive the subscription radio's "connected ✓" /
  "(not connected)" from this owner status, not `useClaudeSubscriptions()`.
- Show the save-time warning text when owner status is `connected && !valid`.

Mirror the same callout in the shared coding-agent config surfaces
(`CodingAgentForm.tsx` / `useCodingAgentProviderState.ts`) if low-cost; otherwise
scope to `AppSettings.tsx` and note the others as follow-up.

### 6. Legible session error (Story 3)

**Primary (Helix-only, testable in the inner loop):** In `handleChatResponseError`
(`websocket_external_agent_sync.go:3853`), when the interaction belongs to a
**subscription-mode** session and the error is the generic mid-turn string,
re-probe the resolved owner's token. On 401, replace `interaction.Error` with
*"Claude subscription authentication failed for <owner> (invalid or expired
token). Reconnect the subscription in Settings."* This directly resolves the
incident without a Zed rebuild. Combined with §3's session-start check, most auth
failures never even reach a doomed turn.

**Thorough (optional, cross-repo follow-up):** Propagate the real cause from Zed:
- `acp_thread.rs:3073` — make `AcpThreadEvent::Error` carry the error string/kind
  instead of an empty payload.
- `thread_service.rs:1140` — forward that into `ChatResponseError.error` and a new
  `error_kind` field.
- `types.rs:242` — add `error_kind` to the struct + serialisation.
- Helix `handleChatResponseError` — read `error_kind`; map `authentication_failed`
  to the legible message generically (covers non-subscription auth too).
- Requires a `helixml/zed` PR + `sandbox-versions.txt` bump (see CLAUDE.md build
  pipeline). Recommend deferring unless in scope.

## Testing Strategy (inner Helix, end-to-end — mandatory)

Per CLAUDE.md, verify live in the inner Helix at `localhost:8080`, not just unit
tests. Show real UI states (screenshots/DOM).

1. **Cross-user visibility:** register two users; user A edits user B's agent →
   switch to subscription mode → screenshot the callout naming B and B's sub
   status; verify the warning when B has no/invalid sub.
2. **Legible error:** give the owner an invalid token, run a turn → screenshot the
   session error showing the subscription-auth message (not "process exited").
   Live Zed connection required — use a spec task (provisions a git repo so Zed
   opens the sync WebSocket), not a bare chat session.
3. **Happy path:** owner with a valid token authenticates and a turn succeeds.
4. Unit tests (gomock, suite pattern per CLAUDE.md) for the probe (401/429/200
   branches) and validation persistence — as support, **not** as the acceptance
   evidence.

## Files Likely Touched

- `api/pkg/types/claude_subscription.go` — add `LastValidatedAt`.
- `api/pkg/anthropic/subscription_probe.go` — new probe + `ValidateSubscription`.
- `api/pkg/server/claude_subscription_handlers.go` — validate on connect.
- `api/pkg/server/app_handlers.go` — save-time owner-sub warning; new owner-status
  endpoint (+ swagger → `./stack update_openapi`).
- `api/pkg/server/external_agent_handlers.go` — session-start validation + legible
  failure.
- `api/pkg/server/websocket_external_agent_sync.go` — auth reclassification in
  `handleChatResponseError`.
- `frontend/src/components/app/AppSettings.tsx` (+ shared coding-agent config) —
  callout, owner-status hook, warning.
- (Optional Zed) `zed/crates/acp_thread/src/acp_thread.rs`,
  `external_websocket_sync/src/{thread_service,types}.rs` + `sandbox-versions.txt`.

## Implementation Notes (as built)

**What shipped (helix repo only — no Zed change was needed):**
- `api/pkg/anthropic/subscription_probe.go` — `ProbeClaudeSubscription` (raw 401/429/200
  classification, mandatory `anthropic-beta: oauth-2025-04-20` header) + `ValidateSubscription`
  (decrypt, pick bearer, probe). `subscriptionProbeURL` is a `var` so the httptest-based unit
  test can point it at a local server.
- `types.ClaudeSubscription.LastValidatedAt` added (GORM AutoMigrate picks up the column).
- `revalidateClaudeSubscription` (in `claude_subscription_handlers.go`) probes + persists
  `Status`/`LastError`/`LastValidatedAt`. Called on connect and by the status endpoint (re-probes
  when stale >5min). **Inconclusive never downgrades Status** (network blip / expired-refreshable
  oauth token must not read as "invalid").
- `GET /api/v1/apps/{id}/claude-subscription-status` → `AppClaudeSubscriptionStatus` — resolves the
  **app owner's** effective sub (the likely session owner), returns `connected/valid/owner_name/
  is_current_user/…`. Backs the callout and fixes the editor-scoped false positive.
- `handleChatResponseError` → `maybeReclassifySubscriptionAuthError`: only rewrites the specific
  generic ACP abort string (`genericACPAbortMarker`), and only for subscription-mode Claude Code
  sessions whose owner sub is missing/invalid. Everything else passes through unchanged.
- Frontend `AppSettings.tsx`: owner-status React Query hook; the subscription radio's connected
  state now derives from owner status; always-on "session owner's / not yours" callout; warning
  Alert for cross-user and own-agent invalid cases.

**Decisions & learnings:**
- **Warn, not block (Open Q1).** No hard save-block; the callout + owner-status endpoint surface the
  warning live, and the session-start `Status != "active"` guard + turn-time reclassification catch
  runtime. Simpler and avoids breaking legit "configure before owner connects" flows.
- **No Zed change required.** The Helix-only reclassification fully covers the incident's auth case
  and is testable in the inner loop. The general Zed `error_kind` propagation remains an optional
  follow-up (out of scope here).
- **OAuth-beta header is load-bearing** — without `anthropic-beta: oauth-2025-04-20`, Anthropic
  rejects subscription tokens with "only authorized for Claude Code" (would look like a false 401).
- **Expired-but-refreshable oauth tokens** are returned `ProbeInconclusive`, not `Invalid` — Claude
  Code refreshes them in-container; probing the stale access token would false-positive.
- **The connect endpoint requires a personal (not app-scoped) API key** — app keys hit
  "path not allowed"; used the browser session cookie instead.

## Verification (what was proven, honestly)
- **Live, against real Anthropic:** connected an invalid `sk-ant-oat…` setup token → probe returned
  401 → row persisted `status=error`, `last_error="invalid or expired token (401 from Anthropic)"`,
  `last_validated_at`. Confirms the whole validation mechanism end-to-end.
- **Live UI (screenshots):** (01) own-agent invalid-sub warning; (02) cross-user — Luke editing
  Chris-owned agent shows "chris@helix.ml has no working Claude subscription connected" + the radio
  disabled/"(not connected)". This is the exact incident, now visible before any turn runs.
- **Unit:** `TestReclassifySubAuthSuite` (reclassification wiring) + `TestProbeClaudeSubscription`
  (probe branches + beta header).
- **Not shown live:** the final Story-3 turn-failure screenshot — the inner-Helix desktop/Zed
  sandbox would not provision (`ubuntu-external` never started; interaction hit the no-connection
  watchdog, a different path). Auth reclassification therefore couldn't be exercised end-to-end via
  a real Zed turn. Happy path also not shown (no working Claude token in this env).
