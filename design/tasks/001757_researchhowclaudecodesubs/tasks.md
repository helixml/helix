# Implementation Tasks

## Phase 1: CLI + tmux + JSONL Tailing (Option D — Primary)

### Container Setup

- [ ] Add Claude Code CLI to the container image (`npm install -g @anthropic-ai/claude-code`)
- [ ] Install tmux in the container image
- [ ] Configure container entrypoint to keep tmux server running (e.g. `tini` as PID 1)
- [ ] Set `TERM=xterm-256color` and tmux `history-limit 50000` in the container

### Auth: Setup Token Flow

- [ ] Build Helix UI for users to generate & provide a `claude setup-token` value
- [ ] Document the flow: user runs `claude setup-token` locally → pastes token into Helix
- [ ] Securely store the token (encrypted at rest)
- [ ] Inject the token into `~/.claude/` config in the container at startup
- [ ] Verify: does the setup-token work with the interactive CLI in a container? (Test this early — it's a critical assumption)
- [ ] Fallback: test `claude auth login` headless flow (displays URL, user pastes code back)

### tmux Session Management

- [ ] Create tmux session with wide terminal: `tmux new-session -d -s claude -x 220 -y 50`
- [ ] Launch Claude CLI: `tmux send-keys -t claude "claude --dangerously-skip-permissions" Enter`
- [ ] Build prompt injection function: `tmux send-keys -t claude -l "<prompt>" Enter`
- [ ] Build interrupt function: `tmux send-keys -t claude C-c`
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
- [ ] Test `claude setup-token` end-to-end in a Docker container

## Phase 2: Contact Anthropic / Zed (Parallel)

- [ ] Ask Zed whether they have a formal partner agreement with Anthropic for subscription OAuth
- [ ] Contact Anthropic sales: `https://www.anthropic.com/contact-sales?utm_source=claude_code&utm_medium=docs&utm_content=legal_compliance_contact_sales`
- [ ] Frame Helix as a cloud dev environment (like Codespaces), not a Claude wrapper
- [ ] If approved, evaluate switching back to ACP integration for richer UX

## Phase 3: API Key Auth (Fallback)

- [ ] Add option for users to provide `ANTHROPIC_API_KEY` instead of subscription token
- [ ] Inject as env var in container — works with both CLI and ACP/Agent SDK paths
- [ ] Add cost comparison info so users understand subscription vs API key pricing

## Monitoring

- [ ] Track Anthropic announcements (Boris Cherny on X, Anthropic blog) for policy updates
- [ ] Monitor anthropics/claude-agent-sdk-typescript GitHub repo for auth-related changes
- [ ] Watch for changes to code.claude.com/docs/en/legal-and-compliance
- [ ] Watch for new CLI flags that might improve headless/container usage
