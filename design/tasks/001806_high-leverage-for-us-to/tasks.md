# Implementation Tasks

## Settings-sync-daemon: Pre-configure all agents

- [ ] Modify `generateAgentServerConfig()` in `api/cmd/settings-sync-daemon/main.go` to return configs for ALL supported agents simultaneously (qwen, claude-acp, codex, gemini) instead of just the selected runtime
- [ ] Verify Zed lazily connects to agent_servers (doesn't eagerly spawn all processes at startup) — if not, investigate deferred registration
- [ ] Ensure credentials (API keys, OAuth tokens) are correctly set for all agent configs
- [ ] Test that all agents appear in Zed's agent selector dropdown inside the container

## Helix API: Switch endpoint + WebSocket command

- [ ] Add `ExternalAgentCommandSwitchAgent` constant to `api/pkg/types/types.go`
- [ ] Add `POST /api/v1/sessions/{id}/switch-agent` endpoint: validate idle state, update `ZedAgentName` + `CodeAgentRuntime` in session metadata, create system interaction marker, send `switch_agent` command via WebSocket
- [ ] Add `switch_agent` WebSocket command construction and sending in `api/pkg/server/websocket_external_agent_sync.go`
- [ ] Handle `thread_switched` event from Zed — update `Session.Metadata.ZedThreadID` and context mappings to point to the new thread ID
- [ ] Add system interaction creation for agent switch events with `Trigger: "agent_switch"`

## Zed: Handle switch_agent command

- [ ] Handle `switch_agent` command in external WebSocket sync module — parse command, dispatch to agent system
- [ ] Implement agent switch flow: save current thread → get new agent connection → create new AcpThread → replay old thread messages → report new thread ID back via WebSocket
- [ ] Verify that `replay()` + `handle_thread_events()` correctly populates the new AcpThread with old conversation history
- [ ] Verify that external agent `run_turn()` sends full replayed message history to the new agent process (ACP protocol check)

## Helix Frontend

- [ ] Add agent selector dropdown to session controls showing all available agents with active indicator
- [ ] Wire selector to call `POST /sessions/{id}/switch-agent` and show "Switching..." loading state
- [ ] Render "Agent switched" system interaction as a visual divider in conversation timeline

## Testing

- [ ] Test full switch flow: send prompts with Agent A → switch to Agent B → verify Agent B sees prior history and can continue
- [ ] Test switch is rejected while an interaction is in `waiting` state
- [ ] Test workspace persistence: files and git state unchanged after switch
- [ ] Test MCP tools remain functional after switch
- [ ] Measure resource usage with all agents pre-configured vs single agent
