# Requirements: Mid-Session Agent Switching via Fork-and-Pause

## User Stories

### 1. Fork to a different agent mid-session
**As a** user working in a Helix external agent session,
**I want to** switch from one code agent (e.g., Claude Code) to another (e.g., Qwen Code) without losing conversation context,
**So that** I can use the best agent for each phase of my work.

**Acceptance Criteria:**
- User can trigger a fork from the chat-panel agent dropdown while a session is active.
- Available agents: Claude Code, Qwen Code, Codex, Gemini, Zed built-in.
- Switching **forks** the current session into a new session with the target agent (does not mutate the source). The source session is **paused** as a frozen checkpoint.
- The new (forked) session is seeded with the parent's full event stream via first-turn transcript injection, so the new agent can immediately continue with full context.
- Workspace (files, git state) is preserved on the new session.
- The Helix UI navigates to the new session id after a successful fork; the parent's id is preserved and accessible.
- MCP tools remain available on the new session.

### 2. All agents pre-configured in container
**As a** platform operator,
**I want** all supported agents configured in every container upfront,
**So that** forking is instant.

**Acceptance Criteria:**
- Zed `settings.json` includes `agent_servers` for all supported agents.
- All credentials available for all agents.
- Idle agents don't consume excessive resources (lazy ACP spawn).

### 3. Graceful format degradation on fork
**As a** user forking between different agent types,
**I want** the conversation history to transfer even if some agent-specific features don't translate perfectly,
**So that** I get useful context even across very different agents.

**Acceptance Criteria:**
- Core messages (user prompts, agent text, tool calls/results) are serialized into a readable transcript.
- Agent-specific features (sub-agent runs, thinking blocks) degrade to readable text/markdown.
- The new agent can parse the transcript and continue the conversation coherently.

### 4. Visual indicator of lineage
**As a** user viewing my session,
**I want** clear markers showing fork lineage and paused state,
**So that** I can navigate the conversation tree.

**Acceptance Criteria:**
- Child sessions show a "Forked from <parent>" badge with a clickable parent link.
- Paused sessions show a "Paused — forked to <child>" banner with a clickable child link.
- The chat input is disabled on paused sessions.
- The conversation timeline shows a `fork_seed` divider on child sessions, with an expandable disclosure containing the raw transcript content.

### 5. Pause enforcement
**As a** user,
**I want** sending messages to a paused session to fail clearly,
**So that** I don't accidentally interact with a frozen checkpoint.

**Acceptance Criteria:**
- `POST /sessions/{id}/messages` on a paused session returns HTTP 409 with a `"session is paused (reason: X)"` body.
- All other ingress paths (chat-message routing, queue pickup, queued prompt sending, websocket notify) are similarly gated.

## Out of Scope (v1)
- Switching while a response is actively streaming (forking is always safe; the parent's in-flight interaction is allowed to complete on its own thread).
- Automatic agent selection based on task type.
- Running multiple agents simultaneously on the same thread.
- Transferring agent-specific internal state (Claude Code memory files, Qwen state).
- Manual `/pause` and `/unpause` endpoints (stretch — fork is the only paused-state-producing operation in v1).
- A `/duplicate` endpoint (fork to the same agent).
- Container reaping policy for paused sessions (deferred to v2; v1 lets the normal session-idle reaper take parent containers).
- Shared-container optimization between parent and child.
