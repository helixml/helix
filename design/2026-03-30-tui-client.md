# Helix TUI Client (hmux)

**Date:** 2026-03-30
**Updated:** 2026-03-31
**Branch:** `feature/tui-client`
**Status:** Built and functional

## What is hmux

hmux (Helix agent multiplexer) is a full terminal-native client for Helix. It provides a complete interface for managing spec tasks, chatting with agents, reviewing specs, and navigating projects -- all without a browser. It works well over SSH, mosh, and satellite connections where the web UI is unusable.

The name and interaction model are inspired by tmux: prefix-key command dispatch, split panes, tabs, detach/reattach. Keybindings are parsed from the user's `~/.tmux.conf` automatically.

### Invocation

```bash
helix hmux                    # start with org picker
helix hmux --project proj_x   # skip picker, go to kanban
helix hmux attach             # reattach to previous session
helix hmux demo               # explore with mock data (no Helix instance needed)
helix hmux serve              # start SSH server mode
```

Registered in `api/pkg/cli/tui/cmd.go` as both `hmux` and `tui` aliases.

## UX Flow

### 1. Org picker (ModeOrgPicker)

If the user has multiple organizations, the TUI starts here. Single-org users auto-advance. Navigate with j/k, select with enter.

### 2. Project picker (ModePicker)

Lists all projects in the selected org. Pinned projects sort to the top (separated by a visual divider). Supports:
- `j/k` to navigate, `enter` to select
- `p` to pin/unpin projects (calls `POST /projects/{id}/pin`)
- `n` to create a new project inline (two-step form: name, then optional GitHub URL)
- `esc` to go back to orgs
- `g/G` for top/bottom

If only one project exists, it auto-selects.

### 3. Kanban board (ModeMain, tab 0)

Full-screen kanban with 5 columns: Backlog, Planning, In Progress, Review, Done. Tasks are sorted into columns by mapping `SpecTaskStatus` values to columns in `statusToColumn()`.

Each column is a bordered box with color-coded headers. The focused column has a brighter border. Tasks show their `ShortTitle` or `UserShortTitle`, truncated to fit.

Navigation:
- `h/l` or arrows: column left/right
- `j/k` or arrows: task up/down within column
- `1-5`: jump to column by number
- `ctrl+d/ctrl+u`: half-page scroll
- `enter`: open task (behavior varies by status -- backlog tasks trigger planning, spec_review tasks open review, others open chat)
- `n`: new task modal
- `r`: refresh
- `esc`: back to project picker

Columns scroll independently with `ensureVisible()` keeping the cursor in view. Scroll indicators ("N above" / "N below") appear when content overflows.

### 4. Chat view (task detail)

Opening a task creates a new tab with a `ChatModel`. The view has three sections:

**Header:** Project name, task name, status, and branch name, joined by dim separators.

**Messages area:** Scrollable viewport rendering all interactions. Each interaction shows the user prompt (labeled "You" in blue) and the assistant response. Responses are rendered from `ResponseEntries` (structured Zed WebSocket sync entries) when available, falling back to `ResponseMessage` for legacy interactions.

**Input area:** Claude Code-style input with `>` prompt, visible cursor (reverse-video on character, block at end), and status line showing `enter: send  ctrl+enter: interrupt`.

Key input bindings:
- `enter`: send message (queues behind current work)
- `ctrl+enter`: send message (interrupts current agent work)
- `shift+enter`: newline (multiline input)
- `esc`: clear input -> stop agent -> go back to kanban (progressive)
- `up/down`: prompt history navigation
- `ctrl+o`: toggle expanded tool call details
- `ctrl+d/ctrl+u`: half-page scroll
- `pgup/pgdown` (and shift/alt/ctrl+up/down): page scroll
- `ctrl+a/ctrl+e`: home/end
- `ctrl+u/ctrl+k`: delete to start/end of line
- `ctrl+w`: delete word
- `ctrl+l`: clear and re-render screen

### 5. Tabs (tmux-style, bottom bar)

