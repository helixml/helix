# Helix TUI Client (hmux)

**Date:** 2026-03-30
**Branch:** `feature/tui-client`
**Status:** Design + Phase 1 built

## Problem

The Helix web UI requires a browser with reasonable bandwidth. Over satellite
internet, SSH/mosh connections, or air-gapped environments, the browser UI is
unusable. We need a full terminal-native Helix client — not just a viewer, but
a complete interface for managing spec tasks, reviewing specs, chatting with
agents, and interacting with dev containers.

**Design goals:**
- Works great over high-latency, low-bandwidth connections (satellite, mosh)
- TUI users and web users coexist — same backend, same data
- No flickering, no missed keystrokes, no polling-induced input lag
- Individually testable components for long-term sustainability
- Eventually embeddable in the web interface as a terminal component

## UX Philosophy

### Mirror Claude Code exactly
Copy Claude Code's look and feel as closely as possible: colors, borders,
spacing, spinner, input prompt, diff rendering. Users should feel at home
immediately.

### Spinner with British humor
The "thinking" spinner uses hilariously British verbs instead of generic
"Thinking...":

```
✽ Waiting for kettle… (12s · ↑ 1.3k tokens)
✽ Taking afternoon tea… (34s · ↑ 2.1k tokens)
✽ Complaining about the weather… (59s · ↑ 4.2k tokens)
✽ Raise your pinky finger… (1m23s · ↑ 6.0k tokens)
✽ Going for a walk… (2m01s · ↑ 8.4k tokens)
✽ Watching the football… (3m12s · ↑ 12k tokens)
✽ Going to the pub… (4m44s · ↑ 15k tokens)
✽ Going for lunch… (6m02s · ↑ 18k tokens)
✽ Sunday roast anyone?… (8m15s · ↑ 22k tokens)
✽ Fancy a cuppa?… (10m30s · ↑ 25k tokens)
```

Below spinner, show tip like Claude Code:
```
  ⎿  Tip: Use /btw to ask a quick side question without interrupting the agent
```

### Input area — exact Claude Code clone

```
────────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions is always on (you're in a sandbox) · esc to interrupt
```

### No flickering, no missed keystrokes

**Problem:** Claude Code re-renders the entire conversation on every update,
causing flickering and dropped keystrokes when rendering is slow.

**Solution:**
- Use bubbletea's `tea.WithoutRenderer()` for the input area — manage it
  separately with direct terminal writes so input is always responsive
- Only re-render the message viewport when new content arrives, not on every
  keystroke
- Use a ring buffer for visible messages, not full re-render
- Input buffer is completely independent of render cycle — keystrokes go
  straight into a local buffer, rendered independently
- Test: input latency must be <16ms (one frame) even during heavy re-render

## Views

### 1. Kanban Board (home view)

Full-screen kanban board. See Phase 1 implementation (already built).

### 2. Chat View (spec task detail)

Shows the conversation with the agent. Renders tool calls visually:

#### Tool call rendering

Must render the same tool calls that Zed sends over the wire. Key renders:

**File edits (red/green diff):**
```
  Assistant
  ✽ Editing src/auth/login.go

  ┌─ src/auth/login.go ──────────────────────────────────────────────┐
  │  - func validateEmail(email string) bool {                       │
  │  -     return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@...`).     │
  │  -         MatchString(email)                                    │
  │  + func validateEmail(email string) bool {                       │
  │  +     if len(email) > 254 {                                     │
  │  +         return false                                          │
  │  +     }                                                         │
  │  +     parts := strings.SplitN(email, "@", 2)                    │
  │  +     return len(parts) == 2 && parts[0] != "" && parts[1] != ""│
  │  }                                                               │
  └──────────────────────────────────────────────────────────────────┘
```

Red background for removed lines, green for added (same colors as Claude Code).

**File reads:**
```
  ✽ Read src/auth/login.go (lines 1-45)
```

**Terminal commands:**
```
  ✽ Running: go test ./auth/...
  ⎿  ok  github.com/example/auth  0.234s
```

**Search results:**
```
  ✽ Searched for "validateEmail" in 42 files
  ⎿  Found 3 matches
