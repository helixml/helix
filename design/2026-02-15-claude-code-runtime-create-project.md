# Claude Code Runtime in CreateProjectDialog & AgentSelectionModal

**Date:** 2026-02-15
**Status:** Implemented

## Problem

The Onboarding flow already supports `claude_code` as a runtime with subscription/API key credential selector. But `CreateProjectDialog` and `AgentSelectionModal` -- the two other places where agents are created -- only had `zed_agent` and `qwen_code`. Users with Claude subscriptions had no way to create Claude Code agents outside of onboarding.

## Changes

### Files Modified

1. `frontend/src/components/project/CreateProjectDialog.tsx`
2. `frontend/src/components/project/AgentSelectionModal.tsx`
3. `frontend/src/pages/Onboarding.tsx`

### What Was Added

**Both CreateProjectDialog and AgentSelectionModal:**

- Added `useClaudeSubscriptions()` hook to detect Claude subscription status
- Added `useListProviders({ loadModels: false })` to detect Anthropic API key providers
- Added `claudeCodeMode` state (`'subscription' | 'api_key'`)
- Added Claude Code as a third `MenuItem` in the runtime dropdown (with note that it works with Claude subscriptions)
- Added credential selector (Radio group: subscription vs API key) shown when `claude_code` is selected
- Model picker is hidden when subscription mode is selected (subscription doesn't need a model)
- Updated validation to allow agent creation without a model when using `claude_code` + `subscription`
- Auto-default effect: when the user has a Claude subscription but no Anthropic API key and no other user providers, auto-select `claude_code` + `subscription`

**Onboarding.tsx:**

- Added the same auto-default effect (was missing)
- Updated Claude Code dropdown description to mention it works with Claude subscriptions

### Auto-Default Logic

```tsx
useEffect(() => {
  if (hasClaudeSubscription && !hasAnthropicProvider && userProviderCount === 0) {
    setCodeAgentRuntime('claude_code')
    setClaudeCodeMode('subscription')
  }
}, [hasClaudeSubscription, hasAnthropicProvider, userProviderCount])
```

This fires when:
- User has at least one Claude subscription connected
- User has NOT configured an Anthropic API key
- User has NO other user-type provider endpoints configured

## Verification

- `cd frontend && yarn build` passes with no TypeScript errors
- Manual testing: Create Project dialog shows Claude Code in dropdown
- When Claude subscription is the only AI provider, Claude Code auto-selects
- Subscription mode hides model picker, create button remains enabled
