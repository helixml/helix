# Design: Fix Claude Code Auto-Suggestion with Global Anthropic Provider

## Root Cause

`hasAnthropicProvider` is computed independently in 6 components, with two still on the old broken version that filters to user-only providers:

| File | Pattern | Correct? |
|------|---------|----------|
| `CreateProjectDialog.tsx` | `providerEndpoints.some(p => p.name === 'anthropic')` | ✅ |
| `AgentSelectionModal.tsx` | same | ✅ |
| `ProjectSettings.tsx` | same | ✅ |
| `NewSpecTaskForm.tsx` | same | ✅ |
| `Onboarding.tsx` | `connectedProviderIds.has("anthropic")` (user-only) | ❌ |
| `AppSettings.tsx` | `ep.endpoint_type === 'user' && ep.name === 'anthropic'` | ❌ |

The auto-select logic (which runtime/mode to default to) is also inconsistent across files — some handle only subscription mode, some handle both.

## Approach: Extract `useCodingAgentProviderState` Hook

The right fix is not to patch each call site again. The duplication itself is the bug — the next PR will miss another file. Instead, centralise the provider-awareness and auto-select logic into a single hook that `CodingAgentForm` calls internally, removing `hasAnthropicProvider` and `hasClaudeSubscription` from its props entirely.

### Why a hook, not inline in the component

`CodingAgentForm` is already a `forwardRef` component. Adding fetching + auto-select logic + a guard ref inline makes it harder to read and impossible to test the logic in isolation. A companion hook keeps the component focused on rendering.

### Why the hook lives inside `CodingAgentForm`, not in parents

Research finding: all five parents follow an identical pattern — they manage `codeAgentRuntime`, `claudeCodeMode`, `selectedProvider`, `selectedModel` as separate `useState` variables, assemble them into a `value` object, and pass `value`/`onChange` as controlled props. They keep this state because they need it for submit-button validation and conditional rendering. They do **not** read state directly for submission — all five call `codingAgentFormRef.current?.handleCreateAgent()`.

This means `value`/`onChange` stay as controlled props (parents keep their state). The hook only needs to own the two things parents currently duplicate: computing `hasAnthropicProvider`/`hasClaudeSubscription`, and auto-selecting the correct runtime/mode via `onChange` when providers first load.

### State management: calling `onChange` from a `useEffect`

The auto-select effect calls `onChange` (a parent callback) from inside a `useEffect`. This is valid but needs one guard: a `useRef` flag (`hasAutoSelected`) so it fires only once after providers first load, and never overrides an explicit user selection. Per the project's React rules, `onChange` does not go in the dependency array — only the boolean primitives `hasAnthropicProvider` and `hasClaudeSubscription` do.

React Query naturally deduplicates the fetching: `useListProviders({ loadModels: false })` and `useClaudeSubscriptions()` use stable query keys, so calling them inside `CodingAgentForm` costs no extra network requests regardless of how many instances are on screen.

## Hook Design

```typescript
// frontend/src/components/agent/useCodingAgentProviderState.ts

export function useCodingAgentProviderState(
  value: CodingAgentFormValue,
  onChange: (value: CodingAgentFormValue) => void,
) {
  const { data: providerEndpoints } = useListProviders({ loadModels: false })
  const { data: claudeSubscriptions } = useClaudeSubscriptions()

  const hasAnthropicProvider = useMemo(() => {
    if (!providerEndpoints) return false
    return providerEndpoints.some(p => p.name === 'anthropic')
  }, [providerEndpoints])

  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0

  // Auto-select correct runtime/mode once, when provider data first resolves.
  // Never overrides an explicit user selection.
  const hasAutoSelected = useRef(false)
  useEffect(() => {
    if (hasAutoSelected.current) return
    if (!providerEndpoints) return  // not loaded yet
    hasAutoSelected.current = true
    if (hasAnthropicProvider) {
      onChange({ ...value, codeAgentRuntime: 'claude_code', claudeCodeMode: 'api_key' })
    } else if (hasClaudeSubscription) {
      onChange({ ...value, codeAgentRuntime: 'claude_code', claudeCodeMode: 'subscription' })
    }
  }, [hasAnthropicProvider, hasClaudeSubscription, providerEndpoints])

  return { hasAnthropicProvider, hasClaudeSubscription }
}
```

`CodingAgentForm` calls this hook internally and uses the returned booleans to drive its credential UI. `hasAnthropicProvider` and `hasClaudeSubscription` are removed from `CodingAgentFormProps`.

## Files to Change

### 1. `frontend/src/components/agent/useCodingAgentProviderState.ts` *(new file)*

Create the hook as above.

### 2. `frontend/src/components/agent/CodingAgentForm.tsx`

- Remove `hasAnthropicProvider` and `hasClaudeSubscription` from `CodingAgentFormProps`
- Call `useCodingAgentProviderState(value, onChange)` at the top of the component
- Use the returned booleans in the credential UI (replacing the props)

### 3. All parent components — remove the now-redundant code

Every parent currently:
- Calls `useListProviders` and `useClaudeSubscriptions`
- Computes `hasAnthropicProvider` and `hasClaudeSubscription`
- Has an auto-select `useEffect`
- Passes `hasAnthropicProvider` and `hasClaudeSubscription` as props

All of this can be deleted. Affected files:
- `frontend/src/pages/Onboarding.tsx`
- `frontend/src/pages/ProjectSettings.tsx`
- `frontend/src/components/project/CreateProjectDialog.tsx`
- `frontend/src/components/project/AgentSelectionModal.tsx`
- `frontend/src/components/tasks/NewSpecTaskForm.tsx`

Note: `Onboarding.tsx` fetches providers org-scoped (`orgId: createdOrg?.id`) for the model list — that fetch stays. Only the `hasAnthropicProvider` derivation and auto-select effect are removed.

### 4. `frontend/src/components/app/AppSettings.tsx`

This file has the same broken `hasAnthropicProvider` pattern but does **not** use `CodingAgentForm` — it has its own inline credential UI. Fix the computation to match the correct pattern:

```typescript
// Before:
const hasAnthropicProvider = providerEndpoints.some(
  ep => ep.endpoint_type === 'user' && ep.name === 'anthropic'
)

// After:
const hasAnthropicProvider = providerEndpoints.some(ep => ep.name === 'anthropic')
```

## Gotchas

- The `hasAutoSelected` ref resets if the component unmounts and remounts (e.g. dialog open/close). This is correct behaviour — reopening a dialog should re-apply the default.
- `Onboarding.tsx` has a more complex `onChange` handler than other parents (it tracks whether the user explicitly switched to subscription mode and resets model selection flags). The hook's `onChange` call will go through this handler correctly since it calls the prop, not a setter directly.
- `AppSettings.tsx` uses `CodingAgentForm` differently — check whether it also needs the auto-select fix or just the `hasAnthropicProvider` computation fix.
