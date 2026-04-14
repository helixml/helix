# Implementation Tasks

## Zed: conversation transcript serialization

- [ ] Build transcript serializer: convert `Thread.messages` (Vec<Message>) to a readable markdown transcript — user turns, agent turns, tool calls with results, sub-agent runs as summaries
- [ ] Handle truncation for long conversations — if transcript exceeds a size threshold, summarize or trim older turns to fit within agent context windows
- [ ] Test transcript quality: verify Claude Code, Qwen Code, and Zed built-in agent can parse the transcript and continue coherently

## Zed: handle switch_agent command

- [ ] Handle `switch_agent` WebSocket command in external sync module — parse command, dispatch to agent system
- [ ] Implement switch flow: save current thread → serialize transcript from thread messages → get new agent connection → call `new_session()` → populate new AcpThread with old messages for UI display → on first user turn, prepend transcript to `PromptRequest` content
- [ ] Send `thread_switched` event back via WebSocket with old and new thread IDs

## Settings-sync-daemon: pre-configure all agents

- [~] Modify `generateAgentServerConfig()` in `api/cmd/settings-sync-daemon/main.go` to return configs for all agents (qwen, claude-acp, and future codex/gemini) instead of just the selected runtime
- [ ] Verify Zed lazily spawns agent_servers processes (not all at boot)
- [ ] Ensure credentials are correctly set for all agent configs

## Helix API + thread ID mapping (critical)

- [ ] Add `POST /api/v1/sessions/{id}/switch-agent` endpoint — validate idle state (reject if any interaction is `waiting`), update `ZedAgentName` + `CodeAgentRuntime`, create system interaction marker, send `switch_agent` WebSocket command. Do NOT update `ZedThreadID` yet.
- [ ] Implement two-phase thread ID swap: on receiving `thread_switched` from Zed, atomically update `Session.Metadata.ZedThreadID`, swap `contextMappings[old] → contextMappings[new]`, and remove old mapping
- [ ] Add old thread ID to a short-lived draining set — silently drop any late-arriving events from the old thread instead of routing them
- [ ] Handle switch failure: if Zed doesn't confirm `thread_switched` within timeout, roll back `ZedAgentName` + `CodeAgentRuntime` to previous values
- [ ] Ensure `requestToSessionMapping` entries for in-flight requests are cleaned up on switch

## Helix Frontend

- [ ] Add agent selector dropdown to session controls showing available agents
- [ ] Wire selector to call switch-agent endpoint with loading state
- [ ] Render "Agent switched" system interaction as visual divider in timeline
