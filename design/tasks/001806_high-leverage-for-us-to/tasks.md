# Implementation Tasks

## ACP protocol extension

- [ ] Define `ImportSessionRequest` / `ImportSessionResponse` message types in the ACP spec — request contains `cwd`, `mcp_servers`, and `messages: Vec<ImportedMessage>` with chronologically ordered conversation history
- [ ] Add `import_session` capability flag to agent capabilities
- [ ] Add `import_session()` to `AgentConnection` trait in `crates/acp_thread/src/connection.rs`
- [ ] Implement `import_session()` in `AcpConnection` (`crates/agent_servers/src/acp.rs`) — sends the request, creates AcpThread, receives SessionUpdate replay stream

## Agent runtimes: implement import_session

- [ ] Implement `import_session` handler in Claude Code (claude-acp) — create session, seed internal message buffer with imported messages
- [ ] Implement `import_session` handler in Qwen Code — same pattern
- [ ] Define message format mapping: Zed `Thread.messages` → `ImportedMessage` (user text, agent text, tool calls/results as content blocks; sub-agent runs and thinking flattened to markdown)

## Settings-sync-daemon: pre-configure all agents

- [ ] Modify `generateAgentServerConfig()` in `api/cmd/settings-sync-daemon/main.go` to return configs for all agents (qwen, claude-acp, and future codex/gemini) instead of just the selected runtime
- [ ] Verify Zed lazily spawns agent_servers processes (not all at boot)
- [ ] Ensure credentials are correctly set for all agent configs

## Zed: handle switch_agent command

- [ ] Handle `switch_agent` WebSocket command in external sync module — parse command, dispatch to agent system
- [ ] Implement switch flow: save current thread → extract messages → get new agent connection → call `import_session()` with extracted messages → report new thread ID back via WebSocket
- [ ] Build message extraction: convert `Thread.messages` to `Vec<ImportedMessage>`, gracefully degrading agent-specific features to text

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
