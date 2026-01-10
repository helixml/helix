# Session Table of Contents and Interaction Summaries

**Date:** 2026-01-04
**Author:** Luke
**Status:** Implemented

## Problem

AI agents lose context as conversations grow long. The "context compaction problem" means agents can't easily recall how they approached similar tasks in previous conversation turns. This leads to:

1. Agents repeating work they've done before
2. Loss of patterns and approaches discovered earlier in the session
3. Inability to reference specific past interactions efficiently

## Solution

Add a **Table of Contents (TOC)** for sessions with:

1. **Numbered turn-based navigation** - Each interaction gets a turn number (1-indexed)
2. **One-line summaries** - Generated for each interaction using the kodit enrichment model
3. **Session title auto-updates** - Title reflects the most recent topic being worked on

## Architecture

### New Endpoints

```
GET /api/v1/sessions/{id}/toc           - Get session TOC (numbered list of summaries)
GET /api/v1/sessions/{id}/turns/{turn}  - Get specific interaction with prev/next context
GET /api/v1/sessions/{id}/search        - Search within session interactions
```

### Response Types

```go
type SessionTOCEntry struct {
    Turn        int       `json:"turn"`         // 1-indexed turn number
    ID          string    `json:"id"`           // Interaction ID
    Summary     string    `json:"summary"`      // One-line summary (max 100 chars)
    Created     time.Time `json:"created"`
    HasPrompt   bool      `json:"has_prompt"`
    HasResponse bool      `json:"has_response"`
}

type SessionTOCResponse struct {
    SessionID   string            `json:"session_id"`
    SessionName string            `json:"session_name"`
    TotalTurns  int               `json:"total_turns"`
    Entries     []SessionTOCEntry `json:"entries"`
    Formatted   string            `json:"formatted"`  // Pre-formatted numbered list
}

type InteractionWithContext struct {
    Turn        int                `json:"turn"`
    Interaction *types.Interaction `json:"interaction"`
    Previous    *InteractionBrief  `json:"previous,omitempty"`
    Next        *InteractionBrief  `json:"next,omitempty"`
}
```

### Summary Generation Flow

```
Interaction Completes
        │
        ▼
triggerSummaryGeneration()
        │
        ├──► GenerateInteractionSummaryAsync()
        │           │
        │           ▼
        │    Call kodit model with prompt/response
        │           │
        │           ▼
        │    Save summary to interaction.Summary
        │
        └──► UpdateSessionTitleAsync()
                    │
                    ▼
             Build TOC (reverse chronological, [RECENT] markers)
                    │
                    ▼
             Call kodit model with title prompt
                    │
                    ▼
             Update session.Name if topic changed
```

### Avoiding Infinite Loops

Summary generation requests do NOT create new interactions because:

1. The `SummaryService` uses the provider manager directly (not the `/v1/chat/completions` endpoint)
2. Sessions with `Metadata.SystemSession = true` skip summary generation
3. Rate limiting prevents overwhelming the LLM provider (max 5 concurrent requests)

## MCP Server Separation

Session navigation and desktop control are provided by **separate MCP servers**:

### Desktop MCP Server (`api/pkg/desktop/mcp_server.go`)

Tools for desktop interaction, only useful in Sway/desktop environments:
- `take_screenshot` - Capture and return base64 screenshot
- `save_screenshot` - Capture and save to file
- `type_text` - Type text via wtype/ydotool
- `mouse_click` - Click at screen coordinates
- `get_clipboard` / `set_clipboard` - Wayland clipboard access

Port: 9877

### Session MCP Server (`api/pkg/session/mcp_server.go`)

Tools for session navigation, useful for **all agents** (not just desktop):
- `current_session` - Quick overview of current session
- `session_toc` - Table of contents for a session
- `session_title_history` - See how session topic evolved
- `search_session` - Search within a session
- `search_all_sessions` - Cross-session search
- `list_sessions` - List recent sessions
- `get_turn` / `get_turns` - Retrieve specific conversation turns
- `get_interaction` - Get interaction by ID

Port: 9878

The separation allows session navigation to be used by any AI agent, not just those in desktop environments.

## Configuration

The kodit model is configured in System Settings:

```go
type SystemSettings struct {
    KoditEnrichmentProvider string  // e.g., "together_ai", "openai", "helix"
    KoditEnrichmentModel    string  // e.g., "Qwen/Qwen3-8B", "gpt-4o"
}
```

If not configured, extractive summaries are used (first line of prompt/response).

## Session Title Update Logic

Titles are biased toward **new topics at the end** of the conversation:

