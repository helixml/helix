# Implementation Tasks

## Settings-sync-daemon: pre-configure all agents

- [x] Modify `generateAgentServerConfig()` in `api/cmd/settings-sync-daemon/main.go` to return configs for all agents (qwen, claude-acp, and future codex/gemini) instead of just the selected runtime
- [ ] Verify Zed lazily spawns agent_servers processes (not all at boot)
- [ ] Ensure credentials are correctly set for all agent configs

## Helix API: switch-agent endpoint + transcript injection

- [x] Add `POST /api/v1/sessions/{id}/switch-agent` endpoint — validate idle state (reject if any interaction is `waiting`), update `ZedAgentName` + `CodeAgentRuntime`, clear `ZedThreadID`, clean up old contextMappings, add old thread to draining set, create system interaction marker
- [x] Add `serializeTranscript()` — convert interactions to markdown transcript (user turns, agent responses with tool calls from ResponseEntries, truncation at 100KB)
- [x] Add `maybePrependTranscript()` — detect post-switch state (ZedThreadID empty + completed interactions exist), prepend transcript to message
- [x] Inject transcript in all 3 message-sending paths: `NotifyExternalAgentOfNewInteraction`, `pickupWaitingInteraction`, `sendChatMessageToExternalAgent`
- [x] Fix `NotifyExternalAgentOfNewInteraction` to include `agent_name` in command data (pre-existing bug)
- [x] Add old thread ID draining set — silently drop late-arriving events from old thread

## Helix Frontend

- [ ] Add agent selector dropdown to session controls showing available agents
- [ ] Wire selector to call switch-agent endpoint with loading state
- [ ] Render "Agent switched" system interaction as visual divider in timeline
