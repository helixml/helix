# Requirements: Mid-Session Agent Switching via ACP

> **STATUS: SUPERSEDED by task [002081](../002081_kickoff-mid-session/)** (2026-06-09). The fork-and-pause redesign documented here was never built on this task — the predecessor's in-place mutation implementation never landed on the helix branch either. The continuation lives in 002081. This directory is preserved as a record of the architectural pivot.

## User Stories

### 1. Switch agent mid-session (fork-and-pause)
**As a** user working in a Helix external agent session,
**I want to** switch from one code agent (e.g., Claude Code) to another (e.g., Qwen Code) without losing conversation context,
**So that** I can use the best agent for each phase of my work.

**Acceptance Criteria:**
- User can trigger an agent switch from the Helix UI while a session is active
- Available agents: Claude Code, Qwen Code, Codex, Gemini, Zed built-in
- Switching **forks** the current session into a new session with the target agent (does not mutate the source). The source session is **paused** as a frozen checkpoint.
- The new (forked) session is seeded with the parent's full event stream via first-turn transcript injection, so the new agent can immediately continue with full context.
- Workspace (files, git state) is preserved on the new session
- The Helix UI navigates to the new session id after a successful fork; the parent's id is preserved and accessible
- MCP tools remain available on the new session

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
