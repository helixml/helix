# Implementation Tasks

## Phase 1: Pure Claude Mode (Subscription — tmux + JSONL)

### Container Setup

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

### tmux Session Management

- [ ] Create tmux session with wide terminal: `tmux new-session -d -s claude -x 220 -y 50`
- [ ] Launch Claude CLI: `tmux send-keys -t claude "claude --dangerously-skip-permissions" Enter`
- [ ] Build prompt injection via paste-buffer: `tmux set-buffer "<prompt>" && tmux paste-buffer -t claude && tmux send-keys -t claude Enter` (handles multiline and special chars)
- [ ] Use send-keys for simple keypresses: `y`/`n` approvals, `Enter`, `C-c` interrupt
- [ ] Determine how to detect "Claude is ready for input" — check JSONL for `queue-operation:dequeue` or `stop_reason: "end_turn"`
- [ ] Evaluate `--permission-mode acceptEdits` vs `--dangerously-skip-permissions` — what's the right safety level for Helix users?

### JSONL Tailing Daemon

- [ ] Build process to find session UUID: read `~/.claude/sessions/<pid>.json` to get the sessionId for the running claude process
- [ ] Build JSONL tailer: `tail -f ~/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl`
- [ ] Parse each line as JSON, dispatch by `type` field:
  - `assistant` → extract `message.content` blocks (thinking, text, tool_use)
  - `user` → track tool results, user messages
  - `queue-operation` → detect turn start/end
  - `attachment` → handle skill listings, system attachments
- [ ] Handle incremental assistant messages: multiple JSONL lines share one `message.id`, group by it
- [ ] Handle large tool results stored in `<session>/tool-results/toolu_*.json`
- [ ] Handle subagent transcripts in `<session>/subagents/agent-*.jsonl`
- [ ] Build CWD encoding function: replace all `/` with `-` in absolute path

### Helix UI Sync

- [ ] Map JSONL events to Helix's UI components:
  - `text` blocks → chat messages
  - `tool_use` blocks → tool call displays (show name, input, status)
  - `tool_result` blocks → tool result displays
  - `thinking` blocks → collapsible thinking sections (optional)
  - `message.usage` → token usage / cost display
- [ ] Show real-time streaming: relay JSONL lines as they appear
- [ ] Show "Claude is working..." / "Claude is waiting for input" based on queue-operation events
- [ ] Build prompt input UI that sends text via tmux send-keys

### Testing

- [ ] Test full flow: auth → start session → send prompt → receive response → send follow-up
- [ ] Test tool execution: Claude edits a file, verify JSONL captures the full tool_use + tool_result cycle
- [ ] Test long sessions: verify JSONL tailing handles sessions with 100+ turns
- [ ] Test container restart: can we resume a session with `claude -c` or `claude -r <session-id>`?
- [ ] Test `claude auth login` end-to-end in a Docker container with no browser

## Phase 2: Zed ACP Mode (API Key — Richer UI)

Keep existing Zed ACP integration for users who have API keys and want the richer IDE experience.

- [ ] Add option for users to provide `ANTHROPIC_API_KEY` in Helix settings
- [ ] Inject as env var in container — Zed ACP + Agent SDK picks it up automatically
- [ ] Build mode selector in Helix UI: "Claude Subscription (terminal)" vs "API Key (Zed integration)"
- [ ] Add cost comparison info so users understand subscription vs API key pricing

## Phase 3: Contact Anthropic / Zed (Parallel)

- [ ] Ask Zed whether they have a formal partner agreement with Anthropic for subscription OAuth
- [ ] Contact Anthropic sales: `https://www.anthropic.com/contact-sales?utm_source=claude_code&utm_medium=docs&utm_content=legal_compliance_contact_sales`
- [ ] Frame Helix as a cloud dev environment (like Codespaces), not a Claude wrapper
- [ ] If approved, evaluate whether Zed ACP mode can also support subscription auth (collapsing the two modes)

## Monitoring

- [ ] Track Anthropic announcements (Boris Cherny on X, Anthropic blog) for policy updates
- [ ] Monitor anthropics/claude-agent-sdk-typescript GitHub repo for auth-related changes
- [ ] Watch for changes to code.claude.com/docs/en/legal-and-compliance
- [ ] Watch for new CLI flags that might improve headless/container usage
