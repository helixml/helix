# Design: Skip Notification When User Already Replied

## Key Files

- **`/api/pkg/server/websocket_external_agent_sync.go`** — `handleMessageCompleted()` (lines ~1995-2342) is where the attention event is emitted. This is the only change point.
- **`/api/pkg/store/`** — Store interface used to query interactions by session.

## Approach

In `handleMessageCompleted()`, before calling `attentionService.EmitEvent(...)`, check whether the session already has a newer interaction created after the one that just completed.

**Condition to skip notification:**
```
session has an interaction where:
  - Created > targetInteraction.Created (or targetInteraction.Completed)
  - State == InteractionStateWaiting  (user already sent a follow-up)
  - SessionID == current session
```

If such an interaction exists, skip `EmitEvent` and log at debug level: `"skipping agent_interaction_completed notification: user already sent follow-up message"`.

## Implementation Detail

After setting `targetInteraction.State = InteractionStateComplete` and before the attention event goroutine, add:

```go
// Don't notify if the user already sent a follow-up message
interactions, err := store.ListInteractions(ctx, helixSessionID)
if err == nil {
    for _, i := range interactions {
        if i.State == types.InteractionStateWaiting && i.Created.After(targetInteraction.Created) {
            log.Debug().Msg("skipping agent_interaction_completed: user already replied")
            // skip EmitEvent
        }
    }
}
```

Use the existing store method for listing interactions by session (already used elsewhere in the file).

## Why Server-Side

The check must be server-side because:
1. The attention event also triggers a Slack notification — suppressing client-side only would still create the Slack noise.
2. The server already has all the data needed (session interactions) without any extra API calls.

## Edge Cases

- **Race condition:** User message arrives slightly after `message_completed` is processed. This is acceptable — the window is very small and the user will still see the notification go away quickly. No complex synchronization needed.
- **Multiple follow-ups:** Any waiting interaction in the session (regardless of count) suppresses the notification.
- **Error state:** If the store query fails, emit the notification as normal (safe default).