```

### 3. Split Panes (tmux-style)

Already designed and built in Phase 1.

### 4. Tabs (tmux-style, bottom bar)

Tabs at the bottom of the screen, like tmux's window list. Each tab is a
spec task (or special view like kanban).

```
╭─ Fix login (#3) ───────────────────────────────────────────────────╮
│  ...conversation...                                                │
╰────────────────────────────────────────────────────────────────────╯
❯ █
────────────────────────────────────────────────────────────────────────
 0:kanban  1:Fix login*  2:Refactor DB  3:Add auth
```

- `{prefix} c` — create new tab (shows picker: create task or select existing)
- `{prefix} n` / `{prefix} p` — next/prev tab
- `{prefix} 0-9` — jump to tab by number
- `{prefix} ,` — rename tab
- `{prefix} &` — close tab
- Tab `0` is always kanban

Tabs and panes are independent — each tab can have its own pane splits.

### 5. Embedded Terminal (sandbox shell)

Resumable remote terminal sessions into the dev container. When you detach
and reattach, the terminal session survives.

**Option A: Use tmux under the hood**
- The TUI starts a tmux session inside the sandbox container
- Terminal panes connect to that tmux session
- Detach/reattach is free — tmux handles it
- Predictive local echo for interactive use (like mosh)

**Option B: Lightweight implementation**
- PTY allocation via exec API into container
- Session state stored in container-side process
- Reconnect re-attaches to same PTY
- Simpler but no session survival across container restarts

Recommendation: **Option A** — tmux is already available in the sandbox
containers, and reinventing session persistence is not worth it.

### 6. Spec Review UI

For tasks in `spec_review` status, the TUI provides a review interface:

```
╭─ Review: Add auth (#1) ───────────────────────────────────────────╮
│                                                                    │
│  ## Requirements Specification                                     │
│                                                                    │
│  ### User Stories                                                  │
│  1. As a user, I want to log in with OAuth2 so that I can use     │
│     my existing Google/GitHub account                              │
│                                                                    │
│  ### Acceptance Criteria                                           │
│  - [ ] OAuth2 flow works with Google provider                     │
│  - [ ] OAuth2 flow works with GitHub provider                     │
│  - [ ] Session tokens are stored securely                         │
│                                                                    │
│  ## Technical Design                                               │
│  ...                                                               │
│                                                                    │
│  ┌─ Comment on lines 5-7 ─────────────────────────────────────┐   │
│  │ Should we also support Microsoft Entra ID? Enterprise       │   │
│  │ customers will need it.                                     │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
╰────────────────────────────────────────────────────────────────────╯
 shift+V: select lines  c: comment  a: approve  r: request changes
```

- **shift+V**: enter visual line selection mode (vim-style), select range
  of spec text to comment on
- **c**: add comment on selected text (opens input)
- **a**: approve specs → moves task to implementation
- **r**: request changes → sends revision request with comment
- Comments stream back from the agent as it responds

Emacs equivalent for visual select: TBD (C-SPC for mark, movement for
region selection).

### 7. Slash Commands

Clone Claude Code's slash command UX. Type `/` in the input to see available
commands:

```
❯ /
  /mcp         Configure MCP servers for this project/agent
  /model       Switch the model for this task
  /approve     Approve the current spec/implementation
  /reject      Request changes to spec/implementation
  /branch      Show/change the working branch
  /pr          Show pull request status
  /logs        Show agent logs
  /web         Open in web browser
  /btw         Ask a side question without interrupting
  /status      Show task status details
  /help        Show all commands
```

**MCP configuration** (clone Claude Code's UX exactly, same colors):
```
❯ /mcp
  MCP Servers for "myproject"

  ✓ drone-ci       Connected (4 tools)
  ✓ github         Connected (12 tools)
  ✗ slack          Disconnected

  a: add server  r: remove  e: edit config  esc: back
```

Adding an MCP server calls the Helix API to update project/agent settings.

### 8. Mosh-style Connection Management

#### Disconnection bar
When the connection to the Helix API is lost (timeout, network change),
show a mosh-style bar at the top:

```
┌─────────────────────────────────────────────────────────────────────┐
│ helix: Last contact 2:08 ago. [To quit: Ctrl-^ .]                 │
└─────────────────────────────────────────────────────────────────────┘
```

The bar appears/disappears automatically. While disconnected:
- Input continues to work (queued locally)
- Typing is shown with predictive local echo
- Queued messages are sent when connection resumes
- Notifications are fetched on reconnect (already queued on backend)

#### Predictive write-ahead
For users running hmux locally (not over SSH), implement mosh-style
predictive local echo:
- Keystrokes are rendered immediately in the input buffer
- A background goroutine sends them to the API
- If the prediction was wrong (API rejects), roll back
- Visual indicator: predicted text shown in slightly dimmer color until
  confirmed

#### Reliable prompt entry
Reuse the same infrastructure the web frontend uses:
- Messages are queued in a local outbox
- Each message gets a client-side ID
- On send, message goes to outbox → API call → on success, remove from outbox
- On failure, retry with exponential backoff
- On reconnect, flush the outbox
- User sees their message immediately (optimistic rendering) with a
  "sending..." indicator until confirmed

### 9. Notifications

Backend already queues notifications. The TUI:
- Polls for new notifications on the tick interval
- Shows unread count in the tab bar: `1:Fix login* (2)`
- `{prefix} !` opens notification list
- Notifications include: spec ready for review, implementation complete,
  PR created, agent error, etc.

## Key Bindings (complete)

### Global
| Key | Action |
|-----|--------|
| `ctrl+c` | Quit |
| `esc` | Stop agent / clear input / back (progressive) |

### Kanban
| Key | Action |
|-----|--------|
| `h/l` | Column left/right |
| `j/k` | Task up/down |
| `enter` | Open task in new tab |
| `n` | New task |
| `r` | Refresh |
| `/` | Search/filter |
| `1-5` | Jump to column |

### Chat (tab/pane)
| Key | Action |
|-----|--------|
| `enter` | Send message |
| `shift+enter` | Newline |
| `up/down` | Scroll |
| `/` | Slash command |

### Prefix commands (parsed from tmux.conf)
| Key | tmux default | Action |
|-----|--------------|--------|
| `{prefix} c` | `c` | New tab (create/select task) |
| `{prefix} n` | `n` | Next tab |
| `{prefix} p` | `p` | Previous tab |
| `{prefix} 0-9` | `0-9` | Jump to tab |
| `{prefix} %` | `%` | Split vertical |
| `{prefix} "` | `"` | Split horizontal |
| `{prefix} o` | `o` | Next pane |
| `{prefix} ;` | `;` | Previous pane |
| `{prefix} h/j/k/l` | (user-defined) | Directional pane focus |
| `{prefix} x` | `x` | Close pane |
| `{prefix} &` | `&` | Close tab |
| `{prefix} d` | `d` | Detach |
| `{prefix} t` | `t` | Terminal into sandbox |
| `{prefix} !` | (custom) | Notifications |
| `{prefix} w` | (custom) | Open in web browser |

### Spec Review
| Key | Action |
|-----|--------|
| `shift+V` | Visual line select |
| `c` | Comment on selection |
| `a` | Approve |
| `r` | Request changes |

## Architecture

### File structure

```
api/pkg/cli/tui/
├── cmd.go              CLI entry (helix tui, subcommands)
├── app.go              Top-level model: tabs → panes → views
├── api.go              API client wrapper (reuses api/pkg/types)
├── state.go            Serialize/restore layout to disk
│
├── kanban.go           Kanban board view
├── chat.go             Chat conversation renderer
├── chat_input.go       Input area (independent of render cycle)
├── chat_spinner.go     British spinner + token counter
├── chat_tools.go       Tool call rendering (diffs, file reads, etc.)
├── review.go           Spec review UI (visual select, comments)
├── newtask.go          New task creation modal
├── picker.go           Project picker
├── taskpicker.go       Task picker for splits/tabs
├── slash.go            Slash command handler
├── mcp.go              MCP server management UI
├── notifications.go    Notification list + badge
│
├── pane.go             Binary tree pane manager
├── tabs.go             Tab bar (bottom, tmux-style)
├── terminal.go         Embedded terminal (tmux-in-sandbox)
│
├── connection.go       Connection state, retry, mosh-style bar
├── outbox.go           Reliable message queue (send-on-connect)
├── predict.go          Predictive local echo
│
├── tmux.go             Parse ~/.tmux.conf for keybindings
├── styles.go           Claude Code aesthetic (colors, borders)
├── diff.go             Red/green diff renderer
├── markdown.go         Terminal markdown renderer
├── utils.go            Shared helpers (truncate, timeAgo, etc.)
│
└── *_test.go           Tests for each component
```

### Testability

Each component is designed to be independently testable:

| Component | Test approach |
|-----------|--------------|
| `tmux.go` | Unit test: parse sample configs, verify bindings |
| `pane.go` | Unit test: split, focus, close, serialize |
| `tabs.go` | Unit test: create, switch, close, render |
| `diff.go` | Unit test: parse unified diff, render with colors |
| `markdown.go` | Unit test: render markdown subsets |
| `outbox.go` | Unit test: queue, retry, flush, dedup |
| `connection.go` | Unit test: state machine (connected/disconnected/reconnecting) |
| `predict.go` | Unit test: predict, confirm, rollback |
| `kanban.go` | Integration test: mock API, verify column sorting |
| `chat.go` | Integration test: mock API, verify message rendering |
| `review.go` | Integration test: visual select, comment flow |
| `app.go` | E2E test: full flow with mock API server |

### Connection state machine

```
                    ┌──────────┐
           ┌──────►│ Connected │◄────────────┐
           │       └─────┬─────┘             │
           │             │ timeout/error      │ API responds
           │             ▼                    │
           │       ┌──────────────┐          │
           │       │ Disconnected │──────────┘
           │       │ (show bar)   │  retry
           │       └──────┬───────┘
           │              │ user quits
           │              ▼
           │        ┌──────────┐
           └────────│  Exited  │
                    └──────────┘
```

While disconnected:
- Input works (local buffer + outbox)
- Spinner shows "reconnecting..." instead of British verbs
- Bar shows elapsed time since last contact
- On reconnect: flush outbox, fetch notifications, refresh active views

### API Endpoints Used

| Endpoint | Purpose |
|----------|---------|
| `GET /projects` | Project picker |
| `GET /spec-tasks?project_id=X` | Kanban board |
| `GET /spec-tasks/{id}` | Task detail |
| `GET /sessions/{id}/interactions` | Chat history |
| `POST /sessions/chat` | Send message |
| `POST /spec-tasks/from-prompt` | Create task |
| `POST /spec-tasks/{id}/start-planning` | Start planning |
| `POST /spec-tasks/{id}/approve-specs` | Approve specs |
| `GET /spec-tasks/{id}/design-reviews` | List reviews |
| `POST /spec-tasks/{id}/design-reviews/{id}/comments` | Add comment |
| `GET /sessions/{id}/rdp-connection` | Terminal connection |
| `GET /projects/{id}/tasks-progress` | Batch progress |
| Backend notification endpoint | Poll for notifications |

### Data model (types reuse)

All from `api/pkg/types` — no duplication:
- `types.SpecTask` — kanban cards, tabs
- `types.Interaction` — chat messages
- `types.SessionChatRequest` — sending messages
- `types.CreateTaskRequest` — new tasks
- `types.Project` — project picker
- `types.DesignReview` / `types.DesignReviewComment` — spec review

## Implementation Plan

### Phase 1: Core (DONE)
1. ✅ `styles.go`, `tmux.go`, `api.go`, `picker.go`
2. ✅ `kanban.go`, `chat.go`, `pane.go`, `app.go`, `cmd.go`
3. ✅ `state.go`, `newtask.go`, `taskpicker.go`, `utils.go`
4. ✅ Registered in root.go, bubbletea deps in go.mod
5. ✅ Compiles and builds successfully

### Phase 2: Claude Code UX clone
6. `chat_input.go` — independent input renderer (no flicker)
7. `chat_spinner.go` — British verb spinner with token counter
8. `chat_tools.go` — tool call rendering (diffs, reads, commands)
9. `diff.go` — red/green unified diff renderer
10. `markdown.go` — terminal markdown renderer

### Phase 3: Tabs + Review
11. `tabs.go` — bottom tab bar
12. `review.go` — spec review with visual select + comments
13. `slash.go` — slash command handler
14. Integrate tabs into app.go

### Phase 4: Connection resilience
15. `connection.go` — connection state machine + mosh bar
16. `outbox.go` — reliable message queue
17. `predict.go` — predictive local echo

### Phase 5: Terminal + MCP
18. `terminal.go` — embedded terminal via tmux-in-sandbox
19. `mcp.go` — MCP server management UI
20. `notifications.go` — notification polling + badge

### Phase 6: Web integration
21. Embed TUI as a terminal component in the web interface
22. Shared rendering protocol between TUI and web xterm.js

## Web interface embedding

The TUI can also be embedded in the web interface via xterm.js. This means:
- Users can choose TUI or GUI on the same page
- The terminal component renders the same output as the standalone TUI
- Implementation: the TUI process runs server-side, connected via WebSocket
  to xterm.js in the browser
- Same code, same rendering, different transport
