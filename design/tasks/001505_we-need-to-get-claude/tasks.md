# Implementation Tasks

## Test Claude Code with API Key Mode (Anthropic Proxy)

- [x] Create a Claude Code agent via the UI: runtime=`claude_code`, credential_type=`api_key`, Anthropic provider, Claude model (e.g. `claude-sonnet-4-20250514`)
  - **BLOCKER FOUND & FIXED:** The UI showed "Anthropic API Key (not configured)" even though a global Anthropic provider exists (via `ANTHROPIC_API_KEY` env var). The `hasAnthropicProvider` check in the frontend only matched `endpoint_type === 'user'`, ignoring `global` providers. Fixed by removing the endpoint_type filter — now checks `name === 'anthropic'` regardless of type.
  - Fixed in 6 files: `CreateProjectDialog.tsx`, `AgentSelectionModal.tsx`, `NewSpecTaskForm.tsx`, `ProjectSettings.tsx`, `Onboarding.tsx`, `AppSettings.tsx`
  - Screenshot: `screenshots/04-anthropic-api-key-configured.png`
- [~] After fix: create agent, start session, verify Claude Code launches without errors
  - Agent created with `claude_code` runtime + `api_key` mode + `claude-sonnet-4-20250514` model ✓
  - Project "Claude Code Test" created ✓
  - Task started, session provisioning ("Starting Desktop...") — waiting for container to come up
- [ ] Verify no permission prompts appear — the agent should run autonomously
- [ ] Send a real coding task (e.g. "Create a file called hello.py that prints hello world, then run it") and confirm the agent completes it
- [ ] Check inner API logs to confirm requests flow through the Helix Anthropic proxy at `/v1/messages`
- [ ] In helix-in-helix setup, verify the full proxy chain works: container → inner Helix `/v1/messages` → outer Helix API → Google Vertex Anthropic hosting
- [ ] If anything breaks, diagnose and fix the specific issue

## Fix: `hasAnthropicProvider` ignores global providers

- [x] Update `hasAnthropicProvider` in all files to check for `name === 'anthropic'` regardless of `endpoint_type`:
  - `components/project/CreateProjectDialog.tsx` ✓
  - `components/project/AgentSelectionModal.tsx` ✓
  - `components/tasks/NewSpecTaskForm.tsx` ✓
  - `pages/ProjectSettings.tsx` ✓
  - `pages/Onboarding.tsx` ✓
  - `components/app/AppSettings.tsx` ✓
- [x] Verify in browser: with only a global Anthropic provider, "Anthropic API Key" radio shows "(configured)" and is selectable ✓
- [x] Verify creating a Claude Code agent with API key mode now works ✓

## Make Claude Code the Default Runtime (Conditional)

Only proceed here once API key mode is confirmed working.

- [ ] Add a shared helper in `frontend/src/contexts/apps.tsx`, e.g. `getDefaultCodeAgentRuntime(hasAnthropicProvider, hasClaudeSubscription)` that returns `'claude_code'` if either is true, otherwise `'zed_agent'`
- [ ] Use the helper to set the initial `useState` default in all agent creation flows:
  - `pages/Onboarding.tsx` (~line 258)
  - `components/project/CreateProjectDialog.tsx` (~line 196)
  - `components/project/AgentSelectionModal.tsx` (~line 91)
  - `components/tasks/NewSpecTaskForm.tsx` (~line 175)
  - `pages/ProjectSettings.tsx` (~line 514)
- [ ] Update the `createAgent` fallback in `contexts/apps.tsx` (~line 226): `params.codeAgentRuntime || 'zed_agent'` → use the helper or keep `'zed_agent'` as the safe server-side fallback
- [ ] Update `components/app/AppSettings.tsx` (~line 253) fallback for existing apps with no runtime set
- [ ] Remove or generalize the existing partial auto-select logic in `AgentSelectionModal.tsx` (~line 150-155) that only covers Claude subscription — the new helper covers both subscription and API key provider cases
- [ ] Verify in browser: user WITH Anthropic provider → Claude Code pre-selected; user WITHOUT → Zed Agent pre-selected
- [ ] Verify existing agents with saved `code_agent_runtime` are not affected (their value takes precedence)