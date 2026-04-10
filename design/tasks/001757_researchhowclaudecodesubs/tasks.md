# Implementation Tasks

> **Positioning reminder:** Helix provides cloud computers for agents to run on — NOT an agent harness. Every implementation decision should reinforce this: the user runs Claude on their own machine, Helix provides the infrastructure.

## Phase 1: Pure Claude Mode (Subscription — tmux + JSONL)

### Container Setup (the user's cloud computer)

- [ ] Add Claude Code CLI to the container image (`npm install -g @anthropic-ai/claude-code`)
- [ ] Install tmux in the container image
- [ ] Configure container entrypoint to keep tmux server running (e.g. `tini` as PID 1)
- [ ] Set `TERM=xterm-256color` and tmux `history-limit 50000` in the container

### Dotfile Backup/Restore (General Purpose)

Helix backs up and restores all user dotfiles across container sessions — not Claude-specific. Claude auth persistence is just a natural consequence.

- [ ] Implement general dotfile backup/restore for user home directories across container lifecycles
- [ ] Cover: `~/.claude/`, `~/.gitconfig`, `~/.ssh/`, `~/.config/`, shell rc files, etc.
- [ ] Exclude ephemeral/large directories (e.g. `~/.claude/projects/`, `~/.claude/sessions/`, caches)
- [ ] Store backups in Helix user profile storage (encrypted at rest)

### Claude Auth

- [ ] Ensure `claude auth login` works in the container terminal (headless OAuth: CLI shows URL, user clicks in browser, pastes code back)
- [ ] Verify auth survives dotfile restore: login in session 1, destroy container, start session 2, run `claude auth status`
- [ ] If token expired after restore, prompt user to re-login (should be rare)

### Remove Legacy Claude Token UI

- [ ] Remove the existing claude get-token UI and flow from the Helix platform (naive initial implementation)
- [ ] Remove any associated API endpoints, token storage, and frontend components
- [ ] Update any documentation or onboarding flows that reference the old token mechanism
- [ ] Auth is now handled entirely inside the container via `claude auth login` — Helix platform doesn't touch it

### `helix-claude-sync` Guest Daemon

A guest daemon that runs inside the container alongside Claude. It replaces Zed's role in the WebSocket sync protocol — connecting upstream to the Helix API and downstream to Claude via JSONL tailing + tmux. Part of Helix's existing guest tools (like `desktop-bridge`, `settings-sync-daemon`).

#### tmux Management (downstream → Claude)

- [ ] Create tmux session with wide terminal: `tmux new-session -d -s claude -x 220 -y 50`
- [ ] Launch Claude CLI: `tmux send-keys -t claude "claude --dangerously-skip-permissions" Enter`
- [ ] Build prompt injection via paste-buffer: `tmux set-buffer "<prompt>" && tmux paste-buffer -t claude && tmux send-keys -t claude Enter` (handles multiline and special chars)
- [ ] Use send-keys for simple keypresses: `y`/`n` approvals, `Enter`, `C-c` interrupt
- [ ] Evaluate `--permission-mode acceptEdits` vs `--dangerously-skip-permissions` — what's the right safety level for Helix users?

#### JSONL Tailing (downstream → Claude)

- [ ] Build process to find session UUID: read `~/.claude/sessions/<pid>.json` to get the sessionId for the running claude process
- [ ] Build JSONL tailer: `tail -f ~/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl` with ~100ms poll
- [ ] Parse each line as JSON, dispatch by `type` field:
  - `assistant` → extract `message.content` blocks (thinking, text, tool_use)
  - `user` → track tool results, user messages
  - `queue-operation` → detect turn start/end
  - `attachment` → handle skill listings, system attachments
- [ ] Handle incremental assistant messages: multiple JSONL lines share one `message.id`, group by it
- [ ] Handle large tool results stored in `<session>/tool-results/toolu_*.json`
- [ ] Handle subagent transcripts in `<session>/subagents/agent-*.jsonl`
- [ ] Build CWD encoding function: replace all `/` with `-` in absolute path
- [ ] Detect user-created sessions: new JSONL file appears without a pending `request_id` → user started Claude directly in terminal

#### WebSocket Protocol (upstream → Helix API)

The daemon speaks the same WebSocket sync protocol as Zed. These are the events it sends/receives.

**Events sent TO Helix API:**
- [ ] `agent_ready` — send when claude process starts and first `queue-operation:dequeue` is detected in JSONL
- [ ] `thread_created` — send when new JSONL session file appears, with `acp_thread_id` = claude session UUID and `request_id` from pending prompt
- [ ] `user_created_thread` — send when user starts a new claude session directly (not Helix-initiated)
- [ ] `message_added` (role=assistant) — translate JSONL `assistant` lines into `message_added` events with `entry_type`, `tool_name`, `tool_status`
- [ ] `message_added` (role=user) — translate JSONL `user` lines (non-meta) into `message_added` events
- [ ] `message_completed` — send on `queue-operation:dequeue` or `stop_reason: end_turn`. Include `request_id`. Guard against double-send
- [ ] `thread_title_changed` — poll `sessions.jsonl` for title updates
- [ ] `thread_load_error` — send when claude process exits with error, session file not found, or 60s readiness timeout