Tab 0 is always kanban (cannot be closed). Each opened task gets its own tab. The tab bar renders at the bottom with tmux-style numbering (`0:kanban  1:Fix login*  2:Refactor DB`). Active tab is highlighted with a bold purple background. Unread notification badges show as `(N)`.

Tab operations (all require prefix key):
- `{prefix} c`: new tab (opens blank chat -- first message creates spec task)
- `{prefix} n`: next tab
- `{prefix} p`: previous tab
- `{prefix} 0-9`: jump to tab by number
- `{prefix} &`: close tab

Also: `ctrl+left/ctrl+right` switch tabs without prefix.

### 6. Split panes (tmux-style)

Each tab has its own `PaneManager` containing a binary tree (`PaneTree`) of chat views. Panes split in any direction with configurable ratios. Vertical splits draw a `|` divider; horizontal splits draw a `---` divider. Single panes render without borders.

Pane operations (all require prefix key):
- `{prefix} %` (or configured `SplitV`): split vertical -- opens task picker
- `{prefix} "` (or configured `SplitH`): split horizontal -- opens task picker
- `{prefix} o` (or configured `PaneNext`): focus next pane
- `{prefix} ;` (or configured `PanePrev`): focus previous pane
- `{prefix} x` (or configured `ClosePane`): close focused pane (if last pane, closes tab)
- `{prefix} d` (or configured `Detach`): save state and quit
- Arrow keys after prefix: directional pane navigation
- Custom `h/j/k/l` pane bindings from tmux.conf

The task picker is a centered modal overlay with type-to-filter search. The first option is always "+ New task" which creates a blank chat pane.

### 7. Spec review

For tasks in `spec_review` status, a `ReviewModel` renders the requirements spec, technical design, and implementation plan with line numbers and markdown formatting.

Three modes:
- **Viewing:** scroll with j/k, jump with g/G. Lines are styled: `##` headers bold, `- [ ]` checkboxes rendered as unicode, cursor line highlighted.
- **Selecting (shift+V):** visual line selection. Extend selection with j/k. Selected lines get a blue background.
- **Commenting (c or enter from selection):** opens inline comment input.

Actions:
- `a`: approve specs (calls `POST /spec-tasks/{id}/approve-specs`)
- `r`: request changes (opens comment input for the entire spec)
- `shift+V` then `c`: comment on selected lines

### 8. Slash commands

Typing `/` in the input area shows a completion menu. Implemented commands:

| Command | Description |
|---------|-------------|
| `/mcp` | Configure MCP servers for this project |
| `/model` | Switch the model for this task |
| `/approve` | Approve the current spec/implementation |
| `/reject` | Request changes |
| `/branch` | Show/change the working branch |
| `/pr` | Show pull request status |
| `/logs` | Show agent logs |
| `/web` | Open in web browser |
| `/status` | Show task status details |
| `/help` | Show all commands |

The `/web` and `/status` commands are functional. Others are registered but have TODO handlers.

### 9. MCP server management

The `/mcp` command opens a centered modal listing configured MCP servers with status icons (green check for connected, red X for disconnected) and tool counts. Keys: `j/k` navigate, `a/r/e` for add/remove/edit (TODO), `esc` to close.

Currently renders with mock data; API integration is stubbed.

### 10. Notifications

`NotificationManager` tracks notifications with types: SpecReady, ImplComplete, PRCreated, AgentError, TaskStatusChange, Info. The notification list renders as a centered modal overlay with unread dots, age labels, and type-specific icons.

Access: `{prefix} !` toggles the list. Keys: `j/k` navigate, `enter` go to task, `m` mark all read, `esc` close.

Notifications are tracked in-memory; backend polling is wired but notification endpoints are not yet connected.

## Keybindings and tmux.conf Parsing

### tmux.conf parser (`tmux.go`)

On startup, `LoadTmuxConfig()` searches `~/.tmux.conf` and `~/.config/tmux/tmux.conf`. It parses:

