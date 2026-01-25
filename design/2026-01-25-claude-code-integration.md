# Claude Code Integration Design

**Date:** 2026-01-25
**Author:** Claude + Luke
**Status:** Draft

## Executive Summary

This document explores options for integrating Claude Code into Helix as a third code agent type (alongside Zed + Qwen Code and VS Code + Roo Code). The goal is to provide users with a native Claude Code experience while capturing all interactions in Helix's session database for persistence across context compaction cycles.

## Requirements

### Functional Requirements

1. **Native Claude Code Experience**: Users should have access to the full Claude Code feature set
2. **Web Terminal Access**: Stream Claude Code terminal to browser via xterm.js
3. **SSH Access**: Allow users to SSH directly into their Claude Code session
4. **Session Persistence**: Capture all interactions in Helix sessions/interactions tables
5. **Compaction Survival**: Preserve full history even when Claude compacts context
6. **Real-time Streaming**: Stream Claude Code output to Helix's WebSocket sync protocol
7. **BYOK Support**: Allow users to bring their own Claude subscription or API key
8. **Auto-upgrade**: Claude Code CLI should auto-upgrade to get new features

### Non-Functional Requirements

1. **Stability**: Integration should be resilient to Claude Code CLI changes
2. **Minimal Latency**: Terminal streaming should feel responsive
3. **Token Metrics**: Track token usage through Helix's proxy when possible

## Architecture Options

### Option A: Terminal Streaming (Recommended)

**Architecture:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Helix Desktop Container (helix-ubuntu)                          │
│ ┌─────────────┐   ┌──────────────┐   ┌─────────────────────────┐│
│ │   Claude    │──▶│    tmux      │──▶│  helix-claude-bridge    ││
│ │   Code CLI  │   │  (session)   │   │  (Go process)           ││
│ └─────────────┘   └──────────────┘   │  - PTY management       ││
│                                       │  - JSONL watcher        ││
│                                       │  - WebSocket relay      ││
│                                       │  - SSH server           ││
│                                       └──────────┬──────────────┘│
└──────────────────────────────────────────────────┼──────────────┘
                                                   │
                    ┌──────────────────────────────┼──────────────┐
                    │                              ▼              │
                    │  ┌─────────────────────────────────────────┐│
                    │  │           Helix API                     ││
                    │  │  - /api/v1/external-agents/sync (WS)    ││
                    │  │  - /api/v1/sessions/:id/terminal (WS)   ││
                    │  │  - Sessions/Interactions tables         ││
                    │  └─────────────────────────────────────────┘│
                    │                              ▲              │
                    │  ┌───────────────────────────┼─────────────┐│
                    │  │          Frontend                       ││
                    │  │  - xterm.js terminal                    ││
                    │  │  - DesktopStreamViewer                  ││
                    │  └─────────────────────────────────────────┘│
                    └─────────────────────────────────────────────┘
```

**How it works:**

1. **Claude Code CLI runs in tmux** for session persistence
2. **helix-claude-bridge** (new Go component):
   - Attaches to tmux session via PTY
   - Watches `~/.claude/projects/` JSONL files for structured data
   - Relays terminal output to Helix API via WebSocket
   - Exposes SSH server (gliderlabs/ssh) for direct access
   - Sends parsed interactions to Helix sync API
3. **Frontend** displays terminal via xterm.js
4. **BYOK**: User's ANTHROPIC_API_KEY or Claude OAuth token passed to container

**Pros:**
- Native Claude Code experience (CLI as-is)
- Auto-upgrades when user runs `claude update`
- Stable integration (we're just watching files + streaming terminal)
- Full feature access (hooks, subagents, MCP, everything)
- Works with compaction (JSONL has full history)

**Cons:**
- Two data sources (terminal visual + JSONL structured)
- JSONL parsing may need updates as format evolves
- SSH port exposure per session

---

### Option B: SDK-First with Terminal View

**Architecture:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Helix Desktop Container                                         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │  helix-claude-agent (TypeScript/Go)                         │ │
│ │  - Uses @anthropic-ai/claude-agent-sdk                      │ │
│ │  - Renders terminal-like output                             │ │
│ │  - Streams to Helix API                                     │ │
│ └──────────────────────────┬──────────────────────────────────┘ │
└────────────────────────────┼────────────────────────────────────┘
                             ▼
                    Helix API + Frontend (same as Option A)
```