1. TOC is built in **reverse chronological order** (newest first)
2. Last 3 turns are marked with `[RECENT]`
3. Prompt instructs model to:
   - Update title if [RECENT] turns discuss a NEW topic
   - Keep current title if conversation continues on same topic
   - Focus on what user is CURRENTLY working on

Example prompt:
```
Current session title: "Implementing user authentication"

Conversation turns (newest first, [RECENT] marks last 3 turns):
[RECENT] Turn 15: Setting up MCP server for screenshots
[RECENT] Turn 14: Adding desktop screenshot tool
[RECENT] Turn 13: Exploring MCP integration
Turn 12: Testing OAuth flow
...

Generate a session title (max 60 characters).
- If the [RECENT] turns discuss a NEW topic different from the current title, update to new topic
- If still on same topic, keep current title
- Focus on what user is CURRENTLY working on
```

Result: "MCP server for desktop screenshots"

## Title History Tracking

Session titles evolve as the conversation changes topics. We track this history so users can:
- See what topics were covered in a session at a glance
- Click on a historical title to jump to that part of the conversation

### TitleHistoryEntry Structure

```go
type TitleHistoryEntry struct {
    Title         string    `json:"title"`          // The title that was set
    ChangedAt     time.Time `json:"changed_at"`     // When the title was changed
    Turn          int       `json:"turn"`           // Turn number that triggered the change
    InteractionID string    `json:"interaction_id"` // For navigation - click to jump here
}
```

### Storage

- Stored in `SessionMetadata.TitleHistory` (JSONB in config column)
- Newest first (prepend on title change)
- Max 20 entries retained

## UI Integration

### SpecTask Tab View

When hovering over a session tab:
- Show title history (newest first) - each entry shows the topic that was worked on
- Click on a historical title to jump to that interaction in the conversation
- User can see at a glance what topics were covered in the session

### Chat Session View

- Session title auto-updates as conversation evolves
- TOC can be displayed in a sidebar for quick navigation
- Click on turn number to jump to that interaction

## Files Changed

- `api/pkg/types/types.go` - Added `Summary`, `SummaryUpdatedAt` to Interaction; `SystemSession`, `TitleHistory` to SessionMetadata; `TitleHistoryEntry` type
- `api/pkg/store/store.go` - Added `UpdateSessionMetadata`, `UpdateInteractionSummary` interface methods
- `api/pkg/store/store_sessions.go` - Implemented `UpdateSessionMetadata`
- `api/pkg/store/store_interactions.go` - Implemented `UpdateInteractionSummary`
- `api/pkg/server/session_toc_handlers.go` - NEW: TOC endpoint handlers
- `api/pkg/server/summary_service.go` - NEW: Async summary generation service with title history tracking
- `api/pkg/server/session_handlers.go` - Hook summary generation on interaction completion
- `api/pkg/server/server.go` - Add routes and initialize SummaryService
- `api/pkg/session/mcp_server.go` - NEW: Session navigation MCP server (separate from desktop)
- `api/pkg/desktop/mcp_server.go` - Desktop-only MCP tools (screenshot, clipboard, mouse, keyboard)

## Window Management Tools

Added comprehensive window management MCP tools to the desktop MCP server (`api/pkg/desktop/mcp_server.go`):

| Tool | Description |
|------|-------------|
| `list_windows` | Get all open windows with IDs, titles, and workspace |
| `focus_window` | Focus a window by ID or title |
| `maximize_window` | Maximize or fullscreen a window |
| `tile_window` | Tile window to left or right half of screen |
| `move_to_workspace` | Move window to a specific workspace number |
| `switch_to_workspace` | Switch to a workspace number |
| `get_workspaces` | List all workspaces with focus state |

### Desktop Environment Detection

The MCP server auto-detects the desktop environment:

```go
func (m *MCPServer) detectDesktopEnvironment() string {
    if _, err := exec.LookPath("swaymsg"); err == nil {
        cmd := exec.Command("swaymsg", "-t", "get_version")
        if err := cmd.Run(); err == nil {
            return "sway"
        }
    }
    if os.Getenv("XDG_CURRENT_DESKTOP") == "GNOME" {
        return "gnome"
    }
    return "sway"  // default
}
```

### Sway Backend

Uses `swaymsg` commands:
- `swaymsg -t get_tree` - Parse window tree JSON
- `swaymsg -t get_workspaces` - List workspaces
- `swaymsg [con_id=X] focus` - Focus by container ID
- `swaymsg move container to workspace number N` - Move to workspace

### GNOME Backend

Uses `wmctrl` and `gdbus`:
- `wmctrl -l -p` - List windows
- `wmctrl -i -a <id>` - Focus by ID
- `wmctrl -r :ACTIVE: -b add,maximized_vert,maximized_horz` - Maximize
- `wmctrl -s N` - Switch to workspace N

