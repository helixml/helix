# Design: Fix Onboarding Credentials Messaging for System/Global Providers

## Context

The `CodingAgentForm` component controls the Claude Code credentials UI (subscription vs API key radio buttons). It receives a boolean `hasAnthropicProvider` prop that currently only reflects whether the user has a **user-level** Anthropic API key configured. It does not account for system/global Anthropic providers that are available via the Helix proxy.

The Helix proxy (`anthropicAPIProxyHandler` → `getProviderEndpoint`) resolves providers in this order:
1. Project-specific provider
2. Org-level provider
3. Any matching provider in the database (including global)
4. Built-in provider from `ANTHROPIC_API_KEY` env var

This means the "API key" credential mode works fine with global/system providers — the backend handles it correctly. The bug is purely a frontend display issue.

## Codebase Patterns Discovered

- **`hasAnthropicProvider`** is computed identically in 6 files, always checking only `endpoint_type === 'user'` and `name === 'anthropic'`. The relevant files:
  - `Onboarding.tsx` (line 323)
  - `AppSettings.tsx` (line 286)
  - `AgentSelectionModal.tsx` (line 81)
  - `CreateProjectDialog.tsx` (line 115)
  - `NewSpecTaskForm.tsx` (line 189)
  - `ProjectSettings.tsx` (line 527)

- **`CodingAgentForm`** is a shared component used by all the above surfaces. It takes `hasClaudeSubscription` and `hasAnthropicProvider` as separate booleans.

- **`globalProviders`** is already computed in `Onboarding.tsx` (line 307) by filtering for `ProviderEndpointTypeGlobal`. Other surfaces don't compute this.

- **Provider types** from the API: `global`, `user`, `org`, `team` (enum `TypesProviderEndpointType`).

- The `useListProviders` hook returns all visible providers (user + global + org depending on context). The data is already available — it just needs to be checked.

## Approach

### Option A: Broaden `hasAnthropicProvider` to include global providers ✅ (chosen)

Change the `hasAnthropicProvider` computation in each consuming component to also check for global/system Anthropic providers. This is the minimal change that fixes the bug at the source.

### Option B: Add a separate `hasGlobalAnthropicProvider` prop to CodingAgentForm

More explicit but adds prop proliferation. Rejected — the form doesn't need to know *why* Anthropic is available, just *whether* it is.

### Option C: Centralize provider availability into a hook

Create a `useProviderAvailability()` hook that returns `{ hasAnthropicProvider, hasClaudeSubscription, ... }`. Good long-term but over-scoped for this bug fix.

## Design

### 1. Broaden `hasAnthropicProvider` to include global/system providers

In every file that computes `hasAnthropicProvider`, change from:

```ts
// Before: only checks user-level
const hasAnthropicProvider = providerEndpoints.some(
  ep => ep.endpoint_type === 'user' && ep.name === 'anthropic'
)
```

To:

```ts
// After: checks user-level, global, and org-level
const hasAnthropicProvider = providerEndpoints.some(
  ep => ep.name === 'anthropic' && (
    ep.endpoint_type === 'user' ||
    ep.endpoint_type === 'global' ||
    ep.endpoint_type === 'org'
  )
)
```

In `Onboarding.tsx` specifically, the pattern is slightly different (uses `connectedProviderIds` Set), so it needs adjustment:

```ts
// Before
const hasAnthropicProvider = connectedProviderIds.has("anthropic");

// After: also check global providers
const hasAnthropicProvider = connectedProviderIds.has("anthropic") ||
  globalProviders.some(p => p.name === "anthropic");
```

### 2. Add a `hasSystemAnthropicProvider` flag for label differentiation

To show the right label text ("configured" vs "available via Helix"), we need `CodingAgentForm` to know whether the Anthropic availability comes from the user's own key or a system provider. Add one prop:

```ts
// New prop on CodingAgentForm
hasSystemAnthropicProvider?: boolean  // true when Anthropic is available only via global/system provider
```

This controls the label text:
- User has own key → `"Anthropic API Key (configured)"`
- Only system provider → `"Anthropic API Key (available via Helix)"`
- Neither → `"Anthropic API Key (not configured)"` (disabled)

### 3. Add billing note to global providers section in onboarding

In `Onboarding.tsx`, under the "Globally configured providers" heading, add a small caption:

> "Usage of these providers is billed through your Helix account."

This is a single `<Typography>` element. No backend changes needed — billing is already tracked by the proxy when `PROVIDERS_BILLING_ENABLED=true`.

### 4. Fix the warning alert logic in CodingAgentForm

Currently at line 288:
```tsx
{!hasClaudeSubscription && !hasAnthropicProvider && (
  <Alert severity="warning">...</Alert>
)}
```

No change needed here — once `hasAnthropicProvider` correctly reflects global provider availability, this alert will naturally hide when a global Anthropic provider exists.

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/pages/Onboarding.tsx` | Broaden `hasAnthropicProvider`, compute `hasSystemAnthropicProvider`, add billing note under global providers |
| `frontend/src/components/agent/CodingAgentForm.tsx` | Add `hasSystemAnthropicProvider` prop, update label text logic |
| `frontend/src/components/app/AppSettings.tsx` | Broaden `hasAnthropicProvider`, compute and pass `hasSystemAnthropicProvider` |
| `frontend/src/components/project/AgentSelectionModal.tsx` | Same pattern |
| `frontend/src/components/project/CreateProjectDialog.tsx` | Same pattern |
| `frontend/src/components/tasks/NewSpecTaskForm.tsx` | Same pattern |
| `frontend/src/pages/ProjectSettings.tsx` | Same pattern |

## No Backend Changes

The backend already correctly resolves providers through the proxy chain (user → org → global → env). This is a frontend-only fix.

## Risks

- **Low risk**: The change is purely presentational — it doesn't change what providers are actually used at runtime. The backend proxy already works with global providers.
- **Label wording**: "available via Helix" might need product review. The implementation should use a clear but brief phrase.