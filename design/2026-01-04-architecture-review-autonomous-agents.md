# Architecture Review: Autonomous Development Agent Infrastructure

**Date:** 2026-01-04
**Author:** Staff Engineering Review
**Status:** Implementation Complete

---

## Executive Summary

This review covers the infrastructure built tonight for autonomous development agents with full desktop access. The changes enable AI agents to:
1. See their work through screenshots
2. Navigate and manage their conversation history
3. Control windows and workspaces
4. Post progress updates with visual proof to Slack/Teams

---

## 1. Session Context Navigation (Session MCP)

### Purpose
Agents lose context in long-running sessions. The Session MCP server provides tools for agents to navigate their own conversation history.

### Implementation

**Location:** `api/pkg/server/mcp_backend_session.go`

**Tools:**
| Tool | Description |
|------|-------------|
| `current_session` | Get session overview (name, turn count, title changes) |
| `session_toc` | Get numbered table of contents with one-line summaries |
| `get_turn` | Retrieve full content of a specific turn |
| `session_title_history` | See how session topic evolved |
| `search_session` | Search within session interactions |

**Data Flow:**
```
Zed Agent → HTTP MCP Gateway → SessionMCPBackend → Store → Session/Interaction data
            /api/v1/mcp/session?session_id=xxx
```

**Key Design Decision:**
The Session MCP runs on the Helix API server (not in the sandbox) because it needs database access. It's authenticated via the MCP gateway with the same token used for other Helix MCP services.

### Integration Points
- Connected via `zed_config.go` → Zed settings.json → Qwen Code Agent
- Registered in `server.go` alongside Kodit and Helix native MCP backends

---

## 2. Desktop Control MCP

### Purpose
Allow agents to see and control the desktop environment - take screenshots, manage windows, interact with applications.

### Implementation

**Location:** `api/pkg/desktop/mcp_server.go`

**Tools Added:**
| Tool | Description | Backend |
|------|-------------|---------|
| `take_screenshot` | Capture current screen | screenshot-server |
| `save_screenshot` | Save screenshot to file | screenshot-server |
| `type_text` | Type text via virtual keyboard | wtype/xdotool |
| `mouse_click` | Click at coordinates | wtype/xdotool |
| `get_clipboard` | Read clipboard content | wl-paste/xclip |
| `set_clipboard` | Set clipboard content | wl-copy/xclip |
| `list_windows` | List all windows with IDs | swaymsg/wmctrl |
| `focus_window` | Focus a window by ID | swaymsg/wmctrl |
| `maximize_window` | Maximize a window | swaymsg/wmctrl |
| `tile_window` | Tile window left/right | swaymsg/wmctrl |
| `move_to_workspace` | Move window to workspace | swaymsg |
| `switch_to_workspace` | Switch active workspace | swaymsg |
| `get_workspaces` | List all workspaces | swaymsg |

**Architecture:**
```
            ┌─────────────────────────────────────┐
            │         Sandbox Container            │
            │                                      │
            │  ┌──────────────────────────────┐   │
            │  │    Desktop MCP Server         │   │
            │  │    (port 9877)                │   │
            │  └──────────────────────────────┘   │
            │              │                       │
            │              ▼                       │
            │  ┌──────────────────────────────┐   │
            │  │    Screenshot Server          │   │
            │  │    (port 9876)                │   │
            │  └──────────────────────────────┘   │
            │              │                       │
            │              ▼                       │
            │  ┌──────────────────────────────┐   │
            │  │  Sway/GNOME Desktop           │   │
            │  │  + Wolf (streaming)            │   │
            │  └──────────────────────────────┘   │
            └─────────────────────────────────────┘
```

### Port Allocation
- 9876: Screenshot server (H264 streaming, capture)
- 9877: Desktop MCP (tools for agent)
- 9878: Reserved for future use

---

## 3. Chrome DevTools MCP

### Purpose
Enable browser automation and debugging for agents testing web applications.

### Implementation

**Dockerfiles:** `Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`

**Installation:**
```dockerfile
# Chrome browser
RUN apt-get install -y fonts-liberation libasound2...
    && wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
    && dpkg -i /tmp/google-chrome.deb

# Chrome DevTools MCP
RUN npm install -g chrome-devtools-mcp@latest
```

**Zed Configuration:** (`zed_config.go`)
```go
config.ContextServers["chrome-devtools"] = ContextServerConfig{
    Command: "npx",
    Args:    []string{"chrome-devtools-mcp@latest"},
    Env: map[string]string{
        "CHROME_DEVTOOLS_MCP_HEADLESS": "true",
        "CHROME_DEVTOOLS_MCP_VIEWPORT": "1920x1080",
    },
}
```

### Available Tools (from chrome-devtools-mcp)
- `navigate` - Go to URL
- `screenshot` - Capture browser viewport
- `click` - Click element
- `type` - Type into element
- `evaluate` - Run JavaScript
- `getConsoleMessages` - Read console
- `getNetworkRequests` - Inspect network
- `startPerformanceTrace` / `endPerformanceTrace` - Performance analysis

---

## 4. Slack/Teams Progress Notifications

### Purpose
Keep humans informed of agent progress without requiring constant monitoring. Post updates with screenshots and interaction summaries.

### Implementation