## Chrome DevTools MCP Server

Added Google Chrome and chrome-devtools-mcp to container images for AI agent browser control.

### Installation

```dockerfile
# Install Chrome
RUN wget -q -O /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
    dpkg -i /tmp/google-chrome.deb || apt-get install -f -y

# Install chrome-devtools-mcp
RUN npm install -g chrome-devtools-mcp@latest

# Configuration
ENV CHROME_DEVTOOLS_MCP_HEADLESS=true \
    CHROME_DEVTOOLS_MCP_VIEWPORT=1920x1080
```

### Capabilities

The Chrome DevTools MCP server provides 26 tools across 6 categories:
- **Input automation** (7 tools): Click, type, scroll, keyboard events
- **Navigation** (7 tools): goto, back, forward, reload, wait
- **Debugging** (4 tools): Console, DOM inspection, CSS
- **Network** (2 tools): Request interception, response analysis
- **Performance** (3 tools): Trace recording, metrics
- **Emulation** (3 tools): Device emulation, geolocation

### Usage

Agents can use Chrome to:
- Test frontend changes in a real browser
- Take screenshots of UI for visual verification
- Debug JavaScript errors in the console
- Measure performance metrics
- Interact with web applications as part of testing

Reference: https://developer.chrome.com/blog/chrome-devtools-mcp

## Slack/Teams Integration (Planned)

**Goal**: Keep users informed of agent progress without requiring them to watch the session.

### Notification Types

1. **Status Updates**: When agent completes a milestone (spec approved, implementation done, tests passing)
2. **Screenshot Previews**: Show current state of work (desktop screenshot, browser screenshot)
3. **Attention Required**: When agent needs human input or is blocked

### Implementation Plan

```go
type AgentNotification struct {
    Type        string    // "status", "screenshot", "attention"
    SessionID   string
    TaskID      string
    Summary     string    // One-line summary from interaction
    ScreenshotURL string  // Optional S3 URL to screenshot
    SessionURL  string    // Link to Helix session for details
}
```

### Integration Points

- **Slack**: Use Slack Block Kit for rich notifications with images
- **Teams**: Use Adaptive Cards for similar rich notifications
- **Webhook**: Generic webhook for custom integrations

### Trigger Logic

```go
// Trigger notification when agent is idle for >30 seconds after completing work
if time.Since(lastInteractionTime) > 30*time.Second && !isAgentActive {
    sendNotification(AgentNotification{
        Type:    "attention",
        Summary: getLatestInteractionSummary(sessionID),
    })
}
```

## Files Changed

- `api/pkg/types/types.go` - Added `Summary`, `SummaryUpdatedAt` to Interaction; `SystemSession`, `TitleHistory` to SessionMetadata; `TitleHistoryEntry` type
- `api/pkg/store/store.go` - Added `UpdateSessionMetadata`, `UpdateInteractionSummary` interface methods
- `api/pkg/store/store_sessions.go` - Implemented `UpdateSessionMetadata`
- `api/pkg/store/store_interactions.go` - Implemented `UpdateInteractionSummary`
- `api/pkg/store/store_session_toc_test.go` - NEW: Unit tests for session TOC functionality
- `api/pkg/server/session_toc_handlers.go` - NEW: TOC endpoint handlers
- `api/pkg/server/summary_service.go` - NEW: Async summary generation service with title history tracking, pubsub integration
- `api/pkg/server/session_handlers.go` - Hook summary generation on interaction completion
- `api/pkg/server/server.go` - Add routes and initialize SummaryService with pubsub
- `api/pkg/controller/sessions.go` - Added WebSocket event publishing on title update
- `api/pkg/session/mcp_server.go` - NEW: Session navigation MCP server (separate from desktop)
- `api/pkg/desktop/mcp_server.go` - Desktop-only MCP tools (screenshot, clipboard, mouse, keyboard, window management)
- `frontend/src/components/tasks/TabsView.tsx` - Title history tooltip on hover
- `Dockerfile.sway-helix` - Added Chrome and chrome-devtools-mcp
- `Dockerfile.ubuntu-helix` - Added Chrome and chrome-devtools-mcp

## Future Enhancements

1. **Semantic Search** - Use embeddings for better session content search
2. **Summary Caching** - Pre-generate summaries in background for older sessions
3. **Cross-Session TOC** - Unified TOC across related SpecTask sessions
4. ~~**Title History UI** - Frontend component to display title history on tab hover~~ (DONE)
5. **Slack/Teams Notifications** - Real-time agent status updates to collaboration platforms
6. **Browser Testing Automation** - Use Chrome MCP for automated UI testing in SpecTask workflow