- `set -g prefix C-a` -> converts `C-a` to `ctrl+a` via `tmuxKeyToTeaKey()`
- `bind X split-window -h` -> maps to `SplitV` (tmux `-h` = vertical split)
- `bind X split-window` (no flag) -> maps to `SplitH`
- `bind X select-pane -L/-D/-R/-U` -> maps to directional pane navigation
- `bind X kill-pane` -> maps to `ClosePane`
- `bind X detach-client` -> maps to `Detach`

Supports `-r` (repeat) flag. Falls back to tmux defaults if no config found.

`IsInTmux()` detects when running inside tmux (checks `$TMUX` env var) and prints a warning about prefix conflicts.

### Complete keybinding table

| Context | Key | Action |
|---------|-----|--------|
| Global | `ctrl+c` x2 | Quit (double-tap within 1s) |
| Global | `ctrl+c` x1 | Stop agent / clear input / "press again to quit" |
| Global | `ctrl+d` | Quit (from chat with empty input at bottom), else page-down |
| Global | `ctrl+l` | Clear and re-render |
| Global | `ctrl+left/right` | Switch tabs (no prefix) |
| Org picker | `j/k` | Navigate |
| Org picker | `enter` | Select |
| Org picker | `q` | Quit |
| Project picker | `j/k` | Navigate |
| Project picker | `enter` | Select |
| Project picker | `n` | New project |
| Project picker | `p` | Pin/unpin |
| Project picker | `g/G` | Top/bottom |
| Project picker | `esc` | Back to orgs |
| Project picker | `q` | Quit |
| Kanban | `h/l` or arrows | Column left/right |
| Kanban | `j/k` or arrows | Task up/down |
| Kanban | `1-5` | Jump to column |
| Kanban | `ctrl+d/ctrl+u` | Half-page scroll |
| Kanban | `enter` | Open task (action varies by status) |
| Kanban | `n` | New task |
| Kanban | `r` | Refresh |
| Kanban | `esc` | Back to projects |
| Kanban | `q` | Save state and quit |
| Chat | `enter` | Send (queue behind agent) |
| Chat | `ctrl+enter` | Send (interrupt agent) |
| Chat | `shift+enter` | Newline |
| Chat | `esc` | Clear input -> stop agent -> back to kanban |
| Chat | `up/down` | Prompt history |
| Chat | `ctrl+o` | Toggle tool call detail expansion |
| Chat | `pgup/pgdown` | Page scroll |
| Chat | `ctrl+d/ctrl+u` | Half-page scroll |
| Chat | `ctrl+a/ctrl+e` | Home/end |
| Chat | `ctrl+u/ctrl+k` | Delete to start/end |
| Chat | `ctrl+w` | Delete word |
| Chat | `/` | Slash command |
| Review | `j/k` | Scroll |
| Review | `g/G` | Top/bottom |
| Review | `shift+V` | Start visual line selection |
| Review (selecting) | `j/k` | Extend selection |
| Review (selecting) | `c` or `enter` | Comment on selection |
| Review (selecting) | `esc` | Cancel selection |
| Review | `a` | Approve specs |
| Review | `r` | Request changes |
| Prefix commands | `{prefix} c` | New tab |
| Prefix commands | `{prefix} n/p` | Next/prev tab |
| Prefix commands | `{prefix} 0-9` | Jump to tab |
| Prefix commands | `{prefix} &` | Close tab |
| Prefix commands | `{prefix} %` | Split vertical (task picker) |
| Prefix commands | `{prefix} "` | Split horizontal (task picker) |
| Prefix commands | `{prefix} o` | Next pane |
| Prefix commands | `{prefix} ;` | Previous pane |
| Prefix commands | `{prefix} x` | Close pane |
| Prefix commands | `{prefix} d` | Detach (save + quit) |
| Prefix commands | `{prefix} t` | Terminal (TODO) |
| Prefix commands | `{prefix} !` | Toggle notifications |
| Prefix commands | `{prefix} w` | Open in web browser |

## Architecture

### File structure

