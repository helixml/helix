# Requirements: Mid-Session Agent Switching

## User Stories

### 1. Switch agent mid-session
**As a** user working on a coding task,  
**I want to** switch from one agent (e.g., Claude Code) to another (e.g., Qwen Code) without losing my conversation history,  
**So that** I can use the best agent for each phase of my work without starting over.

**Acceptance Criteria:**
- User can trigger an agent switch from the Helix UI while a session is active
- The new agent receives the full conversation context from the previous agent
- Tool call results and agent responses are preserved in the history
- The session ID remains the same (no new session created)
- The switch completes within 30 seconds (excluding container cold-start)

### 2. Continue where the other agent left off
**As a** user who just switched agents,  
**I want** the new agent to understand what was already done and what files were changed,  
**So that** it can pick up the work seamlessly.

**Acceptance Criteria:**
- The new agent can see all prior messages (user prompts + agent responses)
- The new agent has access to the same MCP servers and tools
- The workspace (files, git state) is unchanged by the switch
- The new agent can immediately accept new prompts after the switch

### 3. Preserve session continuity in the UI
**As a** user viewing my session history,  
**I want to** see a clear marker when agents were switched,  
**So that** I know which agent produced which responses.

**Acceptance Criteria:**
- The session timeline shows an "Agent switched" marker
- Each message block indicates which agent produced it
- Session title and metadata remain intact

## Out of Scope (for initial implementation)
- Switching agents while a response is actively streaming
- Automatic agent selection based on task type
- Running multiple agents simultaneously in one session
- Preserving agent-specific internal state (e.g., Claude Code's memory files)
