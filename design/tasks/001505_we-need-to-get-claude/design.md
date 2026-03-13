# Design: Claude Code — Test API Key Mode & Conditional Default

## Problem Statement

Claude Code subscription mode has been user-tested and confirmed working. What hasn't been verified is **API key mode**, where Claude Code talks through the Helix Anthropic proxy. In the helix-in-helix setup, the proxy chain is:

```
Claude Code (in container) → ANTHROPIC_BASE_URL (inner Helix /v1/messages) → outer Helix API → Google Vertex Anthropic hosting
```

We need to test this end-to-end and, once confirmed working, make Claude Code the default runtime — but only when the user has an Anthropic provider available.

## Architecture: API Key Mode Proxy Chain

### How it's wired today

1. **UI**: User selects `claude_code` runtime + `api_key` credential type + Anthropic provider + Claude model
2. **API** (`buildCodeAgentConfigFromAssistant` in `zed_config_handlers.go`): Builds `CodeAgentConfig` with `baseURL = helixURL` (no `/v1` suffix — Claude Code SDK appends `/v1/messages` itself)
3. **Settings-sync-daemon** (`generateAgentServerConfig`): Sets `ANTHROPIC_BASE_URL` (Helix proxy URL) in the agent_servers env config for Claude Code
4. **Helix proxy** (`/v1/messages` → `getProviderEndpoint()`): Looks up the `ProviderEndpoint` for the request — tries org-level first, then falls back to the built-in global provider from env vars. Forwards to Anthropic with the resolved API key.

In helix-in-helix, step 4 hits the **inner** Helix API, which has `ANTHROPIC_BASE_URL=http://host.docker.internal:8081` pointing at the **outer** Helix API, which in turn proxies to Google Vertex Anthropic hosting.

### What could go wrong

- The double-proxy chain might mangle headers (e.g., `anthropic-version`, `x-api-key`)
- The `ANTHROPIC_BASE_URL` inside the container might not resolve correctly in the inner network
- Token/auth issues between inner and outer Helix APIs
- Claude Code might not handle non-standard base URLs correctly

## Design Decisions

### Decision 1: Manual E2E test of API key mode

**Approach:** Create a Claude Code agent via the UI with API key mode, start a session, send a real coding task, and verify:
- No permission prompts (bypass layers are working — already confirmed in subscription mode)
- Requests flow through the proxy chain (check inner API logs)
- The agent actually completes work

This is a manual test — if it works, we're done. If something breaks, we fix the specific issue.

### Decision 2: Conditional default — Claude Code only when Anthropic is available

**Rationale:** Claude Code only works with Anthropic models. If the user only has OpenAI or other providers, defaulting to Claude Code would just show them error states. Zed Agent works with any LLM, so it's the safe universal default.

**Approach:** The frontend already has `hasAnthropicProvider` (checks if an Anthropic `ProviderEndpoint` exists) and `hasClaudeSubscription` (checks for active Claude OAuth subscription). The default runtime logic should be:

```
if (hasAnthropicProvider || hasClaudeSubscription) → default to 'claude_code'
else → default to 'zed_agent'
```

There's already partial logic for this in `AgentSelectionModal.tsx` (~line 150-155) that auto-selects `claude_code` when the user has a Claude subscription and no other providers. This needs to be generalized to also cover Anthropic API key providers, and applied consistently across all agent creation flows.

**Files that need the default changed** (currently all hardcoded to `'zed_agent'`):
- `pages/Onboarding.tsx` (~line 258)
- `components/project/CreateProjectDialog.tsx` (~line 196)
- `components/project/AgentSelectionModal.tsx` (~line 91)
- `components/tasks/NewSpecTaskForm.tsx` (~line 175)
- `pages/ProjectSettings.tsx` (~line 514)
- `contexts/apps.tsx` `createAgent` fallback (~line 226)
- `components/app/AppSettings.tsx` (~line 253) — fallback for existing apps

Each of these needs access to the provider/subscription state to compute the default. A shared helper (e.g., `getDefaultCodeAgentRuntime(hasAnthropicProvider, hasClaudeSubscription)`) in `contexts/apps.tsx` would keep it DRY.

### Decision 3: No changes to the Zed fork or backend

The bypass layers and proxy chain are all backend/container config. No Zed Rust changes are needed. The only code changes are in the frontend to adjust the default runtime selection.

## Codebase Patterns

- **Provider detection**: `hasAnthropicProvider` is already computed in `AgentSelectionModal.tsx` by checking if any `ProviderEndpoint` has name matching `anthropic`. This pattern should be reused.
- **`CodingAgentForm.tsx`** receives `value.codeAgentRuntime` as a prop — the default is set by the parent, not the form. So changes are in the parent components.
- **`AppSettings.tsx`** uses `app.code_agent_runtime || 'zed_agent'` for existing agents — this should use the conditional default so that existing agents with no runtime set get the right default for their environment.