**New Files:**
- `api/pkg/trigger/slack/agent_progress.go`
- `api/pkg/trigger/teams/agent_progress.go`

**Thread Type Extensions:**
```go
type SlackThread struct {
    // ... existing fields ...
    PostProgressUpdates bool  // Post turn summaries to thread
    IncludeScreenshots  bool  // Include desktop screenshots
}

type TeamsThread struct {
    // ... existing fields ...
    ServiceURL          string // Required for posting back
    PostProgressUpdates bool
    IncludeScreenshots  bool
}
```

**Automatic Configuration:**
When a Slack/Teams thread creates a session with an external agent (`zed_external`), progress updates are automatically enabled.

**Data Flow:**
```
External Agent completes turn
         │
         ▼
Summary Service generates one-line summary
         │
         ▼
Check if session has Slack/Teams thread with PostProgressUpdates=true
         │
         ▼
Take screenshot (if IncludeScreenshots=true)
         │
         ▼
Post update to thread with summary + screenshot + action buttons
         │
         ▼
User can reply in thread → routed back to agent via existing trigger
```

**Message Format (Slack Block Kit):**
```
🔄 Session Name: Implementing dark mode
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Turn 5: Added toggle component to Settings page
[Screenshot of current desktop state]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[View Session] [Stop Agent]
```

---

## 5. MCP Gateway Architecture

### Current MCP Backends

| Backend | Path | Purpose |
|---------|------|---------|
| `kodit` | `/api/v1/mcp/kodit` | Code intelligence |
| `helix` | `/api/v1/mcp/helix` | Native tools (APIs, RAG, Zapier) |
| `session` | `/api/v1/mcp/session` | Session navigation |
| `helix-desktop` | `localhost:9877/mcp` | Desktop control (in-sandbox) |
| `chrome-devtools` | stdio (npx) | Browser automation |

### Authentication Flow
```
Zed Agent
    │
    ├─── Local MCPs (no auth) ──────────► Desktop MCP, Chrome DevTools
    │
    └─── Remote MCPs (auth required) ──► Helix MCP Gateway
                                              │
                                              ▼
                                         Auth validation
                                         (Bearer token from settings.json)
                                              │
                                              ▼
                                         Route to backend
```

---

## 6. Configuration Flow

### How Zed Gets Its MCP Configuration

```
1. Session created with external agent
           │
           ▼
2. wolf_executor.go launches sandbox with:
   - HELIX_SESSION_ID
   - HELIX_API_URL
   - HELIX_API_TOKEN
           │
           ▼
3. settings-sync-daemon inside sandbox:
   - Fetches /api/v1/sessions/{id}/zed-config
   - Writes to ~/.config/zed/settings.json
           │
           ▼
4. GenerateZedMCPConfig() creates config:
   - helix-native (if APIs/RAG configured)
   - kodit (if enabled)
   - helix-desktop (always, port 9877)
   - helix-session (always, via HTTP)
   - chrome-devtools (always, stdio)
           │
           ▼
5. Zed loads settings.json → MCPs available to Qwen Code Agent
```

---

## 7. Testing Gaps

### Needs Tests
1. **SessionMCPBackend** - Unit tests for all 5 tools
2. **Window management** - Integration tests with mock swaymsg
3. **Slack/Teams progress** - Mock webhook tests

### Has Tests
- Session TOC store operations (`store_session_toc_test.go`)
- Summary service (existing tests)
- Slack/Teams bot core functionality (existing tests)

---

## 8. Security Considerations

### Sandbox Isolation
- Desktop MCP runs inside sandbox, only accessible to that agent
- Session MCP uses token validation via MCP gateway
- Chrome runs in headless mode with no external network by default

### Token Management
- Settings-sync-daemon receives token from environment
- Token has session-scoped permissions
- Token embedded in settings.json (file permissions protect it)

---

## 9. Future Improvements

1. **Semantic Search Across Sessions**
   - Use embeddings to find similar problems solved before
   - Currently search is substring-based

2. **Multi-Agent Coordination**
   - Share context between frontend/backend/test agents
   - Currently each agent is isolated

3. **Continuous Learning**
   - Track which solutions worked well
   - Suggest proven patterns for similar problems

4. **Rate Limiting for Progress Updates**
   - Add frequency control (every N turns)
   - Currently would post every turn

---

## 10. Commit History (Tonight)

```
83a41ae71 feat(triggers): enable progress updates for external agent sessions
816a41476 feat(triggers): add agent progress updates with screenshots to Slack/Teams
141381e2f feat(mcp): add Chrome DevTools, Session MCP, and enhanced desktop tools
4597d034a feat(desktop): add window management MCP tools with Sway/GNOME backends
3903255d4 feat(session): add session TOC, summaries, and separate session MCP server
```

---

## Conclusion

The infrastructure is in place for fully autonomous development agents that can:
- See what they're doing (screenshots via Desktop MCP)
- Remember what they've done (session navigation via Session MCP)
- Test their work (Chrome DevTools MCP)
- Report progress (Slack/Teams notifications with screenshots)
- Receive human feedback (existing trigger threads)

The architecture is modular - each MCP server handles one domain, and the gateway provides unified authentication. External agents triggered via Slack/Teams automatically get progress updates enabled.

Next steps: Add unit tests for SessionMCPBackend and consider rate limiting for progress updates.
