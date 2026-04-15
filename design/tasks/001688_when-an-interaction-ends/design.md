# Design: Skip Notification When User Is Active

## Key Files

- **`/api/pkg/server/websocket_external_agent_sync.go`** — `handleMessageCompleted()` (lines ~1995-2342) is where the attention event is emitted. This is the only change point.

## How It Works Today

When `message_completed` fires:
1. `targetInteraction.State` is set to `complete`
2. `attentionService.EmitEvent(agent_interaction_completed)` fires in a goroutine
3. This creates an attention event (DB + Slack + browser notification)

The key scenarios where the notification is pointless:
- **User interrupted the agent** — user sent a message, which stopped the agent mid-response. `handleMessageAdded` already created a new interaction with `state=waiting` before the agent's `message_completed` arrives. The agent finished *because* the user sent a message.
- **User sent a quick follow-up** — agent finished naturally, but user already typed another message. Same result: a newer `waiting` interaction exists.

## Approach

In `handleMessageCompleted()`, before calling `attentionService.EmitEvent(...)`, check whether the session already has a newer interaction. This is the same pattern `processPromptQueue` already uses (line ~2357) — it lists interactions to check if the last one is `waiting`.

**Condition to skip notification:**
```
session has an interaction where:
  - Created > targetInteraction.Created
  - State == InteractionStateWaiting
```

If such an interaction exists, skip `EmitEvent` and log at debug level.

## Implementation Detail

After setting `targetInteraction.State = InteractionStateComplete` and before the attention event goroutine, add:

```go
// Don't notify if the user is already active (sent a follow-up or interrupted)
skipNotification := false
interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
    SessionID:    helixSessionID,
    GenerationID: helixSession.GenerationID,
    PerPage:      1,
})
if err == nil && len(interactions) > 0 {
    lastInteraction := interactions[len(interactions)-1]
    if lastInteraction.State == types.InteractionStateWaiting && lastInteraction.Created.After(targetInteraction.Created) {
        log.Debug().Msg("skipping agent_interaction_completed: user already active in session")
        skipNotification = true
    }
}
```

Then wrap the existing `EmitEvent` block with `if !skipNotification { ... }`.

Uses `ListInteractions` with `PerPage: 1` — only needs to check the most recent interaction.

## Why Server-Side

The check must be server-side because:
1. The attention event triggers a Slack notification — suppressing client-side only would still post to Slack.
2. The server already has all the data needed without extra API calls.

## Edge Cases

- **Race condition:** User message arrives slightly after `message_completed` is processed. Acceptable — the window is very small and the occasional unnecessary notification is harmless.
- **Error state:** If the store query fails, emit the notification as normal (safe default).