```
api/pkg/cli/tui/
  cmd.go              CLI entry (cobra commands: hmux, attach, demo, serve)
  app.go              Top-level bubbletea model (mode dispatch, tab/pane management)
  api.go              API client wrapper (ListProjects, ListSpecTasks, SyncPromptHistory, etc.)
  state.go            Serialize/restore layout to ~/.helix/tui/state.json

  orgpicker.go        Organization picker view
  picker.go           Project picker (pin/unpin, create new, sort pinned first)
  kanban.go           Kanban board (5 columns, per-column scroll, status mapping)
  chat.go             Chat view (interaction rendering, scrolling, agent state)
  chat_input.go       Independent input model (cursor, history, multiline)
  chat_spinner.go     British humor spinner + tips
  chat_tools.go       Tool call rendering (ToolCallRenderer, dispatch by function name)
  review.go           Spec review (viewing, visual select, commenting, approve)
  newtask.go          New task creation modal
  taskpicker.go       Task picker modal (type-to-filter, split direction)
  slash.go            Slash command registry + completion rendering
  mcp.go              MCP server management modal
  notifications.go    Notification manager + modal overlay
  prompt_queue.go     Visible prompt queue UI (pending/sending/editing states)

  tabs.go             Tab bar (bottom, tmux-style numbering + unread badges)
  pane.go             Binary tree pane manager (split, focus, close, render)
  terminal.go         Embedded terminal model (scaffold, WebSocket to sandbox tmux)

  connection.go       Connection state machine + mosh-style disconnect bar
  outbox.go           Reliable message queue (enqueue, retry, cleanup)
  predict.go          Predictive local echo (confirm, rollback, render)

  tmux.go             Parse ~/.tmux.conf for keybindings
  styles.go           Color palette + reusable lipgloss styles
  diff.go             Red/green diff renderer (unified + inline)
  markdown.go         Terminal markdown renderer (headers, code blocks, lists, etc.)
  utils.go            Shared helpers (truncate, timeAgo)

  demo.go             Mock Helix API server with realistic fake data

  diff_test.go
  markdown_test.go
  notifications_test.go
  outbox_test.go
  pane_test.go
  predict_test.go
  prompt_queue_test.go
  prompt_sync_test.go
  slash_test.go
  tmux_test.go
  integration_test.go

  e2e-test/
    Dockerfile          Ubuntu 25.10 + Xvfb + Vulkan for headless Zed
    run_docker_e2e.sh   Build binaries + Docker image + run
    run_e2e.sh          Test script (inside container)
    tui-test-server/    Go test server (real HelixAPIServer + memorystore)
      main.go
      go.mod
      tui_e2e_test.go   Go E2E test driving the App model
    zed-binary          Pre-built Zed binary (copied from helix/zed-build/)
    helix-binary        Pre-built helix CLI binary
    tui-e2e-test        Compiled Go test binary
```

### Key dependencies

- `github.com/charmbracelet/bubbletea` -- TUI framework (Elm architecture)
- `github.com/charmbracelet/lipgloss` -- terminal styling
- `github.com/charmbracelet/wish` + `github.com/charmbracelet/ssh` -- SSH server mode
- `github.com/helixml/helix/api/pkg/types` -- all types shared with backend
- `github.com/helixml/helix/api/pkg/client` -- HTTP client for Helix API
- `github.com/helixml/helix/api/pkg/server/wsprotocol` -- ResponseEntry types from Zed sync

## Prompt History Sync

Prompts are delivered through the `PromptHistorySyncRequest` API rather than the older `SessionChatRequest` path. This supports the queue-and-interrupt model:

### enter vs ctrl+enter

- **enter** queues the prompt behind any in-flight agent work (`interrupt: false`). The prompt is sent to `POST /prompt-history/sync` with `status: "pending"`. The backend processes it in order.
- **ctrl+enter** interrupts the current agent work (`interrupt: true`). The prompt is sent with the interrupt flag, causing the backend to cancel the current work and process this prompt immediately.

### Flow

1. User types and presses enter/ctrl+enter
2. `sendPrompt()` generates a client-side ID (`tui_<timestamp>`)
3. Prompt is enqueued in the local `Outbox` (survives disconnects)
4. `PromptHistorySyncRequest` is sent to the server with the entry
5. On success, `outbox.MarkSent(promptID)` removes it from the queue
6. On failure, the outbox retries (up to 3 attempts)
7. Chat enters spinner state and polls interactions every 2 seconds

