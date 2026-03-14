# Requirements: Fix Onboarding Claude Code Auto-Suggestion with Global Anthropic Provider

## Problem

Two bugs exist in the onboarding flow (and partially in the "Create New Agent" flow) when Anthropic is configured as a **system/global** provider rather than a user-level provider:

1. **Claude Code is not auto-suggested** during onboarding when Anthropic is a global provider
2. **"Anthropic API Key (not configured)"** is shown even though a global Anthropic provider exists

A recent PR fixed these bugs in `AgentSelectionModal`, `CreateProjectDialog`, `NewSpecTaskForm`, and `ProjectSettings` — but `Onboarding.tsx` was missed (duplicate code not updated). The "Create New Agent" modal also still lacks the api_key auto-select logic.

## Root Cause

In `Onboarding.tsx` line 312:
```typescript
const hasAnthropicProvider = connectedProviderIds.has("anthropic");
```
`connectedProviderIds` is built from **user-only** providers (filtered by `ProviderEndpointTypeUser`). Global/system providers are excluded, so `hasAnthropicProvider` is always `false` when Anthropic is only a global provider.

This wrong value is:
- Passed to `CodingAgentForm` → shows "(not configured)" label
- Used in auto-select logic → Claude Code (api_key mode) is never auto-suggested

In `AgentSelectionModal.tsx`, `hasAnthropicProvider` was already fixed to include all providers, but the auto-select logic still only handles subscription mode, not api_key mode.

## User Stories

**US1:** As a new user during onboarding, when a system Anthropic provider exists, I want Claude Code to be auto-suggested as the code agent runtime, so I don't need to manually discover and select it.

**US2:** As a user opening onboarding when a global Anthropic API key is configured, I want Claude Code to be auto-selected with the "Anthropic API Key" credential mode already chosen, so I can proceed without any manual selection.

**US3:** As a user creating a new agent in project settings, when a global Anthropic provider exists, I want Claude Code to be auto-suggested as the runtime (with api_key mode), consistent with the onboarding experience.

## Acceptance Criteria

- [ ] When Anthropic is a **global/system** provider and no user providers exist, onboarding auto-selects Claude Code with `api_key` mode
- [ ] When Anthropic is a **global/system** provider, Claude Code is auto-selected and the "Anthropic API Key (configured)" credential mode is already chosen by default
- [ ] When both Claude subscription and Anthropic global provider exist, Claude Code with `api_key` mode takes priority in auto-selection (consistent with NewSpecTaskForm pattern)
- [ ] Same auto-select behavior applies in the "Create New Agent" modal (`AgentSelectionModal`)
- [ ] Existing behavior is preserved: when only a Claude subscription exists, subscription mode is auto-selected
- [ ] When neither exists, no auto-selection occurs