**Commands received FROM Helix API:**
- [ ] `chat_message` — inject prompt via paste-buffer. If no claude process running, start one first. Track `request_id` for response correlation
- [ ] `chat_message` (with `acp_thread_id`) — ensure correct claude session is active, inject prompt
- [ ] `chat_message` (with `is_continue`) — start claude with `-r <session-id>`, wait for ready, inject continue prompt
- [ ] `open_thread` — start/resume claude session for the given thread ID

#### Session Lifecycle & State Recovery

- [ ] Maintain `helix_thread_id ↔ claude_session_uuid` mapping, persist to disk for crash recovery
- [ ] On WebSocket reconnect to Helix API, re-send `agent_ready`. Helix API handles `pickupWaitingInteraction` and `open_thread`
- [ ] On container restart, check for existing tmux/claude process. If found, resume tailing. If not, wait for `chat_message`
- [ ] Handle continue prompt after container restart (`is_continue: true` → `claude -r <session-id>`)
- [ ] Track pending `request_id`s for response correlation (interaction state transitions)
- [ ] If new prompt arrives while previous turn active, send `Ctrl+C` → wait for turn end → send `message_completed` for old turn → inject new prompt
- [ ] Monitor claude PID liveness (`kill -0`). If dead, send `thread_load_error`. Restart on next `chat_message`

### Testing

**Auth & container basics:**
- [ ] Test `claude auth login` end-to-end in a Docker container with no browser
- [ ] Test auth survives dotfile backup/restore: login in session 1, destroy container, start session 2, run `claude auth status`

**Guest daemon ↔ Helix API (WebSocket protocol):**
- [ ] Test full flow: Helix sends `chat_message` → daemon starts claude → daemon sends `agent_ready` → daemon sends `thread_created` → daemon streams `message_added` → daemon sends `message_completed`
- [ ] Test follow-up: Helix sends second `chat_message` with `acp_thread_id` → daemon injects into existing session → response cycle completes
- [ ] Test tool execution: Claude edits a file → daemon sends `message_added` with `entry_type: "tool_call"` + `tool_name` + `tool_status` → daemon sends tool result update
- [ ] Test mid-turn interrupt: Helix sends new `chat_message` while turn active → daemon sends `Ctrl+C` → sends `message_completed` for old turn → injects new prompt
- [ ] Test `thread_load_error`: attempt to resume non-existent session → daemon sends error event
- [ ] Test `user_created_thread`: user starts claude directly in terminal → daemon detects new JSONL → sends `user_created_thread` to Helix API
- [ ] Test duplicate `message_completed` dedup: ensure exactly one completion event per turn

**Guest daemon resilience:**
- [ ] Test daemon WebSocket reconnect: kill WS connection → daemon reconnects → Helix API sends `open_thread` → daemon resumes
- [ ] Test container restart: destroy container → new container starts → daemon reconnects → Helix sends `chat_message` with `is_continue` → daemon starts `claude -r <session-id>` → session resumes
- [ ] Test claude process crash: kill claude PID → daemon sends `thread_load_error` → Helix sends new `chat_message` → daemon restarts claude
- [ ] Test long sessions: verify JSONL tailing handles sessions with 100+ turns without memory growth

**User direct access (cloud computer story):**
- [ ] Test user attaches to tmux via desktop stream terminal — can interact with Claude directly
- [ ] Test user types prompt in terminal while daemon is active — both daemon and user see the response
- [ ] Test user starts new Claude session in a second tmux pane — daemon detects via `user_created_thread`

**End-to-end with Helix UI:**
- [ ] Test Helix UI shows streaming response in real-time (text, tool calls, thinking blocks)
- [ ] Test Helix UI shows correct session/thread state (ready, working, waiting for input)
- [ ] Test Helix UI can send prompts and receive responses through full roundtrip

## Phase 2: Zed ACP Mode (API Key — Richer UI)

Keep existing Zed ACP integration for users who have API keys and want the richer IDE experience.

- [ ] Add option for users to provide `ANTHROPIC_API_KEY` in Helix settings
- [ ] Inject as env var in container — Zed ACP + Agent SDK picks it up automatically
- [ ] Build mode selector in Helix UI: "Claude Subscription (terminal)" vs "API Key (Zed integration)"
- [ ] Add cost comparison info so users understand subscription vs API key pricing

## Phase 3: Contact Anthropic / Zed (Parallel)

- [ ] Ask Zed whether they have a formal partner agreement with Anthropic for subscription OAuth
- [ ] Contact Anthropic sales: `https://www.anthropic.com/contact-sales?utm_source=claude_code&utm_medium=docs&utm_content=legal_compliance_contact_sales`
- [ ] Frame Helix as providing cloud computers for agents to run on (like Codespaces) — NOT an agent harness or Claude wrapper
- [ ] Emphasise: user authenticates directly, Claude CLI talks directly to api.anthropic.com, Helix doesn't route/proxy/manage credentials, every session is user-initiated
- [ ] If approved, evaluate whether Zed ACP mode can also support subscription auth (collapsing the two modes)

## Monitoring

- [ ] Track Anthropic announcements (Boris Cherny on X, Anthropic blog) for policy updates
- [ ] Monitor anthropics/claude-agent-sdk-typescript GitHub repo for auth-related changes
- [ ] Watch for changes to code.claude.com/docs/en/legal-and-compliance
- [ ] Watch for new CLI flags that might improve headless/container usage