For new tasks (no task ID), the first message creates the spec task via `POST /spec-tasks/from-prompt`, then the chat model updates with the real task ID, session ID, and app ID.

## Prompt Queue UI

`PromptQueue` (`prompt_queue.go`) manages a visible queue of pending prompts above the input bar. Each prompt shows a status icon:
- `○` pending
- `◉` sending (blue)
- `✎` editing (orange)

Interrupts are sorted before non-interrupts. The queue supports:
- `Add()`: insert prompt (interrupts go before non-interrupts)
- `ToggleInterrupt()`: toggle a prompt's priority and re-sort
- `StartEdit()` / `FinishEdit()`: edit queued prompts (pauses sending)
- `Remove()`: delete a queued prompt
- `MarkSent()`: remove after successful delivery

When editing is in progress (`IsPaused()`), the queue stops sending to prevent race conditions.

## Tool Call Rendering

Response entries from the Zed WebSocket sync protocol are rendered by type:

### Text entries
Plain text with thinking tag collapsing. `<thinking>...</thinking>` and `<think>...</think>` blocks are stripped entirely from the displayed text.

### Tool call entries
Each tool call shows a `✽` icon, the tool name, and a status indicator:
- `✓` green for Completed
- `⟳` orange for Running/In Progress
- `✗` red for Error/Failed

Content rendering depends on tool type and the `showDetails` toggle (`ctrl+o`):

