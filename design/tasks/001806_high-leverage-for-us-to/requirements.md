# Requirements: Mid-Session Agent Switching

## User Stories

### 1. Switch agent mid-session
**As a** user working on a coding task in a Helix external agent session (running Zed),
**I want to** switch from one code agent (e.g., Claude Code) to another (e.g., Qwen Code) without losing my conversation history or workspace state,
**So that** I can use the best agent for each phase of my work without starting a new session.

**Acceptance Criteria:**
- User can trigger an agent switch from the Helix UI while a session is active
- Available agents: Claude Code, Qwen Code, Codex, Gemini, Zed built-in
- The new agent receives the full conversation history from the prior agent
- The workspace (files, git state, running processes) is unchanged
- The Helix session ID remains the same
- MCP tools (chrome-devtools, helix-desktop, helix-session, etc.) remain available
- The switch completes without restarting the container

### 2. All agents pre-configured in container
**As a** platform operator,
**I want** all supported code agents to be configured in every container upfront,
**So that** switching between agents is instant (no waiting for agent process startup or config changes).

**Acceptance Criteria:**
- The container's Zed settings.json includes agent_servers entries for all supported agents
- All necessary credentials (API keys) are available for all agents
- Idle agents don't consume excessive resources

### 3. Conversation continuity after switch
**As a** user who just switched agents,
**I want** the new agent to see the full conversation history (prompts, responses, tool calls, file changes),
**So that** it can continue the work without me re-explaining context.

**Acceptance Criteria:**
- The new agent's first response demonstrates awareness of prior conversation
- Tool call results (file edits, terminal output, etc.) are visible in history
- The user can immediately send new prompts to the new agent

### 4. Visual indicator of agent switch
**As a** user viewing my session,
**I want** a clear marker showing when agents were switched and which agent produced each response,
**So that** I can attribute work to the correct agent.

**Acceptance Criteria:**
- Conversation timeline shows an "Agent switched" divider
- Each message block indicates which agent produced it

## Out of Scope (v1)
- Switching while a response is actively streaming
- Automatic agent selection based on task type
- Running multiple agents simultaneously on the same thread
- Transferring agent-specific internal state (Claude Code memory, Qwen state)
- Hot-adding new agent types without container restart
