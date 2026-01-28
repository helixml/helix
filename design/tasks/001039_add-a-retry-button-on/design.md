# Design: Retry Button for External Agent Errors

## Problem

Currently, when an external agent (Zed) interaction fails, users see an error message but have no way to retry the failed prompt. The existing retry mechanism is explicitly disabled for external agents in `EmbeddedSessionView.tsx`:

```typescript
// Regenerate doesn't work for external agents, so disable it
const isExternalAgent = session.config?.agent_type === AGENT_TYPE_ZED_EXTERNAL
onRegenerate={isExternalAgent ? undefined : handleRegenerate}
```

## Root Cause Analysis

After deeper investigation, **the retry button was disabled because it would send an empty message**.

### The Bug Chain

1. **Retry button passes empty string**: In `InteractionInference.tsx` line 385:
   ```typescript
   onClick={() => onRegenerate(interaction.id || '', '')}  // Empty message!
   ```

2. **EmbeddedSessionView's handleRegenerate is too simple**: Unlike `Session.tsx` which rebuilds the full message history, `EmbeddedSessionView.tsx` just passes the message through:
   ```typescript
   const handleRegenerate = useCallback(async (interactionID: string, message: string) => {
       await NewInference({
         message: message,  // Empty string = nothing sent to Zed!
         sessionId: sessionId,
         type: SESSION_TYPE_TEXT,
       })
   }, [...])
   ```

3. **For external agents, empty message = failure**: The backend requires a user message to send to Zed. An empty message would fail or do nothing useful.

### Contrast with Working Regenerate Button

The "Regenerate" button (for assistant messages, line 260) works correctly because it passes the original prompt:
```typescript
onClick={() => onRegenerate(interaction.id || '', interaction.prompt_message || '')}
```

## Solution

Fix the retry button to pass the original prompt message instead of an empty string.

### Option A: Fix at InteractionInference.tsx (Recommended)

Change the retry button onClick from:
```typescript
onClick={() => onRegenerate(interaction.id || '', '')}
```
To:
```typescript
onClick={() => onRegenerate(interaction.id || '', interaction.prompt_message || '')}
```

This matches the behavior of the working "Regenerate" button and fixes retry for ALL agent types.

### Option B: Fix at EmbeddedSessionView.tsx

Modify `handleRegenerate` to look up the original prompt when message is empty. This is more complex and less clean.

## Interaction States (from `api/pkg/types/enums.go`)

- `InteractionStateWaiting` - in progress
- `InteractionStateComplete` - finished successfully  
- `InteractionStateError` - failed (this is when retry should show)

## Architecture Flow (After Fix)

```
User clicks Retry
        ↓
InteractionInference: onRegenerate(interaction.id, interaction.prompt_message)
        ↓
EmbeddedSessionView.handleRegenerate(id, originalPrompt)
        ↓
NewInference({ message: originalPrompt, sessionId, type: SESSION_TYPE_TEXT })
        ↓
Backend creates new interaction → routes to Zed agent with original prompt
```

## Files to Modify

1. **`frontend/src/components/session/InteractionInference.tsx`** - Fix retry button to pass `interaction.prompt_message` instead of empty string (line ~385)

2. **`frontend/src/components/session/EmbeddedSessionView.tsx`** - Remove the `isExternalAgent` check that disables `onRegenerate` (line ~390)

## Risk Assessment

**Low risk**: 
- The fix makes retry behavior consistent with the already-working regenerate button
- Worst case: if prompt_message is somehow empty, behavior is same as before (retry sends empty = fails)
- Benefits all agent types, not just external agents

## Implementation Notes

### Changes Made

1. **`InteractionInference.tsx` (line ~385)**: Changed retry button onClick from `onRegenerate(interaction.id || '', '')` to `onRegenerate(interaction.id || '', interaction.prompt_message || '')` - now passes the original prompt message instead of empty string.

2. **`EmbeddedSessionView.tsx` (line ~390)**: Removed the `isExternalAgent` conditional check. Changed `onRegenerate={isExternalAgent ? undefined : handleRegenerate}` to just `onRegenerate={handleRegenerate}`.

3. **Cleanup**: Removed unused `AGENT_TYPE_ZED_EXTERNAL` import from EmbeddedSessionView.tsx.

### Code Pushed

- Feature branch: `feature/001039-add-a-retry-button-on`
- Commit message: "feat: Enable retry button for external agent (Zed) errors"

### Testing Required

Manual UI testing needed to verify:
- Retry button appears when external agent interaction has error state
- Clicking retry resends the original prompt to Zed agent
- Retry works in both EmbeddedSessionView (floating desktop panel) and standalone session views