# Helix TUI Client

**Date:** 2026-03-30
**Branch:** `feature/tui-client`
**Status:** Design

## Problem

The Helix web UI requires a browser with reasonable bandwidth. Over satellite internet, SSH/mosh connections, or air-gapped environments, the browser UI is unusable. We need a terminal-native way to:

1. View and navigate the kanban board
2. Chat with spec task agents
3. View multiple spec task conversations simultaneously
4. Create new tasks

## Design

### Technology

- **Bubble Tea** (charmbracelet/bubbletea) — the standard Go TUI framework
- **Lip Gloss** (charmbracelet/lipgloss) — styling
- **Bubbles** (charmbracelet/bubbles) — text input, viewport, spinner components
- Lives in `api/pkg/cli/tui/`, invoked via `helix tui` (alias: `helix ui`)

### UX Philosophy

Mirror Claude Code's look and feel as closely as possible:
- Muted color palette, clean borders, information-dense
- Vim-style navigation (j/k/h/l)
- **esc** stops the running agent (same as Claude Code), does NOT navigate
- Input prompt at the bottom of chat panes
- Compact, no wasted space

### Views

#### 1. Kanban Board (home view)

Full-screen kanban board showing spec tasks in columns by status.

```
╭─ helix tui ─────────────────────────────────────────────────────────╮
│ myproject                                                      r:⟳ │
│─────────────────────────────────────────────────────────────────────│
│ Backlog (3)    │ Planning (1)   │ In Progress (1)│ Review (1) │Done│
│────────────────│────────────────│────────────────│────────────│────│
│▸ Add auth    ▲ │  Refactor DB   │  Fix login     │ Update API │ M  │
│  Fix CSS       │                │                │            │ A  │
│  New search    │                │                │            │    │
│                │                │                │            │    │
│                │                │                │            │    │
│                │                │                │            │    │
│                │                │                │            │    │
│                │                │                │            │    │
╰─────────────────────────────────────────────────────────────────────╯
 h/l:column  j/k:task  enter:open  n:new task  s:sessions  r:refresh
```

Columns map to SpecTask statuses:
- **Backlog**: `backlog`
- **Planning**: `queued_spec_generation`, `spec_generation`, `spec_review`, `spec_revision`, `spec_approved`
- **In Progress**: `queued_implementation`, `implementation_queued`, `implementation`
- **Review**: `implementation_review`, `pull_request`
- **Done**: `done`

Each card shows: task name, priority indicator, type badge, agent work state.

#### 2. Chat View (spec task detail)

Entering a task from kanban opens its chat history. Shows the spec task's
planning session interactions as a conversation.

```
╭─ Fix login bug (#3) ─────────────────────── impl ─ high ─ fix/login-3 ╮
│                                                                        │
│  You                                                                   │
│  The login page crashes when you enter a long email address             │
│                                                                        │
│  Assistant                                                             │
│  I've identified the issue. The email validation regex has a           │
│  catastrophic backtracking pattern. Here's my plan:                    │
│    1. Replace the regex with a simpler check                           │
│    2. Add max-length validation on the input                           │
│                                                                        │
│  You                                                                   │
│  Looks good, go ahead                                                  │
│                                                                        │
│  Assistant                                                             │
│  Done. I've pushed the fix to fix/login-3.                             │
│                                                                        │
╰────────────────────────────────────────────────────────────────────────╯
 > █
```

#### 3. Split Panes (tmux-style)

From any chat view, split to open another task alongside. Panes are
arranged in a binary tree (each split creates two children).

```
╭─ Fix login (#3) ──────────────────╮╭─ Refactor DB (#5) ───────────────╮
│                                    ││                                   │
│  Assistant                        ││  You                              │
│  Done. PR ready for review.       ││  Refactor the ORM layer           │
│                                    ││                                   │
│                                    ││  Assistant                       │
│                                    ││  I'll analyze the query patterns  │
│                                    ││  and create a design doc.         │
│                                    ││                                   │
╰────────────────────────────────────╯╰───────────────────────────────────╯
 > █                                   (unfocused)
───────────────────────────────────────────────────────────────────────────
 ctrl+b |:split-v  ctrl+b -:split-h  ctrl+b n:next  ctrl+b x:close
```

