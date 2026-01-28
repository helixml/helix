# Design: Retry Button for External Agent Errors

## Problem

Currently, when an external agent (Zed) interaction fails, users see an error message but have no way to retry the failed prompt. The existing retry mechanism is explicitly disabled for external agents in `EmbeddedSessionView.tsx`:

```typescript
// Regenerate doesn't work for external agents, so disable it
const isExternalAgent = session.config?.agent_type === AGENT_TYPE_ZED_EXTERNAL
onRegenerate={isExternalAgent ? undefined : handleRegenerate}
```

## Solution

Enable the existing retry functionality for external agents. The infrastructure already exists in `InteractionInference.tsx` - we just need to pass the `onRegenerate` callback for external agent sessions.

## Key Findings

### Existing Error Handling
- `InteractionInference.tsx` already has a retry button that shows when `error` is present and `onRegenerate` is provided
- The button calls `onRegenerate(interaction.id, '')` which resends the original prompt
- External agents are intentionally excluded via the `isExternalAgent` check

### Why It Was Disabled
The comment says "Regenerate doesn't work for external agents" but doesn't explain why. Testing needed to verify if this is still true or was a temporary limitation.

### Interaction States (from `types/enums.go`)
- `InteractionStateWaiting` - in progress
- `InteractionStateComplete` - finished successfully  
- `InteractionStateError` - failed (this is when retry should show)

## Architecture

```
User clicks Retry
        ↓
EmbeddedSessionView.handleRegenerate()
        ↓
NewInference({ message, sessionId, type: SESSION_TYPE_TEXT })
        ↓
Backend creates new interaction → routes to Zed agent
```

## Design Decision

**Enable retry for external agents** by removing the `isExternalAgent` conditional in `EmbeddedSessionView.tsx`.

The `handleRegenerate` function uses `NewInference` which already supports external agents - it's the same code path used when sending new messages.

## Risk Assessment

**Low risk**: This change only removes a restriction. If there was a real incompatibility, the worst case is the retry fails with another error (same as current state, just with an extra click).

## Files to Modify

1. `frontend/src/components/session/EmbeddedSessionView.tsx` - Remove the `isExternalAgent` check that disables `onRegenerate`
