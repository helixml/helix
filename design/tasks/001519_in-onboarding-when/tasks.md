# Implementation Tasks

## Core Fix: Broaden `hasAnthropicProvider` to include global/system providers

- [ ] **`Onboarding.tsx`**: Update `hasAnthropicProvider` (line 323) to also check `globalProviders` for an Anthropic entry — change from `connectedProviderIds.has("anthropic")` to also include `globalProviders.some(p => p.name === "anthropic")`
- [ ] **`Onboarding.tsx`**: Compute `hasSystemAnthropicProvider` — true when a global Anthropic provider exists but the user has no user-level one (`!connectedProviderIds.has("anthropic") && globalProviders.some(p => p.name === "anthropic")`)
- [ ] **`AppSettings.tsx`** (line 286): Broaden `hasAnthropicProvider` check to include `endpoint_type === 'global'` and `endpoint_type === 'org'`
- [ ] **`AgentSelectionModal.tsx`** (line 81): Same broadening of `hasAnthropicProvider`
- [ ] **`CreateProjectDialog.tsx`** (line 115): Same broadening of `hasAnthropicProvider`
- [ ] **`NewSpecTaskForm.tsx`** (line 189): Same broadening of `hasAnthropicProvider`
- [ ] **`ProjectSettings.tsx`** (line 527): Same broadening of `hasAnthropicProvider`
- [ ] For each of the above 5 non-onboarding files, compute `hasSystemAnthropicProvider` using the same pattern (has global/org anthropic but no user-level one)

## CodingAgentForm: Add system provider awareness

- [ ] Add `hasSystemAnthropicProvider?: boolean` prop to `CodingAgentFormProps` interface in `CodingAgentForm.tsx`
- [ ] Update the "Anthropic API Key" radio button label to show `"(available via Helix)"` when `hasSystemAnthropicProvider` is true, `"(configured)"` when `hasAnthropicProvider && !hasSystemAnthropicProvider`, and `"(not configured)"` when neither
- [ ] Pass `hasSystemAnthropicProvider` from all 6 consuming components (`Onboarding.tsx`, `AppSettings.tsx`, `AgentSelectionModal.tsx`, `CreateProjectDialog.tsx`, `NewSpecTaskForm.tsx`, `ProjectSettings.tsx`)

## Billing clarity for global providers

- [ ] In `Onboarding.tsx`, add a caption under the "Globally configured providers" heading (around line 1588) that says "Usage of these providers is billed through your Helix account" — use the same `rgba(255,255,255,0.3)` / `0.72rem` styling as the heading

## Verification

- [ ] Run `cd frontend && yarn build` — confirm no TypeScript/build errors
- [ ] Manual test: with a global Anthropic provider configured and no user-level Anthropic key, confirm the "Anthropic API Key" radio is enabled and shows "(available via Helix)"
- [ ] Manual test: with a user-level Anthropic key configured, confirm it still shows "(configured)"
- [ ] Manual test: with neither, confirm it shows "(not configured)" and is disabled
- [ ] Manual test: confirm the billing note appears under "Globally configured providers" in onboarding
- [ ] Manual test: confirm the warning alert ("Connect a Claude subscription or add an Anthropic API key") does NOT appear when a global Anthropic provider exists