The focused pane has a highlighted border. The input prompt only appears
in the focused pane.

### Key Bindings

#### Global
| Key | Action |
|-----|--------|
| `ctrl+c` | Quit |
| `esc` | Stop running agent (in chat) / clear input |
| `q` | Quit (kanban only, not in chat input) |

#### Kanban View
| Key | Action |
|-----|--------|
| `h` / `l` | Move between columns |
| `j` / `k` | Move between tasks in column |
| `enter` | Open task chat view |
| `n` | New task (opens prompt input) |
| `s` | Switch to sessions list |
| `r` | Refresh board |
| `/` | Search/filter tasks |
| `1-5` | Jump to column |

#### Chat View (single pane or focused pane)
| Key | Action |
|-----|--------|
| `enter` | Send message |
| `shift+enter` | Newline in input |
| `up` / `down` | Scroll conversation |
| `{prefix}` then `%` | Split vertical (tmux default) |
| `{prefix}` then `"` | Split horizontal (tmux default) |
| `{prefix}` then `o` | Focus next pane (tmux default) |
| `{prefix}` then `;` | Focus previous pane (tmux default) |
| `{prefix}` then `x` | Close focused pane (tmux default) |
| `{prefix}` then `t` | Open terminal (shell into sandbox) |
| `{prefix}` then `d` | Detach (daemon keeps running) |
| `{prefix}` then `q` | Close all panes, back to kanban |
| `w` | Open task in web browser (prints URL) |
| `esc` | Stop agent if running / clear input if text / back to kanban if empty |

`{prefix}` defaults to `ctrl+b` (tmux default). Overridden by user's
tmux.conf — e.g. `set -g prefix C-a` makes it `ctrl+a`. Split and
navigation bindings are also parsed: e.g. `bind | split-window -h`
replaces `%` with `|`, `bind - split-window -v` replaces `"` with `-`,
`bind h/j/k/l select-pane` adds vim-style pane navigation.

#### Tmux Keybinding Detection

On startup, parse the user's `~/.tmux.conf` (and `~/.config/tmux/tmux.conf`)
to detect their actual tmux configuration. At minimum, extract:

| tmux.conf directive | What we use it for | tmux default |
|---------------------|--------------------|--------------|
| `set -g prefix C-x` | Our prefix key | `ctrl+b` |
| `bind X split-window -h` | Vertical split key | `%` |
| `bind X split-window -v` | Horizontal split key | `"` |
| `bind X select-pane -L/D/U/R` | Pane navigation keys | `o` (next), `;` (prev) |
| `bind X kill-pane` | Close pane key | `x` |

**Parsing approach:**
- Read the file line by line
- Match `set(-option)? -g prefix (C-\w|...)` for prefix
- Match `bind(-key)? ... split-window` for split bindings
- Match `bind(-key)? ... select-pane` for navigation
- Ignore comments (`#`), `if-shell` blocks (too complex)
- Fall back to tmux defaults for anything we can't parse

This means if a user has `set -g prefix C-a`, our TUI will use `ctrl+a`
as the prefix automatically. Their muscle memory just works.

**Edge cases:**
- No tmux.conf → use tmux defaults (`ctrl+b` prefix)
- Nested tmux (TUI is running inside tmux) → detect `$TMUX` env var and
  warn or suggest using a different prefix via `--prefix` flag