**Diffs are always expanded.** The chat renderer checks `hasDiffLines()` for `+/-` prefixed lines. When found, added lines get green background, removed lines get red background (matching Claude Code's colors). The `diff.go` renderer draws bordered boxes with `┌─ filename ─┐` headers.

**Other tool calls are compact by default.** File reads show as one line (`✽ Read src/auth/login.go (lines 1-45)`). Commands show the command text and first 5 lines of output. Searches show the query and match count. Unknown tools show the function name with the first string argument.

**ctrl+o toggles detail expansion** for non-diff content. When toggled on, all tool call content is shown.

The `ToolCallRenderer` (`chat_tools.go`) dispatches by function name: `edit_file`/`str_replace_editor` -> diff rendering, `write_file`/`create_file` -> creation summary, `read_file`/`view_file` -> compact read line, `bash`/`execute_command` -> command + output, `search`/`grep` -> search summary, and a generic fallback.

The `diff.go` module provides both `RenderDiff()` (unified diff format) and `RenderInlineDiff()` (old/new text pairs), both with bordered boxes and colored +/- lines.

## Thinking Tag Collapsing

`collapseThinkingTags()` in `chat.go` strips `<thinking>...</thinking>` and `<think>...</think>` blocks from response text. Handles both closed and unclosed tags (strips to end of string for unclosed). The result is trimmed of whitespace.

## Connection Resilience

### Mosh-style disconnect bar (`connection.go`)

`ConnectionManager` tracks API health with a state machine:
- Starts `ConnConnected`
- After 2+ consecutive API failures (`RecordFailure()`), transitions to `ConnDisconnected`
- Any successful API call (`RecordSuccess()`) transitions back to `ConnConnected`

When disconnected, a mosh-style bar renders at the top of the screen:
```
──────────────────────────────────────────────────
 helix: Last contact 2:08 ago. [To quit: Ctrl-^ .]
──────────────────────────────────────────────────
```
Orange background, white text, centered, full width. Time format: seconds, then M:SS, then H:MM:SS.

The bar is suppressed in SSH server mode (`isSSHSession = true`) since the transport handles reconnection.

### Outbox (`outbox.go`)

Messages are queued in a thread-safe `Outbox` before being sent:
- `Enqueue()`: adds to queue with `OutboxPending` status
- `NextPending()`: returns the next unsent message
- `MarkSending()`: increments attempt counter
- `MarkSent()`: marks successful delivery
- `MarkFailed()`: requeues (up to 3 attempts) or marks `OutboxFailed`
- `Cleanup()`: removes old sent entries

`PendingCount()` returns the number of unsent messages for UI display.

### Predictive local echo (`predict.go`)

`PredictiveEcho` implements mosh-style local echo:
- `AddPredicted()`: appends to the predicted buffer on each keystroke
- `RemovePredicted()`: handles backspace
- `Confirm()`: called when the server acknowledges text. Three cases:
  1. Perfect prediction: everything confirmed, predicted buffer cleared
  2. Server confirmed a prefix: remaining text stays predicted
  3. Server diverged: accept server text, clear predictions
- `Rollback()`: clears all predictions (server rejected)
- `RenderInput()`: renders confirmed text in normal style, predicted text in dim style

Thread-safe (mutex-protected). Togglable via `SetEnabled()`.

## SSH Server Mode

`helix hmux serve` starts an SSH server using charmbracelet/wish:

```bash
ssh -p 2222 helix@your-host  # password = API key
```

Configuration:
- `--port` (default 2222)
- `--host` (default 0.0.0.0)
- Host keys stored at `~/.helix/tui/ssh/host_key` (auto-generated)

Authentication:
- Password auth: username must be `helix`, password is treated as the API key
- Public key auth: accepts all keys (API key mapping TODO)

Each SSH session spawns an independent `App` model with its own `APIClient`. The connection manager's `isSSHSession` flag is set to suppress the disconnect bar.

Graceful shutdown on SIGINT/SIGTERM with 5-second timeout.

## E2E Test Infrastructure

### Architecture

The E2E test runs a real Zed binary against a Go test server that imports production Helix server code, validating the full message flow:

```
TUI test driver --HTTP--> tui-test-server --WebSocket--> Zed --LLM--> response
                                                                         |
TUI test driver <--HTTP-- tui-test-server <--WebSocket-- Zed <-----------'
```

### Components

**tui-test-server** (`e2e-test/tui-test-server/main.go`): Runs `server.NewTestServer()` with `memorystore.New()` and `pubsub.NewNoop()`. Serves both REST API endpoints (for the TUI) and the WebSocket sync endpoint (for Zed). Seeds a session, registers a sync event hook for logging `agent_ready`, `thread_created`, `message_added`, and `message_completed` events.

**Zed binary**: Pre-built from `helix/zed-build/`. Runs headless under Xvfb with software Vulkan rendering (`mesa-vulkan-drivers`). Connects to the test server via WebSocket.

**Helix binary**: Pre-built with TUI support. Used for Phase 3 (binary validation).

**Go E2E test binary** (`tui-e2e-test`): Compiled test binary that drives the actual `App` model against the running test server.

### Docker image

Ubuntu 25.10 base with Xvfb, D-Bus, mesa Vulkan drivers, and test tooling. The Dockerfile copies pre-built binaries and the test script.

### Test phases (run_e2e.sh)

1. **Send chat message**: POST to `/sessions/chat`, verify non-empty response
2. **Verify interactions**: GET `/sessions/{id}/interactions`, verify stored count > 0
3. **TUI rendering**: verify `helix tui --help` runs successfully
4. **Server state**: check completions count from `/status` endpoint
5. **Go E2E test**: run `tui-e2e-test` binary driving the App model against the running server

### Running locally

```bash
cd api/pkg/cli/tui/e2e-test
# Ensure zed-binary exists (from ./stack build-zed release)
cp ~/pm/helix/zed-build/zed ./zed-binary

./run_docker_e2e.sh              # full build + run
./run_docker_e2e.sh --no-build   # skip builds, reuse binaries
```

The script sources `ANTHROPIC_API_KEY` from `helix/.env` or `helix/.env.usercreds`.

### Integration tests (no Docker)

`integration_test.go` runs against a test harness with `server.NewTestServer()` + `memorystore`. Tests:
- `TestIntegration_ListProjects`: API client returns seeded projects
- `TestIntegration_ListSpecTasks`: tasks sort correctly into kanban columns
- `TestIntegration_ListInteractions`: interactions with ResponseEntries load correctly
- `TestIntegration_ChatRender`: ChatModel renders "You" and "Assistant" labels
- `TestIntegration_KanbanRender`: KanbanModel renders column headers and task names
- `TestIntegration_SpinnerBritishVerbs`: spinner contains a British verb and tip
- `TestIntegration_DiffRendering`: inline diff contains filename and +/- lines
- `TestIntegration_ConnectionManager`: state machine transitions (connected -> disconnected -> connected)
- `TestIntegration_TabBar`: create, navigate, find by task, close

### Unit tests

| File | Coverage |
|------|----------|
| `tmux_test.go` | Config parsing, key conversion |
| `pane_test.go` | Split, focus, close, tree operations |
| `diff_test.go` | Unified and inline diff rendering |
| `markdown_test.go` | Header, code block, inline formatting |
| `outbox_test.go` | Enqueue, retry, failure, cleanup |
| `predict_test.go` | Predict, confirm, rollback, divergence |
| `prompt_queue_test.go` | Add, toggle interrupt, edit, pause |
| `prompt_sync_test.go` | Sync API calls, failure handling, mock server |
| `slash_test.go` | Command matching, parsing |
| `notifications_test.go` | Add, unread count, mark read |

## Production Bug Fix: sendChatMessageToExternalAgent

The multi-agent E2E tests (design doc `2026-03-20-multi-agent-e2e-tests.md`) uncovered that `sendChatMessageToExternalAgent` was the only code path sending `chat_message` commands without `agent_name` in the data. For Claude Code sessions, Zed received `agent_name=null`, defaulted to `NativeAgent`, and tried loading the thread from local SQLite instead of via Claude Code's ACP agent.

**Fix (PR #1967):** Added `agent_name` (from `getAgentNameForSession`) to the command data in `sendChatMessageToExternalAgent`. Also added `agent_name` to `sendOpenThread`.

The TUI E2E test server exercises this code path: when `/sessions/chat` is called, it calls `srv.SendChatMessage()` which goes through the production `sendChatMessageToExternalAgent` path, then sends via WebSocket to Zed. This validates that the agent routing fix works end-to-end.

Also added to `requestToSessionMapping` tracking, which was missing -- required for the test server to correctly route Zed's responses back to the originating session.

## Demo Mode

`helix hmux demo` starts the TUI against an in-process mock HTTP server with no external dependencies. The mock server (`demo.go`) provides:

### Seeded data
- 1 organization ("Acme Corp")
- 2 projects ("acme-webapp" with 7 tasks, "acme-mobile" with 3 tasks)
- 7 spec tasks across all kanban columns (backlog, planning, in-progress, review, done)
- 3 pre-populated chat sessions with realistic ResponseEntries (tool calls, diffs, code)

### Streaming simulation
When the user sends a message, `streamResponse()` simulates a Zed agent:
1. Creates an interaction in `InteractionStateWaiting` immediately (so the TUI shows the spinner)
2. In a background goroutine, generates tool calls and text entries contextually:
   - Messages containing "test" -> run command with test results
   - Messages containing "fix"/"bug" -> search, read, edit, run pattern
   - Messages containing "status" -> git log, git diff, summary
   - Default -> generic investigation pattern
3. Each tool call starts as "Running", transitions to "Completed" after a delay
4. Text is streamed word-by-word with partial updates
5. Interaction state transitions to `InteractionStateComplete` when done

The TUI polls interactions every 2 seconds and sees the streaming updates naturally.

## Interaction Polling

The TUI uses a poll-based approach for agent responses (no WebSocket from TUI to API):

### Tick-based polling
- `TickMsg` fires every 10 seconds (`pollInterval`)
- On tick, if on the kanban tab, `fetchTasks()` refreshes the board
- The connection manager uses API responses to track health

### Chat polling
- When a message is sent (`chatResponseMsg`), the chat enters `agentBusy` state
- `pollInteractions()` fetches `GET /sessions/{id}/interactions` every 2 seconds
- If the latest interaction is in `InteractionStateWaiting`, the spinner continues and polling re-fires
- If the latest interaction is `InteractionStateComplete`, the spinner stops and `agentBusy` clears
- The `userStopped` flag prevents the spinner from restarting after the user pressed esc to stop

## State Persistence (Detach/Reattach)

### Save state
`saveState()` is called on:
- `{prefix} d` (detach)
- `q` from kanban
- Double `ctrl+c` quit

It serializes to `~/.helix/tui/state.json`:
```json
{
  "project_id": "proj_xxx",
  "panes": {
    "dir": "vertical",
    "left": {"task_id": "spt_1"},
    "right": {"task_id": "spt_2"}
  },
  "focused_task_id": "spt_1"
}
```

`BuildStateFromApp()` collects task IDs from all open tabs and builds a simple pane tree. The pane tree is a recursive structure (`PaneState`) with leaf nodes containing `task_id` and split nodes containing `dir`, `left`, and `right`.

### Restore state
On startup, `NewApp()` checks for saved state. If found with a valid `project_id`, it skips the org/project picker and goes straight to kanban. If the state has panes, `pendingRestore` is set and restoration happens after tasks load:

1. `collectTaskIDs()` extracts all task IDs from the pane tree
2. Each task is fetched via `GetSpecTask()`
3. `applyRestoredPanes()` opens each task in a new tab

`helix hmux attach` explicitly loads saved state and fails with an error if none exists.

### State file location
`~/.helix/tui/state.json` (created with mode 0600).

## Spinner

The spinner (`chat_spinner.go`) displays a rotating flower/asterisk character (`✽ ✦ ✽ ✧`) with a randomly selected British verb that rotates every 15 seconds:

```
✽ Complaining about the weather... (59s)
  ⎿  Tip: Use ctrl+enter to interrupt the agent with a new message
```

18 verbs including: "Waiting for kettle", "Taking afternoon tea", "Complaining about the weather", "Going to the pub", "Having a biscuit", "Queueing politely", "Buttering a scone", "Minding the gap".

Tips are randomly selected from 6 options and reference the actual tmux prefix (dynamically substituted from `{prefix}`).

## Embedded Terminal (scaffold)

`terminal.go` provides a `TerminalModel` scaffold for embedding terminal panes connected to sandbox containers. Architecture:

```
User terminal <-> TerminalModel <-> WebSocket <-> sandbox container tmux
```

The model has a ring buffer for terminal output (1000 lines), connection state tracking, and basic local echo. The WebSocket connection to the sandbox is not yet implemented (TODO comments in place). Accessible via `{prefix} t`.

## API Endpoints Used

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/organizations` | GET | Org picker |
| `/projects` | GET | Project list (with org filter) |
| `/projects/{id}` | GET | Project details |
| `/projects/{id}/pin` | POST/DELETE | Pin/unpin project |
| `/projects` | POST | Create project |
| `/spec-tasks?project_id=X` | GET | Kanban board |
| `/spec-tasks/{id}` | GET | Task detail (for restore) |
| `/spec-tasks/from-prompt` | POST | Create task from first message |
| `/spec-tasks/{id}/start-planning` | POST | Start planning (backlog -> planning) |
| `/spec-tasks/{id}/approve-specs` | POST | Approve specs |
| `/spec-tasks/{id}/stop-agent` | POST | Stop agent |
| `/sessions/{id}/interactions` | GET | Chat history |
| `/prompt-history/sync` | POST | Queue prompt (enter/ctrl+enter) |
| `/status` | GET | User status (pinned projects) |

## Visual Design

Colors are defined in `styles.go` using a muted palette inspired by Claude Code:
- Primary: muted blue-purple (63)
- Text: light gray (252)
- Borders: dark (238) unfocused, blue-purple (63) focused
- Roles: blue (111) for "You", purple (183) for "Assistant"
- Diffs: green bg (22) for adds, red bg (52) for removes
- Status: gray backlog, blue planning, green active, orange review, gray done
- Priority: gray low, warm white medium, orange high, red critical

All rendering uses lipgloss styles. The tab bar uses purple (57) for active and dark gray (236) for inactive.
