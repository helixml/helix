# Requirements: Fix Onboarding Credentials Messaging for System/Global Providers

## Problem

When a user goes through onboarding and:
1. There is a **system/global Anthropic provider** configured by the admin
2. The user has connected their own **Claude subscription** (OAuth)
3. The user has **not** added a user-level Anthropic API key

The onboarding UI incorrectly shows "Anthropic API Key (not configured)" and disables that option in the Claude Code credentials section. In reality, the API key option **is** available because the Helix proxy can route requests through the system/global Anthropic provider — the user doesn't need their own API key.

Additionally, there is no indication anywhere that system/global providers are **Helix-provided** and that usage is billed through Helix, not the user's own API key.

## User Stories

### US-1: Correct API Key availability status
**As a** new user going through onboarding,
**When** a system/global Anthropic provider is available,
**I want** the "Anthropic API Key" credential option to be shown as available (not disabled),
**So that** I can choose to use Claude Code via the Helix-proxied API key route.

### US-2: Clear billing attribution for system providers
**As a** new user,
**When** I see system/global providers in the onboarding flow,
**I want** to understand that these providers are platform-provided and token usage is billed through Helix,
**So that** I can make an informed choice between using my own Claude subscription vs. Helix-provided tokens.

### US-3: Consistent behavior across all agent creation surfaces
**As a** user creating an agent from any surface (onboarding, project settings, new spec task, agent selection modal, create project dialog),
**When** a system/global Anthropic provider exists,
**I want** the API key credential option to reflect that it's available,
**So that** the behavior is consistent regardless of where I create the agent.

## Acceptance Criteria

- [ ] When a global/system Anthropic provider exists, the "Anthropic API Key" radio button in `CodingAgentForm` is **enabled** (not disabled), even if the user has no user-level Anthropic API key.
- [ ] The label for the API key option reflects availability from a system provider (e.g., "Anthropic API Key (available via Helix)" or similar) when no user key exists but a global provider does.
- [ ] The "Globally configured providers" section in onboarding's provider step includes a note clarifying that usage of these providers is billed through Helix.
- [ ] The warning alert ("Connect a Claude subscription or add an Anthropic API key in Providers") is not shown when a global Anthropic provider is available.
- [ ] All surfaces that compute `hasAnthropicProvider` are updated: `Onboarding.tsx`, `AppSettings.tsx`, `AgentSelectionModal.tsx`, `CreateProjectDialog.tsx`, `NewSpecTaskForm.tsx`, `ProjectSettings.tsx`.
- [ ] `CodingAgentForm` accepts a new prop (or enhanced prop) that distinguishes between user-level and system-level Anthropic availability.
- [ ] Frontend builds without errors (`yarn build`).