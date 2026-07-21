# Requirements: Fix Claude Subscription UX — Whose Sub, Cross-User Edits, Legible Auth Errors

## Background

Real incident (meta prod, 2026-07-21): User A (Luke) opened User B's (Chris's)
agent settings and switched the agent from Anthropic **API-key** mode to **Claude
subscription** mode. Luke had a valid Claude subscription on his own account and
reasonably expected it to be used. It wasn't — the agent authenticates with the
**session owner's** subscription (Chris's), whose stored token was invalid. Every
turn failed with a useless generic error:

> "agent turn aborted: the ACP agent process exited mid-turn or hit max tokens
> (see Zed.log 'Error in run turn' for the cause)"

The true cause (`API Error: 401 OAuth access token is invalid. errorKind:
authentication_failed`) was only visible by SSHing into the container.

**Three gaps, all confirmed in code:**

1. **No affordance for "whose subscription".** `subscriptionEnvForSession`
   (`api/pkg/server/external_agent_handlers.go:145`) resolves the token via
   `GetEffectiveClaudeSubscription(ctx, session.Owner, session.OrganizationID)` —
   the session owner's, not the editing user's. The settings UI never says this.
   Worse, the UI's "connected ✓" checkmark comes from `useClaudeSubscriptions()`,
   which lists the **editing user's** subscriptions — so Luke saw his own green
   checkmark while the agent silently used Chris's broken token.
2. **No validation.** A Claude subscription's `Status` is hard-coded to `"active"`
   at connect time; nothing ever probes the token or writes `LastError`. A dead
   token looks healthy everywhere until the first turn 401s.
3. **No legible error.** The real `authentication_failed` cause is discarded in
   Zed (`acp_thread.rs:3073` emits a payload-less `AcpThreadEvent::Error`); the
   WebSocket sync substitutes the hardcoded generic string, which Helix writes
   verbatim to `interaction.Error` and shows in the UI.

## User Stories & Acceptance Criteria

### Story 1 — See whose subscription authenticates the agent
As a user editing an agent in subscription mode, I want the settings UI to state
plainly which account's Claude subscription authenticates it, so I don't assume
it's mine.

**Acceptance criteria**
- When **subscription** credential mode is selected for a `claude_code` agent, the
  settings panel shows a plain-language callout: *"Sessions from this agent
  authenticate with the session owner's connected Claude subscription. If someone
  else runs this agent, their own subscription is used — not yours."*
- When editing **another user's** agent (app owner ≠ current user), the callout
  additionally names the owner and states whether that owner currently has an
  active, valid Claude subscription connected (e.g. *"This agent is owned by
  chris@… who has an active Claude subscription"* / *"…who has no working Claude
  subscription — the agent will fail to authenticate"*).
- The "connected ✓" indicator for the subscription option reflects the **agent
  owner's** subscription status, not the editing user's.

### Story 2 — Be warned when the subscription won't work
As a user selecting subscription mode, I want to be warned at save time (and have
the failure caught at session start) if the relevant account has no valid Claude
subscription, so I don't ship a broken agent.

**Acceptance criteria**
- On saving an agent in subscription mode, if the agent owner has no active,
  non-expired, **actually-valid** Claude subscription, the UI shows a clear
  warning: *"<owner> has no working Claude subscription connected — the agent will
  fail to authenticate. Connect one, or use API-key mode."*
- Validity is checked with a cheap liveness probe: `POST
  https://api.anthropic.com/v1/messages` with `Authorization: Bearer <token>` and
  `anthropic-beta: oauth-2025-04-20`. **401 = invalid; 200/429 = valid** (429 is a
  throttle, still valid).
- The probe result is persisted on the `claude_subscriptions` row
  (`status`, `last_error`, new `last_validated_at`) and is visible in the Claude
  subscription settings.
- At session start, subscription mode with an invalid/missing owner subscription
  surfaces the legible error (Story 3) instead of silently degrading to a doomed
  turn.

### Story 3 — Understand auth failures from the session UI
As a user whose agent turn failed on authentication, I want the session error to
name it as a subscription-auth problem, so I can fix it without reading container
logs.

**Acceptance criteria**
- When a subscription-mode turn fails on authentication, the session error shown
  in the UI reads like *"Claude subscription authentication failed for <owner>
  (invalid or expired token). Reconnect the subscription in Settings."* — **not**
  "the ACP agent process exited mid-turn or hit max tokens".
- The generic string is no longer the user-facing text for this failure class.

## Out of Scope (call out, don't build without review)
- Letting an agent config pin an explicit subscription owner (e.g. an org-level
  shared Claude subscription) instead of strictly `session.Owner`. Noted in the
  design as a future option; do not build without review.
- Codex/ChatGPT subscription parity (same UI path exists but this task targets the
  Claude incident).

## Open Questions
1. **Block vs warn at save.** The brief says "block or warn clearly." Recommended:
   **warn, don't hard-block** (an editor may legitimately configure the agent
   before the owner connects a sub; session-start already fails safely). Confirm
   you don't want a hard save block.
2. **Whose subscription to validate at edit time.** There is no session yet at
   save time, so we validate the **app owner's** effective subscription as the
   best proxy for the eventual session owner. If agents are commonly run by users
   other than the owner, the callout's per-owner claim is only indicative. OK?
3. **Expired-but-refreshable OAuth tokens.** OAuth access tokens expire every ~8h
   and are refreshed by Claude Code inside the container. A raw probe of a merely
   expired (but refreshable) access token would 401 and falsely read "invalid".
   Recommended: for `oauth` creds, refresh first (or treat expired+refresh-token
   present as "unknown, not invalid"); for `setup_token` creds a 401 is
   definitive. Confirm the refresh-before-probe behaviour is acceptable, or accept
   a simpler probe that may occasionally over-warn.
4. **Requirement 3 scope / Zed change.** A fully general fix (any mid-turn cause,
   not just auth) requires a **Zed change** (`acp_thread`/`external_websocket_sync`
   to carry the real cause + a new `error_kind`), which means a second PR against
   `helixml/zed` plus a `sandbox-versions.txt` bump. A **Helix-only** path covers
   the incident's auth case without touching Zed: on a generic mid-turn error for
   a subscription-mode session, re-probe the owner's token and, on 401, rewrite the
   interaction error to the legible message. Recommended: ship the Helix-only path
   (testable in the inner loop) and do the Zed propagation as the thorough
   follow-up. Confirm this split, or require the Zed change in this task.
5. The brief references a "2026-07-21 memory" documenting session-owner resolution;
   it is not present in this workspace. The requirements above are derived solely
   from the brief and the code. Flag anything that memory would contradict.
