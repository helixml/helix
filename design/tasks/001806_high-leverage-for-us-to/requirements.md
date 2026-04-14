# Requirements: Mid-Session Agent Switching via ACP

## User Stories

### 1. Switch agent mid-session
**As a** user working in a Helix external agent session,
**I want to** switch from one code agent (e.g., Claude Code) to another (e.g., Qwen Code) without losing conversation context,
**So that** I can use the best agent for each phase of my work.

**Acceptance Criteria:**
- User can trigger an agent switch from the Helix UI while a session is active
- Available agents: Claude Code, Qwen Code, Codex, Gemini, Zed built-in
- The new agent receives conversation history via first-turn transcript injection (no agent forks needed)
- The new agent can immediately continue with full context
- Workspace (files, git state) is unchanged
- Helix session ID remains the same (but Zed thread ID changes — see design doc for mapping risks)
- MCP tools remain available

### 2. All agents pre-configured in container
**As a** platform operator,
**I want** all supported agents configured in every container upfront,
**So that** switching is instant.

**Acceptance Criteria:**
- Zed settings.json includes agent_servers for all supported agents
- All credentials available for all agents
- Idle agents don't consume excessive resources

### 3. Graceful format degradation on switch
**As a** user switching between different agent types,
**I want** the conversation history to transfer even if some agent-specific features don't translate perfectly,
**So that** I get useful context even across very different agents.

**Acceptance Criteria:**
- Core messages (user prompts, agent text, tool calls/results) are serialized into a readable transcript
- Agent-specific features (sub-agent runs, thinking blocks) degrade to readable text/markdown
- The new agent can parse the transcript and continue the conversation coherently

### 4. Visual indicator of agent switch
**As a** user viewing my session,
**I want** a clear marker showing when agents were switched,
**So that** I can see which agent produced which responses.

**Acceptance Criteria:**
- Conversation timeline shows an "Agent switched" divider
- Each message block indicates which agent produced it

## Out of Scope (v1)
- Switching while a response is actively streaming
- Automatic agent selection based on task type
- Running multiple agents simultaneously on the same thread
- Transferring agent-specific internal state (Claude Code memory files, Qwen state)