**How it works:**

1. **helix-claude-agent** wraps the Claude Agent SDK
2. Programmatically calls `query()` and streams responses
3. Renders SDK output to look like terminal (custom UI)
4. Parses all messages and stores in Helix database

**Pros:**
- Full programmatic control
- Clean structured data access
- No file watching needed

**Cons:**
- NOT native Claude Code experience (SDK ≠ CLI)
- Missing features: hooks, some slash commands, background tasks
- Must implement terminal rendering ourselves
- Tightly coupled to SDK API changes
- No auto-upgrade benefit

---

### Option C: Dual Mode (CLI + SDK Reader)

**Architecture:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Helix Desktop Container                                         │
│ ┌─────────────┐   ┌──────────────┐   ┌─────────────────────────┐│
│ │   Claude    │──▶│    tmux      │   │  helix-claude-bridge    ││
│ │   Code CLI  │   │  (session)   │   │  - PTY/terminal stream  ││
│ └─────────────┘   └──────────────┘   │  - SDK session reader   ││
│       │                               │  - WebSocket relay      ││
│       ▼                               └──────────┬──────────────┘│
│ ~/.claude/projects/*.jsonl ◀──────────reads──────┘              │
└──────────────────────────────────────────────────┼──────────────┘
                                                   ▼
                                           Helix API
```

**How it works:**

1. **CLI runs natively** in tmux for user interaction
2. **SDK used as reader** to parse session files programmatically
3. Best of both worlds: native UX + structured data access

**Pros:**
- Native CLI experience with auto-upgrade
- Could use SDK for session resume, history access (when available)
- More structured than pure file watching

**Cons:**
- SDK currently can't read historical messages (feature requested)
- Still need file watching for real-time updates
- Adds complexity without clear benefit today

---

### Option D: Zed ACP Path

**Architecture:**
```
┌─────────────────────────────────────────────────────────────────┐
│ Helix Desktop Container                                         │
│ ┌─────────────┐                      ┌─────────────────────────┐│
│ │    Zed IDE  │◀─────(ACP)──────────▶│  claude-code-acp        ││
│ │ (existing)  │                      │  (npm package)          ││
│ └─────────────┘                      └──────────┬──────────────┘│
└──────────────────────────────────────────────────┼──────────────┘
                                                   ▼
                                           Claude API
```

**How it works:**

1. Use existing Zed integration with `@zed-industries/claude-code-acp`
2. Replace Qwen Code with Claude Code as the agent in Zed

**Pros:**
- Reuses existing Helix ↔ Zed integration
- Zed manages Claude Code lifecycle

**Cons:**
- **Missing features**: No hooks, no session resume, no MCP passthrough
- No terminal access (it's Zed, not a terminal)
- Not "native Claude Code experience" - it's Claude Code inside Zed
- Helix's WebSocket sync designed for different protocol
- Subagent support exists but limited

**Verdict:** Not suitable for this use case - user wants native Claude Code terminal experience.

---

## Recommendation: Option A (Terminal Streaming)

Option A provides the best balance of:
- **Native experience**: Users get the real Claude Code CLI
- **Stability**: We're just watching/streaming, not wrapping
- **Full features**: Everything Claude Code offers works
- **Auto-upgrade**: CLI updates automatically
- **BYOK**: Simple env var configuration

### Implementation Plan

#### Phase 1: Core Infrastructure

1. **helix-claude-bridge** (new Go component in `api/pkg/desktop/`):
   ```go
   type ClaudeBridge struct {
       sessionID     string
       tmuxSession   string
       pty           *os.File
       jsonlWatcher  *fsnotify.Watcher
       syncClient    *websocket.Conn
       sshServer     ssh.Server
   }
   ```

2. **JSONL Parser** (parse `~/.claude/projects/*.jsonl`):
   - Extract user messages, assistant responses, tool calls
   - Convert to Helix Interaction format
   - Handle compaction summaries

3. **Terminal WebSocket endpoint** (`/api/v1/sessions/:id/terminal`):
   - Binary WebSocket for xterm.js attach
   - Bidirectional terminal I/O

4. **Frontend terminal component**:
   - xterm.js integration in DesktopStreamViewer
   - Tab for video stream, tab for terminal

#### Phase 2: Session Persistence

1. **Real-time JSONL watcher**:
   - Use fsnotify to watch for file changes
   - Parse new lines as they're appended
   - Send to Helix sessions API

2. **Compaction handling**:
   - Detect summary files
   - Link summaries to original session
   - Preserve full history in Helix even when Claude compacts

3. **Session resume support**:
   - Store Claude session ID in Helix session metadata
   - Pass `--resume` flag when restarting Claude Code

#### Phase 3: SSH Access

1. **Per-session SSH server**:
   - Use gliderlabs/ssh
   - Attach to same tmux session
   - Dynamic port allocation or fixed port + session routing

2. **SSH key management**:
   - Use user's SSH keys from Helix account
   - Or generate per-session keys

#### Phase 4: BYOK Integration

1. **API Key mode**:
   - User provides ANTHROPIC_API_KEY in Helix settings
   - Passed to container as env var

2. **Claude Subscription mode**:
   - OAuth token from Claude.ai login
   - Stored in Helix user settings
   - Passed as CLAUDE_CODE_OAUTH_TOKEN

3. **Helix Proxy mode** (optional):
   - Route through Helix's Anthropic proxy
   - Track token usage in Helix
   - Apply rate limits if needed

---

## JSONL File Format

Claude Code stores sessions in `~/.claude/projects/[encoded-path]/[session-id].jsonl`:

```json
{"type":"user","message":{"role":"user","content":"fix the bug"},"uuid":"...","timestamp":"...","sessionId":"..."}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix..."}]},"uuid":"..."}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{...}}]},"uuid":"..."}
{"type":"tool_result","tool_use_id":"...","content":"file contents...","uuid":"..."}
```

**Mapping to Helix:**
- `user` → Helix Interaction (role=user)
- `assistant` with text → Helix Interaction (role=assistant)
- `assistant` with tool_use → Helix Interaction (role=assistant, tool_calls)
- `tool_result` → Attached to previous assistant interaction

---

## BYOK Configuration

### Option 1: User API Key

```yaml
# In Helix user settings or project config
claude_code:
  auth_mode: api_key
  api_key: sk-ant-...  # Stored encrypted
```

Container env:
```bash
ANTHROPIC_API_KEY=sk-ant-...
```

### Option 2: Claude Subscription (OAuth)

```yaml
claude_code:
  auth_mode: oauth
  oauth_token: <from claude.ai login>  # Stored encrypted
```

Container env:
```bash
CLAUDE_CODE_OAUTH_TOKEN=...
```

### Option 3: Helix Proxy

```yaml
claude_code:
  auth_mode: helix_proxy
```

Container env:
```bash
ANTHROPIC_API_KEY=<helix-session-scoped-token>
ANTHROPIC_BASE_URL=http://api:8080/anthropic
```

This routes through Helix's proxy, enabling:
- Token usage tracking
- Rate limiting
- Cost allocation per user/project

---

## API Endpoints

### New Endpoints

```
# Terminal WebSocket (xterm.js attach)
GET /api/v1/sessions/:session_id/terminal
  WebSocket: Binary frames for terminal I/O
  Query params: rows, cols (for initial size)

# SSH connection info
GET /api/v1/sessions/:session_id/ssh
  Response: { host, port, username, one_time_password }
```

### Modified Endpoints

```
# Session creation - add claude_code agent type
POST /api/v1/sessions
  Body: { agent_host_type: "claude_code", ... }

# External agent sync - works as-is
GET /api/v1/external-agents/sync
  WebSocket: Existing protocol, but interactions come from JSONL parser
```

---

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           helix-ubuntu Container                             │
│                                                                             │
│  ┌────────────────┐    stdio    ┌────────────────┐                          │
│  │  Claude Code   │◀───────────▶│     tmux       │                          │
│  │     CLI        │             │   (session)    │                          │
│  └───────┬────────┘             └───────┬────────┘                          │
│          │                              │                                   │
│          │ writes                       │ PTY attach                        │
│          ▼                              ▼                                   │
│  ┌────────────────┐             ┌────────────────────────────────────────┐  │
│  │ ~/.claude/     │  inotify    │         helix-claude-bridge            │  │
│  │  projects/     │────────────▶│                                        │  │
│  │  *.jsonl       │             │  ┌──────────┐ ┌──────────┐ ┌────────┐  │  │
│  └────────────────┘             │  │  JSONL   │ │ Terminal │ │  SSH   │  │  │
│                                 │  │  Parser  │ │  Relay   │ │ Server │  │  │
│                                 │  └────┬─────┘ └────┬─────┘ └───┬────┘  │  │
│                                 └───────┼────────────┼───────────┼───────┘  │
│                                         │            │           │          │
└─────────────────────────────────────────┼────────────┼───────────┼──────────┘
                                          │            │           │
              ┌───────────────────────────┴────────────┴───────────┴────────┐
              │                                                             │
              │  ┌─────────────────┐    ┌─────────────────┐                 │
              │  │  Helix API      │    │  SSH Client     │                 │
              │  │  WebSocket      │    │  (user laptop)  │                 │
              │  │  /sync          │    │                 │                 │
              │  │  /terminal      │    └─────────────────┘                 │
              │  └────────┬────────┘                                        │
              │           │                                                 │
              │           ▼                                                 │
              │  ┌─────────────────────────────────────────────┐            │
              │  │              Frontend                       │            │
              │  │  ┌─────────────────┐ ┌─────────────────────┐│            │
              │  │  │  Video Stream   │ │  Terminal (xterm.js)││            │
              │  │  │  (existing)     │ │  (new)              ││            │
              │  │  └─────────────────┘ └─────────────────────┘│            │
              │  └─────────────────────────────────────────────┘            │
              └─────────────────────────────────────────────────────────────┘
```

---

## Risk Analysis

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| JSONL format changes | Medium | Medium | Version detection, schema validation |
| Claude Code CLI breaking changes | Low | High | Integration tests, version pinning option |
| OAuth token expiration | Medium | Low | Refresh flow, clear error messages |
| Terminal escape sequence issues | Low | Low | Use proven xterm.js + tmux stack |
| High latency on terminal relay | Low | Medium | Local WebSocket, minimal processing |

---

## Success Metrics

1. **Latency**: Terminal keypress-to-display < 50ms
2. **Reliability**: 99.9% of Claude Code sessions successfully streamed to Helix
3. **Completeness**: 100% of JSONL interactions captured in Helix database
4. **User satisfaction**: Users prefer Helix Claude Code over standalone CLI

---

## Open Questions

1. **Terminal size**: Fixed size or dynamic resize support?
2. **Multiple terminals**: Allow multiple Claude Code sessions per project?
3. **Session limits**: Max concurrent Claude Code sessions per user?
4. **SSH key management**: Per-session or per-user SSH keys?
5. **Offline mode**: What happens if Helix API is unreachable?

---

## Timeline Estimate

- Phase 1 (Core Infrastructure): 1 week
- Phase 2 (Session Persistence): 3-4 days
- Phase 3 (SSH Access): 2-3 days
- Phase 4 (BYOK Integration): 2-3 days
- Testing and polish: 1 week

**Total: ~3 weeks**

---

## Appendix: Research Sources

- [Claude Agent SDK Documentation](https://platform.claude.com/docs/en/agent-sdk/overview)
- [Claude Code CLI Reference](https://code.claude.com/docs/en/cli-reference)
- [Claude Code Hooks](https://code.claude.com/docs/en/hooks)
- [Claude Code Session Persistence](https://code.claude.com/docs/en/how-claude-code-works)
- [@zed-industries/claude-code-acp](https://github.com/zed-industries/claude-code-acp)
- [gliderlabs/ssh](https://github.com/gliderlabs/ssh)
- [xterm.js](https://xtermjs.org/)
- [creack/pty](https://github.com/creack/pty)
