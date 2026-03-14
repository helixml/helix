# Design: Fix Claude Code Auto-Suggestion with Global Anthropic Provider

## Architecture Notes

**Pattern discovered:** The recent PR (commit `3b22af89f`) established the correct pattern for checking Anthropic provider availability. It changed all checks from filtering by `ProviderEndpointTypeUser` to checking any provider by name:

```typescript
// WRONG (old, still in Onboarding.tsx):
const hasAnthropicProvider = connectedProviderIds.has("anthropic")
// where connectedProviderIds only includes user providers

// CORRECT (pattern from AgentSelectionModal, CreateProjectDialog, etc.):
const hasAnthropicProvider = useMemo(() => {
  if (!providerEndpoints) return false
  return providerEndpoints.some(p => p.name === 'anthropic')
}, [providerEndpoints])
```

**Auto-select pattern (from `NewSpecTaskForm.tsx`):**
```typescript
if (hasClaudeSubscription || hasAnthropicProvider) {
  setCodeAgentRuntime("claude_code")
  setClaudeCodeMode(hasAnthropicProvider ? "api_key" : "subscription")
}
```
This correctly handles all cases: api_key takes priority when Anthropic provider exists.

## Files to Change

### 1. `frontend/src/pages/Onboarding.tsx`

**Fix 1 — `hasAnthropicProvider` computation (line 312):**

Replace the `connectedProviderIds.has("anthropic")` check with a proper `useMemo` over all `providers` (same variable already fetched in the component), checking all endpoint types:

```typescript
// Before (line 312):
const hasAnthropicProvider = connectedProviderIds.has("anthropic");

// After:
const hasAnthropicProvider = useMemo(() => {
  if (!providers) return false
  return providers.some(p => p.name === 'anthropic')
}, [providers])
```

**Fix 2 — Auto-select logic (lines 545-555):**

Expand to also auto-select Claude Code (api_key mode) when Anthropic provider is available:

```typescript
// Before:
useEffect(() => {
  if (hasClaudeSubscription && !hasAnthropicProvider && connectedProviderIds.size === 0) {
    setCodeAgentRuntime("claude_code")
    setClaudeCodeMode("subscription")
  }
}, [hasClaudeSubscription, hasAnthropicProvider, connectedProviderIds.size])

// After:
useEffect(() => {
  if (hasAnthropicProvider) {
    setCodeAgentRuntime("claude_code")
    setClaudeCodeMode("api_key")
  } else if (hasClaudeSubscription && connectedProviderIds.size === 0) {
    setCodeAgentRuntime("claude_code")
    setClaudeCodeMode("subscription")
  }
}, [hasClaudeSubscription, hasAnthropicProvider, connectedProviderIds.size])
```

Note: `hasAnthropicProvider` takes priority (api_key mode) — consistent with `NewSpecTaskForm`. The `connectedProviderIds.size === 0` guard for subscription mode ensures we don't override user's explicitly configured providers.

### 2. `frontend/src/components/project/AgentSelectionModal.tsx`

`hasAnthropicProvider` was already fixed (includes global providers). But auto-select is still subscription-only (line 135).

**Fix — Add api_key auto-select (lines 133-139):**

```typescript
// Before:
if (hasClaudeSubscription && !hasAnthropicProvider && userProviderCount === 0) {
  setCodeAgentRuntime("claude_code")
  setClaudeCodeMode("subscription")
}

// After:
if (hasAnthropicProvider) {
  setCodeAgentRuntime("claude_code")
  setClaudeCodeMode("api_key")
} else if (hasClaudeSubscription && userProviderCount === 0) {
  setCodeAgentRuntime("claude_code")
  setClaudeCodeMode("subscription")
}
```

## What Was Already Fixed (for reference)

The following files were fixed in commit `3b22af89f` and do NOT need changes:
- `frontend/src/components/project/AgentSelectionModal.tsx` — `hasAnthropicProvider` (but auto-select still needs updating, see above)
- `frontend/src/components/project/CreateProjectDialog.tsx`
- `frontend/src/components/tasks/NewSpecTaskForm.tsx`
- `frontend/src/pages/ProjectSettings.tsx`

## Gotchas

- `Onboarding.tsx` uses `providers` (from `useListProviders`), not `providerEndpoints`. The variable is named differently but holds the same data. The fix should use the `providers` variable already available.
- The auto-select `useEffect` should not run if providers haven't loaded yet. The `useMemo` returning `false` when `!providers` handles this correctly.
- The `connectedProviderIds.size === 0` guard in subscription auto-select prevents overriding cases where the user has explicitly added user providers (where they'd presumably want the regular model picker, not Claude Code subscription).
