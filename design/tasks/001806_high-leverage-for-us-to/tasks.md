# Implementation Tasks

## Helix API

- [ ] Add `switch_agent` WebSocket command constant to `ExternalAgentCommand` types (`api/pkg/types/types.go`)
- [ ] Add `POST /api/v1/sessions/{id}/switch-agent` endpoint that validates idle state, updates `ZedAgentName` + `CodeAgentRuntime` in session metadata, creates a system interaction marker, and sends `switch_agent` command via WebSocket (`api/pkg/server/session_handlers.go` or new file)
- [ ] Add `switch_agent` command handling in WebSocket sync — construct and send the command to the connected Zed instance (`api/pkg/server/websocket_external_agent_sync.go`)
- [ ] Add system interaction creation for agent switch events with `Trigger: "agent_switch"` and `DisplayMessage` showing old→new agent

## Zed (External WebSocket Sync)

- [ ] Handle `switch_agent` command in Zed's external WebSocket sync module — parse the command and dispatch to the agent system (`crates/external_websocket_sync/`)
- [ ] Add `switch_agent_for_session()` to `NativeAgent` — updates the thread's profile and model to match the new agent without requiring thread reload (`crates/agent/src/agent.rs`)
- [ ] Verify that MCP tool availability (ContextServerRegistry) is unaffected by profile switch — tools are container-level and should persist

## Helix Frontend

- [ ] Add agent selector UI control in session view (next to model selector) showing available agent runtimes
- [ ] Wire agent selector to call `POST /sessions/{id}/switch-agent` and show switching indicator
- [ ] Render "Agent switched" system interaction as a visual divider/marker in the conversation timeline
- [ ] Display which agent produced each message block (use `Interaction.Trigger` or new metadata field)

## Testing

- [ ] Test agent switch while agent is idle (happy path)
- [ ] Test agent switch is rejected while an interaction is in `waiting` state
- [ ] Test conversation continuity — new agent can read and respond to prior context after switch
- [ ] Test workspace persistence — files and git state are unchanged after switch
