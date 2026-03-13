# Requirements: Claude Code — Test API Key Mode & Conditional Default

## Context

Claude Code is a code agent runtime in Helix desktop sessions, running inside Zed via ACP. There are two credential modes:

1. **API key mode**: An Anthropic API key is configured via an inference provider (`ProviderEndpoint` — global, user, or org-level). Requests route through Helix's Anthropic proxy.
2. **Subscription mode**: User connects their Claude subscription via OAuth, Claude Code talks directly to Anthropic.

Subscription mode has been user-tested and works fine (no permission prompts). **API key mode has not been verified end-to-end**, especially in the helix-in-helix setup where the proxy chain is: inner Helix → outer Helix API → Google Vertex Anthropic hosting.

## User Stories

### US-1: Claude Code works end-to-end with API key mode (Anthropic proxy)
As a user with an Anthropic inference provider configured, I want Claude Code to work autonomously through the Helix proxy chain without permission prompts or errors.

**Acceptance Criteria:**
- Create a Claude Code agent with: runtime=`claude_code`, credential_type=`api_key`, Anthropic provider, Claude model
- Start a session — Claude Code launches without errors
- No permission prompts appear (all 4 bypass layers are working)
- The agent completes a real coding task (file creation, editing, bash execution)
- Requests flow through the proxy chain correctly: container → inner Helix `/v1/messages` → outer Helix API → Anthropic (Vertex)
- Verify via API container logs that the proxy is receiving and forwarding requests

### US-2: Claude Code is the default runtime when the user has an Anthropic provider
As a new user creating an agent, I want Claude Code to be pre-selected when I have an Anthropic provider available, because it's the best coding agent experience.

**Acceptance Criteria:**
- When the user has an Anthropic inference provider (global, user, or org-level), the runtime dropdown defaults to `claude_code`
- When the user does NOT have an Anthropic provider, the runtime defaults to `zed_agent` (which works with any OpenAI-compatible or Anthropic LLM)
- This applies to all agent creation flows (onboarding, create project, agent selection modal, new spec task, project settings)
- Existing agents are not affected — their saved `code_agent_runtime` is respected

## Known Bypass Layers (Reference)

Subscription mode confirmed working. For API key mode, these same 4 layers must all be correct:

| Layer | Where | What |
|-------|-------|------|
| 1. Claude Code settings.json | `~/.claude/settings.json` (written by `helix-workspace-setup.sh`) | `defaultMode: bypassPermissions` |
| 2. Zed tool_permissions | Zed `settings.json` (written by settings-sync-daemon) | `tool_permissions.default = "allow"` |
| 3. ACP default_mode | agent_servers config (settings-sync-daemon) | `default_mode: "bypassPermissions"` |
| 4. IS_SANDBOX env var | Container env (`devcontainer.go`) | `IS_SANDBOX=1` |