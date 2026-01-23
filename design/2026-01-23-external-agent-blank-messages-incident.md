# Incident Report: External Agent Blank Messages

**Date**: 2026-01-23
**Duration**: ~6 hours debugging
**Severity**: High (complete feature broken)
**Status**: Resolved

## Summary

External agent sessions displayed empty message bubbles for both user messages and assistant responses. Messages appeared correctly during streaming but went blank upon completion.

## Timeline

| Time (UTC) | Event |
|------------|-------|
| 2025-05-12 | Latent bug introduced in `e76ca1ec8` - message render guarded by `onRegenerate` |
| 2026-01-22 18:06 | Commit `61387e47c` - WebSocket sync changes (red herring) |
| 2026-01-22 18:09 | Commit `6a465e4af` - "disable regenerate button for external agents" triggers latent bug |
| 2026-01-22 18:51 | Commit `a57681481` - Revert WebSocket changes (didn't fix it) |
| 2026-01-22 21:30 | Commit `a57126151` - Nessie fixes the actual bug |
| 2026-01-23 02:14 | Commit `f24c60a16` - Additional backend fixes (unrelated) |
| 2026-01-23 03:30 | Root cause finally identified through debugging |

## Root Cause

**The bug**: In `InteractionInference.tsx` line 181:
```typescript
message && onRegenerate && (
  // entire message rendering block
)
```

The `&& onRegenerate` condition incorrectly guarded the **entire message content**, not just the regenerate button.

**The trigger**: Commit `6a465e4af` correctly set `onRegenerate = undefined` for external agents (since regeneration doesn't apply to them):
```typescript
// Session.tsx:1346
onRegenerate={isExternalAgent ? undefined : onRegenerate}
```

**The effect**: `message && undefined` evaluates to `false`, so the entire `<Box>` containing `<Markdown>` never rendered.

## Why It Took 6 Hours to Find

1. **Noise from concurrent changes**: WebSocket sync, streaming, cache handling, and UI changes were all happening simultaneously. Every system was a suspect.

2. **Misleading symptoms**: The bug manifested as "blank after streaming completes" which pointed toward cache race conditions, not a simple render guard.

3. **Context loss**: Claude conversation compaction lost the history of which changes were made and why, making it impossible to trace "I just changed X and Y broke."

4. **Wrong file investigated**: Most debugging focused on `Interaction.tsx` when the bug was in `InteractionInference.tsx`.

5. **Latent bug pattern**: The bug existed for 8 months but only manifested when a specific condition (`onRegenerate = undefined`) was introduced.

## The Fix

```diff
// InteractionInference.tsx
- message && onRegenerate && (
+ message && (
    <Box>
      {/* message content */}
    </Box>
  )

// Move onRegenerate check to only guard the buttons, not content
+ {isFromAssistant && onRegenerate && (
    <Box className="action-buttons">
      {/* regenerate/copy buttons */}
    </Box>
  )}
```

## Lessons Learned

### 1. Git Bisect, Don't Panic
When something breaks after multiple changes, **stop and bisect** instead of trying to fix forward:

```bash
git bisect start
git bisect bad HEAD
git bisect good <last-known-good-commit>
# Test each commit until the breaking one is found
```

We spent 6 hours investigating cache race conditions, WebSocket sync, React Query, and database queries. A 15-minute bisect would have pointed directly at `6a465e4af` and the `onRegenerate = undefined` change.

**Roll back first, understand second.** Don't try to fix something you don't understand by adding more code.

### 2. Slow Down to Speed Up
When making multiple changes, test after each one. A quick refresh after "disable regenerate button" would have immediately shown blank messages.

### 3. Atomic Commits with Testing
Each logical change should be a separate commit with manual verification. This enables:
- Quick bisection when bugs appear
- Clear cause-and-effect relationships
- Easier rollback of specific changes

### 4. Context Preservation
When Claude context is compacted, critical debugging information is lost. Consider:
- Writing down what changes were made before context fills up
- Using git commit messages as documentation of intent
- Taking notes outside of Claude for debugging sessions

### 5. Check the Render Path
When UI shows "empty", trace the actual render path:
1. Is data received? (API response) ✓
2. Is data passed to component? (props) ✓
3. Is component rendering? (console.log in render) ✓
4. Is the JSX conditional allowing render? ← **Bug was here**

## Prevention

1. **Code review**: Look for overly broad guard conditions in JSX
2. **Testing**: Add tests for "message renders without optional callbacks"
3. **Feature flags**: Never use callback presence to guard content, only UI controls

## Related Commits

- `e76ca1ec8` - Bug introduced (May 2025)
- `6a465e4af` - Bug triggered (Jan 22, 2026)
- `a57126151` - Bug fixed (Jan 23, 2026)
