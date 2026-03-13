# Implementation Tasks

## Test Claude Code with API Key Mode (Anthropic Proxy)

- [ ] Create a Claude Code agent via the UI: runtime=`claude_code`, credential_type=`api_key`, Anthropic provider, Claude model (e.g. `claude-sonnet-4-20250514`)
- [ ] Start a session and verify Claude Code launches without errors
- [ ] Verify no permission prompts appear — the agent should run autonomously
- [ ] Send a real coding task (e.g. "Create a file called hello.py that prints hello world, then run it") and confirm the agent completes it
- [ ] Check inner API logs to confirm requests flow through the Helix Anthropic proxy at `/v1/messages`
- [ ] In helix-in-helix setup, verify the full proxy chain works: container → inner Helix `/v1/messages` → outer Helix API → Google Vertex Anthropic hosting
- [ ] If anything breaks, diagnose and fix the specific issue

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