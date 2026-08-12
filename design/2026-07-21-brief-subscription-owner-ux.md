# Fix: Claude-subscription UX — whose sub is used, cross-user agent edits, and legible auth errors

## Problem (real incident, meta prod, 2026-07-21)

User A (Luke) opened User B's (Chris's) agent settings and switched the agent from
Anthropic **API key** mode to **Claude subscription** mode. Luke had a valid Claude
subscription connected to **his own** account, so he reasonably expected it to be
used. It wasn't. Every turn then failed, and the user-visible error was the
useless generic string:

> "agent turn aborted: the ACP agent process exited mid-turn or hit max tokens
>  (see Zed.log 'Error in run turn' for the cause)"

The actual cause (only visible by SSHing into the container and reading Zed.log)
was `Error in run turn: … API Error: 401 OAuth access token is invalid.
errorKind: authentication_failed`.

## Root cause (confirmed in code)

`api/pkg/server/external_agent_handlers.go` `subscriptionEnvForSession` resolves
the Claude token via:

```go
sub, err := apiServer.Store.GetEffectiveClaudeSubscription(ctx, session.Owner, session.OrganizationID)
```

i.e. it uses the **session owner's** subscription (user-level first, then
org-level fallback), NOT the editing user's, and NOT the agent-app owner's
necessarily. So when Luke flips Chris's agent to subscription mode, it silently
means "use **Chris's** Claude subscription". Chris's stored token was invalid →
401. There is **no UI affordance** telling the editor whose subscription will be
used, **no validation** that the resolved owner even has a working subscription,
and **no legible surfacing** of the auth failure. The token is injected as
`CLAUDE_CODE_OAUTH_TOKEN` at desktop-start, so failures only appear at first turn.

## Scope of this task — three improvements

### 1. Make "whose subscription" explicit in the agent-settings UI
When a user selects **subscription** credential mode for an agent/assistant, the
UI must state, in plain language, **which account's Claude subscription will
authenticate the agent** — the session owner's — and that it is NOT the editing
user's. E.g. a callout: *"Sessions from this agent authenticate with the session
owner's connected Claude subscription. If someone else runs this agent, their own
subscription is used — not yours."* If editing another user's agent, name that
owner and whether they currently have an active subscription connected.

### 2. Validate at save time (and/or at session start)
When subscription mode is selected/saved, check that the account whose sub will be
used has an **active, non-expired, actually-valid** Claude subscription. If not,
**block or warn clearly**: *"<owner> has no working Claude subscription connected —
the agent will fail to authenticate. Connect one, or use API-key mode."* A cheap
liveness probe: call `https://api.anthropic.com/v1/messages` with
`Authorization: Bearer <oat>` + `anthropic-beta: oauth-2025-04-20`; **401 =
invalid**, 429/200 = accepted (429 is just a throttle, still "valid"). Consider
recording `last_error` / `last_validated_at` on the `claude_subscriptions` row and
showing it in settings.

### 3. Surface the real auth error to the user
Propagate the underlying ACP failure reason instead of the generic
"agent process exited". When Zed reports `errorKind: authentication_failed` /
`401 OAuth access token is invalid`, the Helix session error shown in the UI must
say something like *"Claude subscription authentication failed for <owner> (invalid
or expired token). Reconnect the subscription in Settings."* Trace where the
generic string is produced (search `agent turn aborted` / `exited mid-turn or hit
max tokens` in `api/pkg/server/` and the Zed
`external_websocket_sync` `chat_response_error` emission) and pass the specific
`authentication_failed` reason through.

## Optional (call out, don't necessarily build): let the agent specify a sub owner
Today the sub is strictly `session.Owner`. Consider whether an agent config should
be able to pin an explicit subscription (e.g. an **org-level** shared Claude
subscription) so cross-user agents don't silently depend on who runs them. If you
add this, keep it minimal and data-driven. Get review before expanding scope.

## Acceptance criteria
Test in the inner Helix browser end-to-end:
1. As user A, edit user B's agent → switch to subscription mode → the UI clearly
   shows B's subscription is what's used, and warns if B has none/invalid.
2. Trigger an auth failure (owner with an invalid token) → the **session error in
   the UI** names it as a subscription-auth failure, not "process exited".
3. Happy path still works (owner with a valid token authenticates).

Do not report "done" from unit tests alone — show the actual UI states
(screenshots/DOM) for the warning and the legible error. Related incident notes:
the session-owner resolution and recovery are documented in the 2026-07-21 memory.