- `source-file` directives → follow one level deep only (don't recurse)

#### 4. Embedded Terminal (sandbox shell)

From a chat view, open a shell directly into the task's sandbox container.
Uses the existing RDP/exec infrastructure to get a shell.

```
╭─ Fix login (#3) ──────────────────╮╭─ shell: Fix login (#3) ──────────╮
│                                    ││                                   │
│  Assistant                        ││ ubuntu@sandbox:~/project$ ls      │
│  Done. PR ready for review.       ││ src/ tests/ README.md             │
│                                    ││ ubuntu@sandbox:~/project$ git log │
│                                    ││ abc1234 Fix email validation      │
│                                    ││ ubuntu@sandbox:~/project$ █       │
│                                    ││                                   │
╰────────────────────────────────────╯╰───────────────────────────────────╯
```

Key binding: `ctrl+b` then `t` opens a terminal pane connected to the
task's sandbox. The terminal pane behaves like a normal terminal emulator
(passes all input through, pty allocation). Uses the session's sandbox
connection to exec into the container.

API: `GET /sessions/{id}/rdp-connection` to get connection info, then
establish a WebSocket/exec channel to the sandbox container.

#### 5. Web UI Link

From any spec task view, press `o` to print/copy the web URL for that task.
The URL format is: `{HELIX_URL}/projects/{project_id}/tasks/{task_id}`

This lets users jump to the browser for desktop streaming, visual review,
or anything that needs a GUI.

```
╭─ Fix login (#3) ───────────────────────────────────────────────────────╮
│                                                                        │
│  ℹ Open in browser: https://app.helix.ml/projects/proj_x/tasks/spt_y  │
│                                                                        │
╰────────────────────────────────────────────────────────────────────────╯
```

#### 6. Detach / Reattach (hmux)

The TUI supports tmux-style detach/reattach. The command is `helix tui`
but we alias it as `hmux` (helix-mux) for convenience.

The key value: the daemon preserves your workspace — which panes you
had open, how they're split, which spec tasks are in each pane, scroll
positions. On reattach, you're right back where you left off without
having to re-open everything. Data (kanban, chat history) is re-fetched
from the Helix API as needed — the state being preserved is the layout.

**Detach:** `{prefix}` then `d` — detaches the terminal. The TUI process
continues running as a daemon, maintaining API connection, cached data,
open panes, scroll positions, polling state, etc.

**Reattach:** `hmux at` (or `helix tui attach`) — reconnects to the
running daemon. All state is preserved, instant restore.

**List sessions:** `hmux ls` (or `helix tui list`)

**Kill session:** `hmux kill` (or `helix tui kill`)

```
$ hmux                          # start new session (or attach if one exists)
$ hmux at                       # reattach to most recent session
$ hmux ls                       # list running sessions
  0: myproject (2 panes, attached)
  1: otherproject (1 pane, detached 5m ago)
$ hmux at -t 1                  # attach to specific session
$ hmux kill -t 0                # kill specific session
```

**Implementation — no daemon needed:**
- On detach (`{prefix}+d`), serialize pane layout state to
  `~/.helix/tui/state.json`: which project, which tasks are open in
  which panes, split directions, focused pane ID
- On `hmux at` / `helix tui`, check for state file. If found, restore
  pane layout and re-fetch data from API for each open task
- State file format:
  ```json
  {
    "project_id": "proj_xxx",
    "panes": {
      "dir": "vertical",
      "left": {"task_id": "spt_aaa"},
      "right": {"dir": "horizontal",
        "left": {"task_id": "spt_bbb"},
        "right": {"task_id": "spt_ccc"}}
    },
    "focused_task_id": "spt_aaa"
  }
  ```
- Clean, simple, no background process. Data is always fresh from the API.
  The only thing being persisted is "which panes were open and how they
  were arranged"

**Key binding:**
| Key | Action |
|-----|--------|
| `{prefix}` then `d` | Detach (tmux default) |

This is parsed from tmux.conf like all other bindings.

### Sessions View (secondary)

Accessible via `s` from kanban. Lists recent chat sessions (non-spec-task).
Same layout as kanban but single-column list. Enter opens chat.
`s` or `tab` switches back to kanban.

### Architecture

```
cmd.go          CLI entry point (hmux, helix tui, subcommands: attach/list/kill)
state.go        Serialize/restore pane layout to ~/.helix/tui/state.json
app.go          Top-level bubbletea model: project picker → kanban ↔ pane mode
kanban.go       Kanban board view
chat.go         Single chat pane (messages + input)
pane.go         Binary tree pane manager (splits, focus, render)
terminal.go     Embedded terminal pane (pty to sandbox shell)
tmux.go         Parse ~/.tmux.conf for prefix key + pane bindings
picker.go       Project picker (startup screen)
api.go          API client wrapper (reuses types from api/pkg/types)
styles.go       Shared lipgloss styles (Claude Code aesthetic)
```

### API Endpoints Used

All via the existing `api/pkg/client.HelixClient` + new `MakeRequest` export:

| Endpoint | Purpose |
|----------|---------|
| `GET /projects` | List projects for picker |
| `GET /spec-tasks?project_id=X` | Load kanban board |
| `GET /spec-tasks/{id}` | Task detail |
| `GET /sessions` | List chat sessions |
| `GET /sessions/{id}` | Session detail |
| `GET /sessions/{id}/interactions` | Load chat history |
| `POST /sessions/chat` | Send chat message |
| `POST /spec-tasks/from-prompt` | Create new task |
| `POST /spec-tasks/{id}/start-planning` | Start planning |
| `POST /spec-tasks/{id}/approve-specs` | Approve specs |

### Data Flow

1. On startup, fetch projects list. If `--project` flag set, skip to kanban.
   Otherwise show a project picker (list with j/k navigation, enter to select).
2. Fetch spec tasks for the project → populate kanban columns.
3. On enter, fetch interactions for the task's `planning_session_id` → render chat.
4. On send message, POST to `/sessions/chat` with the session ID → append response.
5. Background polling: refresh task statuses every 5s while kanban is visible.

### Types Reuse

All data types come directly from `api/pkg/types`:
- `types.SpecTask` — kanban cards
- `types.Session` / `types.SessionSummary` — session list
- `types.Interaction` — chat messages
- `types.SessionChatRequest` — sending messages
- `types.CreateTaskRequest` — new tasks
- `types.Project` — project picker

No type duplication. The TUI `api.go` is a thin wrapper adding methods
the base client doesn't have yet (list projects, list spec tasks, etc).

## Open Questions

All resolved:

1. ~~**Prefix key**~~: Parse user's tmux.conf. Fall back to exact tmux defaults.
   Detect `$TMUX` to warn about nesting conflicts.
2. ~~**Streaming**~~: Poll for v1. No WebSocket/SSE streaming in v1.
3. ~~**Project picker**~~: Show project picker on startup (list with j/k + enter).
   `--project` flag skips the picker.
4. ~~**Sessions view**~~: Deferred to v2. Kanban + spec task chat is the focus.

## Implementation Plan

### Phase 1: Core (MVP)
1. `styles.go` — shared styles matching Claude Code aesthetic
2. `tmux.go` — parse tmux.conf for prefix key + bindings
3. `api.go` — API wrapper (reuses `api/pkg/client` + `api/pkg/types`)
4. `picker.go` — project picker on startup
5. `kanban.go` — kanban board with column navigation
6. `chat.go` — chat pane with message rendering + input
7. `pane.go` — binary tree pane manager
8. `app.go` — top-level model: picker → kanban ↔ panes
9. `cmd.go` — cobra command (`helix tui` / `hmux`)
10. Register in `api/cmd/helix/root.go`, add bubbletea deps to go.mod
11. Build and test

### Phase 2: State + Terminal
12. `state.go` — serialize/restore pane layout to disk
13. `cmd.go` — add `attach` subcommand (restore from state file)
14. `terminal.go` — embedded terminal pane for sandbox shell

### Phase 3: Polish
16. Background polling for kanban/chat updates
17. `w` key to show web URL for task
18. Nested tmux detection and `--prefix` override flag
