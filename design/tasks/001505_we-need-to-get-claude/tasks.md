# Implementation Tasks

## Audit & Fix Current Bypass Configuration

- [ ] Verify all 4 permission bypass layers are correctly configured on current `main`:
  - Layer 1: `~/.claude/settings.json` has `defaultMode: bypassPermissions` + `skipDangerousModePermissionPrompt: true` (written by `helix-workspace-setup.sh`)
  - Layer 2: Zed `settings.json` has `agent.tool_permissions.default = "allow"` (written by settings-sync-daemon `syncFromHelix`)
  - Layer 3: `agent_servers.claude.default_mode = "bypassPermissions"` (written by settings-sync-daemon `generateAgentServerConfig`) — NOT `"default"`, that was the PR #1778 regression
  - Layer 4: `IS_SANDBOX=1` env var set in `devcontainer.go` `buildEnv`
- [ ] Start a Claude Code session (subscription mode) and confirm no permission prompts appear — run a task that does file creation + bash execution
- [ ] Start a Claude Code session (API key mode with an Anthropic API key) and confirm the same

## API Key Mode End-to-End Verification

- [ ] Ensure an Anthropic inference provider (`ProviderEndpoint`) is configured — either a global one via `ANTHROPIC_API_KEY` env var on the server, or a user/org-level one created in the UI
- [ ] Create a Claude Code agent via the UI with: runtime=`claude_code`, credential_type=`api_key`, provider=`anthropic`, model=`claude-sonnet-4-20250514`
- [ ] Verify `buildCodeAgentConfigFromAssistant` returns correct config: `baseURL = helixURL` (no `/v1` suffix), `apiType = "anthropic"`, `agentName = "claude"`
- [ ] Verify settings-sync-daemon sets `ANTHROPIC_BASE_URL` (Helix proxy URL) in the agent_servers env — the proxy resolves the API key server-side from the `ProviderEndpoint`, not from the container env
- [ ] Verify requests flow through Helix proxy at `/v1/messages` → `getProviderEndpoint()` resolves the correct provider (org → user → global fallback) — check API container logs for proxy activity
- [ ] Confirm the agent completes a multi-step coding task (create file, edit file, run bash command) without hanging

## Add Startup Validation in Settings-Sync-Daemon

- [ ] Add a `validateBypassConfig()` function in `api/cmd/settings-sync-daemon/main.go` that checks after writing settings:
  - `agent.tool_permissions.default` == `"allow"` in Zed settings
  - `agent_servers.claude.default_mode` == `"bypassPermissions"` (when claude_code runtime is active)
  - Logs `WARN` with specific details if any check fails
- [ ] Call `validateBypassConfig()` after every `syncFromHelix` cycle
- [ ] Add unit test for `validateBypassConfig` covering correct config, missing field, and wrong field name (`"default"` vs `"default_mode"`)

## Document the 4-Layer Bypass Stack

- [ ] Add a `// PERMISSION BYPASS STACK` comment block in `generateAgentServerConfig()` in `api/cmd/settings-sync-daemon/main.go` listing all 4 layers, what sets each, and what breaks if it's wrong
- [ ] Add a "Claude Code Permission Bypass" section to `CLAUDE.md` with the layer table from requirements.md so future developers know the full stack
- [ ] Add inline comment on the `default_mode` field: `// IMPORTANT: must be "default_mode" not "default" — PR #1778 regression`

## Add E2E Smoke Test

- [ ] Create a test script (e.g. `tests/claude-code-smoke-test.sh`) that:
  - Uses `spectask start` with a Claude Code agent (API key mode, requires an Anthropic inference provider to be configured)
  - Waits for session ready
  - Uses `spectask send` to request: "Create /tmp/smoke-test.txt with 'hello', then cat it"
  - Uses `spectask stream` with a timeout (e.g. 120s)
  - Checks that the task completes (output contains "hello") rather than hanging on a permission prompt
- [ ] Document how to run the test manually (requires an Anthropic inference provider — global via `ANTHROPIC_API_KEY` env var or user-level — and a running Helix stack)
- [ ] Consider adding to CI pipeline (gated on ANTHROPIC_API_KEY secret availability)

## Harden Against Future Regressions

- [ ] Consider pinning `@anthropic-ai/claude-code` to a specific version in `Dockerfile.ubuntu-helix` and `Dockerfile.sway-helix` instead of `@latest`, to avoid surprise config format changes
- [ ] Add a Go unit test in `api/cmd/settings-sync-daemon/` that verifies `generateAgentServerConfig()` output for `claude_code` runtime contains `"default_mode"` key (not `"default"`)
- [ ] Add a Go unit test verifying the agent_servers config structure matches what the ACP expects: `{ "claude": { "default_mode": "bypassPermissions", "env": { ... } } }`

## Make Claude Code the Default Runtime

- [ ] Add a `DEFAULT_CODE_AGENT_RUNTIME` constant set to `'claude_code'` in `frontend/src/contexts/apps.tsx` next to the existing `CodeAgentRuntime` type
- [ ] Replace all 6 hardcoded `'zed_agent'` defaults in `useState` calls with the new constant:
  - `pages/Onboarding.tsx` (~line 258)
  - `components/project/CreateProjectDialog.tsx` (~line 196)
  - `components/project/AgentSelectionModal.tsx` (~line 91)
  - `components/tasks/NewSpecTaskForm.tsx` (~line 175)
  - `pages/ProjectSettings.tsx` (~line 514)
  - `components/app/AppSettings.tsx` (~line 253) — use the constant as the fallback for existing apps with no runtime set
- [ ] Update the `createAgent` fallback in `contexts/apps.tsx` (~line 226): change `params.codeAgentRuntime || 'zed_agent'` to use the new constant
- [ ] Remove the now-redundant auto-select logic in `AgentSelectionModal.tsx` (~line 150-155) that conditionally sets `claude_code` when the user has a Claude subscription and no other providers — the default handles this now
- [ ] Verify in the browser: open onboarding, create project dialog, agent selection modal, new spec task form — all should show "Claude Code" as the pre-selected runtime with the credentials sub-section visible immediately
- [ ] Verify existing agents with `code_agent_runtime: 'zed_agent'` still load correctly in `AppSettings.tsx` (the existing value from the app takes precedence over the default)
