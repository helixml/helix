# Design: Claude Code Subscription Auth — Options for Helix

## Positioning: Helix Provides Computers for Agents to Run On

Helix is **not** an agent harness, a Claude wrapper, or an alternative Claude frontend. Helix provides **cloud computers** (Linux containers) for AI agents to run on — the same way Codespaces, Gitpod, or any cloud VM provider does. The user logs into their Helix container, installs their tools, and runs them. Claude Code is one of those tools.

This distinction is critical for Anthropic compliance:

| Agent harness (NOT Helix) | Cloud computer (Helix) |
|---|---|
| Wraps Claude's API in a custom UX | Provides a Linux environment where users run CLI tools |
| Manages/proxies user credentials | User authenticates directly — Helix never sees tokens |
| Routes API requests through its backend | Claude CLI talks directly to api.anthropic.com |
| Orchestrates prompts programmatically | User initiates every session and prompt |
| Brands itself as "powered by Claude" | Brands itself as a dev environment |

Every architectural decision below reinforces this positioning: Helix provides the infrastructure, the user runs Claude themselves.

## Current Architecture

```
User (browser) → Helix App → Container with Zed IDE
                                  ↓
                              Zed ACP extension
                                  ↓
                          @agentclientprotocol/claude-agent-acp  (by Zed Industries)
                                  ↓
                          @anthropic-ai/claude-agent-sdk           (by Anthropic)
                                  ↓
                              claude subprocess (cli.js)           (by Anthropic, closed-source)
                                  ↓
                          Anthropic API (api.anthropic.com)
```

The user's subscription OAuth token flows through this entire chain. From Anthropic's perspective, the request originates from the Agent SDK — which their docs explicitly say must use API keys, not subscription OAuth.

## Why Helix Feels Like a Grey Area

Helix's model is closer to a **cloud dev environment** (like Codespaces, Gitpod, or a remote VM) than a **third-party Claude wrapper** (like OpenClaw). Helix provides computers for agents to run on — the user isn't using an alternative Claude frontend, they're running Claude Code inside their own container. If the user SSHed into a VM and ran `claude` from the terminal with their own subscription, that would be unambiguously fine.

But the current integration goes through the Agent SDK rather than the CLI directly, which crosses the policy line. The recommended architecture (Option D) eliminates this by having the user run the CLI directly — Helix just provides the Linux container and a richer UI layer on top.

## Options Analysis

### Option A: Contact Anthropic for Partner Approval

The SDK docs say "unless previously approved" — this is an explicit partner exception path.

**Argument to Anthropic:**
- Helix provides a container-based dev environment, not an alternative Claude frontend
- Users authenticate with their own credentials — Helix doesn't manage or pool subscriptions
- The user's experience is equivalent to running Claude Code in a VM
- Helix isn't bypassing prompt caching (the stated technical reason for the ban)
- Helix could agree to branding guidelines, usage reporting, etc.

**Pros:**
- Only path that preserves subscription-based auth legally
- Establishes a direct relationship with Anthropic
- Future-proof against further enforcement

**Cons:**
- Uncertain outcome — Anthropic may say no
- Could take time to negotiate
- May come with conditions (usage caps, revenue sharing, branding requirements)

**Risk: Medium.** The "unless previously approved" language exists for a reason. Anthropic may be receptive to legitimate dev environment use cases vs. wrapper tools that arbitrage subscription pricing.

### Option B: Switch to API Key Authentication

Use `ANTHROPIC_API_KEY` instead of subscription OAuth. This is what Anthropic's docs recommend for all Agent SDK users.

**Implementation:**
- Users create an API key at console.anthropic.com
- Helix stores the key and injects it as `ANTHROPIC_API_KEY` environment variable in the container
- No code changes needed in the Zed/ACP/SDK chain — the Agent SDK already supports this

**Pros:**
- Fully compliant with Anthropic's policy
- No approval needed
- Already supported by the Agent SDK

**Cons:**
- **Dramatically higher cost for users.** Claude Max subscription is ~$100-200/month for heavy usage. API pricing for equivalent usage could be $500-2000+/month depending on volume (Opus at $15/$75 per MTok input/output)
- Users must manage API keys and billing separately
- Removes a key selling point ("bring your own Claude subscription")
- Competitive disadvantage vs. tools that haven't been caught yet

**Risk: Low technical risk, high business risk.** Users may churn if costs increase 5-10x.

### Option C: Use Bedrock/Vertex as API Provider

Route through AWS Bedrock or Google Vertex AI instead of direct Anthropic API. Users or Helix would have a cloud provider account.

**Implementation:**
- Set `CLAUDE_CODE_USE_BEDROCK=1` + AWS credentials, or
- Set `CLAUDE_CODE_USE_VERTEX=1` + GCP credentials
- Agent SDK supports both natively

**Pros:**
- Compliant — these are supported API providers
- May offer negotiated enterprise pricing
- Helix could potentially consolidate billing under its own cloud account and resell

**Cons:**
- Pricing still per-token (no flat rate)
- Adds cloud provider dependency
- Users need cloud accounts or Helix needs to manage credentials
- Feature parity may lag behind direct Anthropic API

**Risk: Low.** Fully supported path but doesn't solve the cost problem.

### Option D: Run Claude Code CLI Directly — tmux + JSONL Tailing (RECOMMENDED TECHNICAL PATH)

Instead of going through Zed ACP → Agent SDK, the user runs the Claude Code CLI directly in a tmux session inside the container. Helix provides prompt templates via `tmux paste-buffer` (equivalent to clipboard paste) and syncs state back to the Helix UI by tailing the session JSONL files.

This approach reinforces Helix's positioning as a cloud computer provider: the user is running Claude Code CLI on their own machine (which happens to be a Helix container), authenticated with their own subscription. Helix reads local files and relays keystrokes — the same things any terminal multiplexer, IDE, or shell integration does.

#### Architecture

```
                         Helix API (Go)
                              │
                    WebSocket sync protocol
                     (same as current Zed)
                              │
    ┌─────────────────────────┼──────────────────────────────────┐
    │  Helix Container (user's cloud computer)                   │
    │                         │                                  │
    │                   helix-claude-sync                         │
    │                   (guest daemon)                            │
    │                    ┌────┴────┐                              │
    │                    │         │                              │
    │              JSONL tailing   tmux paste-buffer/send-keys    │
    │                    │         │                              │
    │                    ▼         ▼                              │
    │               ~/.claude/   tmux server                     │
    │               projects/    └── pane 0: claude               │
    │               <cwd>/           (interactive mode,           │
    │               <session>.jsonl   user's own subscription)   │
    │                                                            │
    │  Auth: user runs `claude auth login` in their terminal     │
    │  ~/.claude/ persisted via general dotfile backup/restore    │
    └────────────────────────────────────────────────────────────┘
```

**`helix-claude-sync` is a guest daemon** — a lightweight process that runs inside the user's container alongside Claude. It replaces Zed's role in the WebSocket sync protocol:

- **Upstream (to Helix API):** Connects to the Helix API via the same WebSocket sync protocol that Zed currently uses. Sends `thread_created`, `message_added`, `message_completed`, `agent_ready`, etc.
- **Downstream (to Claude CLI):** Tails JSONL session files for structured output, injects prompts via tmux `paste-buffer`/`send-keys`.
- **It is part of Helix's guest tools** — the same way Helix already ships `desktop-bridge`, `settings-sync-daemon`, and other guest processes in the container image. It's infrastructure, not an agent.

This means the Helix API server-side code (`websocket_external_agent_sync.go`) changes minimally — the guest daemon speaks the same protocol Zed speaks. The major work is building the guest daemon itself.

#### Authentication in the Container

**Helix provides a Linux computer. The user runs Claude on it.** The user has a terminal in their Helix session. They log in to Claude exactly the same way they would on any machine:

```bash
claude auth login
```

The CLI displays a URL. The user clicks it in their browser, authenticates with their Claude subscription (Pro/Max/etc.), and pastes the code back into the terminal. This is the standard headless OAuth flow — identical to SSHing into any remote machine and running `claude login`. **Helix doesn't manage, store, or proxy any credentials.** The user is an ordinary user running Claude Code on their own cloud computer.

The auth state persists in `~/.claude/` inside the container. As long as the container persists (or `~/.claude/` is on a persistent volume), the user stays logged in across sessions.

Alternative for power users: `claude setup-token` generates a long-lived token on a local machine that can be pasted into the container. But the interactive login is simpler and doesn't require the user to have Claude installed locally.

#### Session JSONL Files — The Data Path

Claude Code writes a real-time session transcript as JSONL (newline-delimited JSON) at:

```
~/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl
```

Where `<encoded-cwd>` replaces all `/` with `-` (e.g., `/home/user/work` → `-home-user-work`).

To find the session UUID for a running claude process:
```
~/.claude/sessions/<pid>.json
→ {"pid":1234,"sessionId":"550e8400-...","cwd":"/workspace","kind":"interactive"}
```

**Each JSONL line is one of four types:**

| Type | Purpose | Key Fields |
|------|---------|------------|
| `queue-operation` | Session start/end markers | `operation` ("enqueue"/"dequeue"), `timestamp` |
| `user` | User messages + tool results | `message.content` (string or content blocks), `permissionMode`, `cwd` |
| `assistant` | Model responses | `message.content` (array of thinking/text/tool_use blocks), `message.model`, `message.usage` |
| `attachment` | System attachments (skills, etc.) | `attachment.type`, `attachment.content` |

**Assistant message content blocks:**

```json
{"type": "thinking", "thinking": "...", "signature": "..."}
{"type": "text", "text": "Here's what I found..."}
{"type": "tool_use", "id": "toolu_...", "name": "Edit", "input": {"file_path": "...", "old_string": "...", "new_string": "..."}}
```

**Tool results (in user messages):**

```json
{"type": "tool_result", "tool_use_id": "toolu_...", "content": "File edited successfully"}
```

**Important:** Assistant messages are emitted incrementally — one JSONL line per content block. A single API response with thinking + text + tool_use produces 3 lines sharing the same `message.id` but different `uuid`s. All messages carry `uuid` and `parentUuid` forming a linked list.

**Large tool results** are stored separately in:
```
~/.claude/projects/<cwd>/<session-uuid>/tool-results/toolu_vrtx_XXXX.json
```

**Subagent transcripts** are stored in:
```
~/.claude/projects/<cwd>/<session-uuid>/subagents/agent-XXXX.jsonl
```

#### Injecting Prompts via tmux

Two approaches for sending prompts into the claude session:

**Option 1: `paste-buffer` (preferred for prompts)**

```bash
# Load prompt into tmux buffer and paste it — simulates clipboard paste
tmux set-buffer "Fix the bug in auth.py"
tmux paste-buffer -t claude
tmux send-keys -t claude Enter
```

Advantages over `send-keys`:
- Handles multiline prompts cleanly (Claude Code accepts pasted blocks natively)
- No issues with special characters (`$`, `"`, `\`, etc.)
- Sends the entire block at once, like a real user paste

**Option 2: `send-keys` (for simple commands and keypresses)**

```bash
# Simple text + enter
tmux send-keys -t claude -l "Fix the bug in auth.py"
tmux send-keys -t claude Enter

# Tool approval
tmux send-keys -t claude "y" Enter

# Ctrl+C to interrupt
tmux send-keys -t claude C-c
```

**Recommended pattern:** Use `paste-buffer` for prompt templates (multiline, may contain special chars). Use `send-keys` for simple keypresses (Enter, y/n, Ctrl+C).

**Key tmux considerations:**
- **Terminal width matters** — set `-x 220` or wider to prevent line wrapping in the TUI
- **Timing:** Both are fire-and-forget with no acknowledgement. Check the JSONL for `queue-operation:dequeue` to confirm Claude finished the previous turn before sending the next prompt
- **Permission mode:** Use `--dangerously-skip-permissions` or `--permission-mode acceptEdits` to minimize interactive approval prompts. This is the biggest simplification — without it, you need to detect and respond to every tool approval prompt

#### Detecting State from JSONL (Not tmux capture-pane)

**The JSONL file is the primary data channel, NOT `tmux capture-pane`.**

`capture-pane` returns raw terminal output with ANSI escape codes, spinner characters, and TUI redraws. It's messy to parse. Use it only as a fallback for detecting prompt readiness.

The JSONL file gives you structured, machine-readable data:

```python
# Pseudocode for JSONL tailing
import json, time

def tail_session(jsonl_path):
    with open(jsonl_path, 'r') as f:
        f.seek(0, 2)  # seek to end
        while True:
            line = f.readline()
            if not line:
                time.sleep(0.1)
                continue
            msg = json.loads(line)
            
            if msg['type'] == 'assistant':
                for block in msg['message']['content']:
                    if block['type'] == 'text':
                        yield ('text', block['text'])
                    elif block['type'] == 'tool_use':
                        yield ('tool_call', block['name'], block['input'])
                    elif block['type'] == 'thinking':
                        yield ('thinking', block['thinking'])
            
            elif msg['type'] == 'user' and not msg.get('isMeta'):
                yield ('user_message', msg['message']['content'])
            
            elif msg['type'] == 'queue-operation':
                if msg['operation'] == 'dequeue':
                    yield ('turn_complete', None)
```

**Detecting "Claude is done with this turn":**
- Watch for a `queue-operation` with `operation: "dequeue"` — this signals the turn is complete
- Alternatively, watch for an `assistant` message with `message.stop_reason: "end_turn"` — this means Claude has finished responding and is waiting for input

#### Feature Parity: WebSocket Sync Protocol → Guest Daemon

The `helix-claude-sync` guest daemon connects to the Helix API via the **same WebSocket sync protocol** that Zed currently uses. It speaks the same event types. The Helix API server-side code (`websocket_external_agent_sync.go`) changes minimally — it just has a different client on the other end.

The daemon's job is to translate between Claude's JSONL session files / tmux interface and the WebSocket protocol events the Helix API expects.

**Helix API → Guest Daemon (commands received via WebSocket):**

| WebSocket Command | Purpose | Guest Daemon Action |
|---|---|---|
| `chat_message` | Send user prompt to Claude | `tmux set-buffer "<prompt>" && tmux paste-buffer -t claude && tmux send-keys -t claude Enter`. If Claude process not running, start it first |
| `chat_message` (with `acp_thread_id`) | Send follow-up to existing thread | Look up Claude session UUID from thread mapping, ensure correct session is active, inject prompt via paste-buffer |
| `chat_message` (with `is_continue`) | Recovery prompt after container restart | Start Claude with `claude -r <session-id>`, wait for ready, inject continue prompt |
| `open_thread` | Open/resume a conversation thread | Start `claude -r <session-id>` (resume specific session) or `claude -c` (continue last). Map Helix thread IDs ↔ Claude session UUIDs |

**Guest Daemon → Helix API (events sent via WebSocket):**

| WebSocket Event | Purpose | JSONL Source |
|---|---|---|
| `agent_ready` | Claude loaded and ready for prompts | Claude process started + first `queue-operation:dequeue` detected in JSONL |
| `thread_created` | New conversation thread started | New JSONL file appears at `~/.claude/projects/<cwd>/<session-uuid>.jsonl`. Send `acp_thread_id` = session UUID, `request_id` from pending prompt |
| `message_added` (role=assistant) | Streaming assistant response | Each new `assistant` JSONL line → extract content blocks → send as `message_added` with `entry_type` ("text" or "tool_call"), `tool_name`, `tool_status` |
| `message_added` (role=user) | Echo user message / tool results | `user` JSONL lines (non-meta) → send as `message_added` with role=user |
| `message_completed` | Assistant turn finished | `queue-operation:dequeue` or `assistant` line with `stop_reason: "end_turn"` → send `message_completed` with `request_id` |
| `thread_title_changed` | Conversation title updated | Poll `~/.claude/projects/<cwd>/sessions.jsonl` for session metadata changes |
| `thread_load_error` | Failed to load/resume session | Claude process exits with error, or session file not found → send `thread_load_error` with error message |
| `user_created_thread` | User started a new Claude session directly in terminal | Detect new JSONL file creation that wasn't initiated by a Helix `chat_message` (no pending `request_id`) → send `user_created_thread` |
| `ping` | Keepalive | Standard WebSocket ping, same as Zed |

**Content block mapping (within `message_added`):**

| JSONL Content Block | WebSocket `message_added` Fields |
|---|---|
| `{"type": "text", "text": "..."}` | `entry_type: "text"`, `content: "..."` |
| `{"type": "tool_use", "id": "toolu_...", "name": "Edit", "input": {...}}` | `entry_type: "tool_call"`, `tool_name: "Edit"`, `tool_status: "running"`, `content: <formatted summary>` |
| `{"type": "tool_result", "tool_use_id": "toolu_...", "content": "..."}` | Update previous tool_call entry: `tool_status: "completed"`, append result to content |
| `{"type": "thinking", "thinking": "..."}` | `entry_type: "text"`, `content: <thinking content>` (or separate entry type if Helix UI supports it) |
| `message.usage` | Not sent via `message_added` — included in `message_completed` or tracked separately for cost display |
| `message.model` | Not sent via `message_added` — available for display but not part of current protocol |

**Streaming, throttling, and accumulation:**

The guest daemon does NOT need to implement DB throttling or frontend publish throttling — that's the Helix API server's job (it already does this in `websocket_external_agent_sync.go`). The daemon just needs to:

| Concern | Guest Daemon Responsibility | Helix API Responsibility |
|---|---|---|
| JSONL polling interval | Poll at ~100ms. Send `message_added` for each new line | — |
| Message grouping | Group JSONL lines by `message.id`. Send one `message_added` per content block, with consistent `message_id` | Accumulate by `message_id` (existing `MessageAccumulator`) |
| DB write throttle | — | Buffer and write at most every 200ms (existing) |
| Frontend publish throttle | — | Publish at most every 50ms (existing `publishInterval`) |
| Per-entry delta patches | — | Compute UTF-16 deltas (existing `computePatch`) |
| Duplicate completion dedup | Send exactly one `message_completed` per turn. Guard against both `queue-operation:dequeue` AND `stop_reason: end_turn` firing | Dedup via `completedRequestIDs` (existing) |

**Session lifecycle and state recovery:**

| Feature | Guest Daemon Implementation |
|---|---|
| Session readiness (60s timeout) | Start claude process → tail JSONL → wait for first `queue-operation:dequeue`. If not seen within 60s, send `thread_load_error`. On success, send `agent_ready` |
| Session ↔ thread mapping | Maintain `helix_thread_id ↔ claude_session_uuid` map. Persist to disk for crash recovery. Discover UUID via `~/.claude/sessions/<pid>.json` |
| Reconnection to Helix API | On WS disconnect, reconnect with backoff. On reconnect, re-send `agent_ready`. Helix API handles `pickupWaitingInteraction` and `open_thread` |
| Container restart recovery | On startup, check for running claude process (tmux session). If found, resume tailing. If not, wait for `chat_message` from Helix API |
| Continue prompt after restart | When Helix sends `chat_message` with `is_continue: true`, start claude with `-r <session-id>`, wait for ready, inject the continue prompt |
| Interaction state transitions | Track pending `request_id` → when `message_completed` fires, include the `request_id` so Helix API can transition the interaction from `Waiting` → `Complete` |
| Auto-complete stale interactions | If a new prompt arrives while a previous turn is still active, send `message_completed` for the old turn before injecting the new prompt (handles interrupt race) |
| Claude process crash | Monitor claude PID (`kill -0`). If dead, send `thread_load_error` to Helix API. On next `chat_message`, restart claude |

**SpecTask integration:**

| Feature | Guest Daemon Implementation |
|---|---|
| SpecTask thread tracking | Helix API handles `SpecTaskWorkSession`/`SpecTaskZedThread` creation on `thread_created` — no daemon changes needed |
| SpecTask activity updates | Helix API updates `LastActivityAt` on `message_completed` — no daemon changes needed |
| SpecTask phase detection | Helix API handles this server-side — no daemon changes needed |

**Design review comment streaming:**

| Feature | Guest Daemon Implementation |
|---|---|
| Dual-publish to commenter | Helix API handles `sessionToCommenterMapping` and dual-publish — no daemon changes needed |
| Comment finalization | Helix API handles `finalizeCommentResponse` on `message_completed` — no daemon changes needed |

**Prompt queue:**

| Feature | Guest Daemon Implementation |
|---|---|
| Queue processing after completion | Helix API calls `processPromptQueue` after `message_completed` → sends next `chat_message` to daemon — no daemon changes needed |
| Interrupt prompts | Helix API sends `chat_message` with interrupt flag. Daemon sends `Ctrl+C` via `tmux send-keys -t claude C-c`, waits for JSONL to show turn ended, then injects new prompt |

**Key insight: most complexity stays server-side.** The existing `websocket_external_agent_sync.go` handles DB operations, throttling, delta computation, prompt queuing, spectask tracking, design review integration, and frontend publishing. The guest daemon is a relatively thin translator between JSONL/tmux and the WebSocket protocol.

**What we gain over Zed/ACP:**

- **Thinking blocks** — full extended thinking content (ACP doesn't expose this)
- **Token usage per turn** — exact input/output/cache token counts
- **Subagent transcripts** — tail `<session>/subagents/agent-*.jsonl` for full visibility into delegated agent work
- **Tool input details** — exact arguments passed to every tool (file paths, search patterns, edit diffs)
- **Session history** — all JSONL files persist on disk, queryable after the fact
- **User direct access** — user can attach to tmux session from desktop stream, SSH, or web terminal at any time

**What Mode 1 (CLI) trades off vs Mode 2 (Zed ACP):**

Note: Zed ACP remains fully supported for API key users. These are trade-offs specific to subscription users who use Mode 1 instead.

- **No inline diffs in Zed** — CLI mode doesn't have IDE integration for showing diffs (Zed ACP does, for API key users)
- **No tool approval UI in Zed** — mitigated by `--dangerously-skip-permissions` or `--permission-mode acceptEdits`
- **Helix UI overlay instead of Zed panel** — the Claude panel in Zed's sidebar is replaced by Helix's own JSONL-powered UI
- **~100ms streaming latency** — JSONL tailing polls at ~100ms vs WebSocket's near-instant delivery (acceptable for UI)

#### Why Interactive Mode, Not Print Mode (`--output-format stream-json`)

The CLI also supports structured I/O in print mode:
```bash
claude -p "Fix the bug" --output-format stream-json --input-format stream-json
```

This emits typed JSON events to stdout including partial messages, tool calls, and results. However, interactive mode is the right choice for three reasons:

1. **Policy compliance.** Print mode (`-p`) is documented as the "SDK mode" and may be subject to the same Agent SDK restrictions on subscription auth. Interactive mode is the primary way individual users run Claude Code — it's unambiguously allowed.

2. **User direct access.** The user must be able to interact with Claude directly at any time — via the desktop streaming session (opening a terminal), or by attaching to the tmux session from any PTY. This is a core part of the "cloud computer" story: the user can always walk up to their machine and use it. Print mode is headless and non-interactive — there's no TUI for the user to attach to.

3. **The JSONL files provide the same structured data.** Everything print mode's stream-json gives us (typed events, tool calls, partial messages), the JSONL session files already provide. We don't need stdout parsing when we have the session transcript on disk.

**User access paths to the Claude session:**

| Access method | How it works |
|---|---|
| Helix UI (primary) | Helix reads JSONL for display, sends prompts via paste-buffer. Rich overlay UX |
| Desktop stream | User opens a terminal in their streamed desktop, runs `tmux attach -t claude`. Full interactive TUI |
| SSH / terminal PTY | User SSHes into their container, runs `tmux attach -t claude`. Full interactive TUI |
| Helix web terminal | If Helix exposes a web terminal (xterm.js), user can attach to the tmux session directly |
| Helix TUI (future) | A TUI client with an embedded PTY session can stream the tmux pane directly — no desktop stream needed |

All of these work simultaneously. The tmux session is the single source of truth — Helix's UI overlay and the user's direct terminal access are both views into the same session. The user can watch Claude work in the Helix UI, then switch to the terminal to type something directly, and both views stay in sync via the JSONL file.

#### Pros

- **Unambiguously allowed** — user running CLI with their own subscription on their own cloud computer. Helix provides the computer, not the agent
- **No credential management by Helix** — user logs in themselves, Helix never sees or touches their tokens
- **Rich structured data via JSONL tailing** — full access to thinking, text, tool calls, usage stats, subagent transcripts
- **No cost change for users** — subscription pricing preserved
- **More data than ACP** — thinking blocks, token usage, subagent transcripts all in JSONL
- **Clean architectural story** — Claude CLI talks directly to api.anthropic.com from the user's container. No intermediary

#### Cons

- Mode 1 users don't get Zed ACP features (inline diffs, tool approvals in UI) — but Zed ACP remains available for API key users in Mode 2
- tmux prompt injection is "good enough" but less robust than a typed API
- Need to build the JSONL tailing daemon and Helix UI sync layer (see feature parity mapping above)
- Need dotfile backup/restore for `~/.claude/` persistence across container lifecycles
- `--dangerously-skip-permissions` bypasses all safety checks — need to evaluate whether this is acceptable for users, or if Helix should detect and relay approval prompts

#### Interaction Model: User-Initiated Actions with Prompt Templates

Helix provides a cloud computer with a guided development flow. Every Claude session and every prompt is initiated by a user action (clicking a button, selecting a task). Helix sends prompt templates into the tmux session via clipboard paste (paste-buffer) — this is functionally identical to a user pasting a saved snippet, shell alias, or slash command into their own terminal.

**What Helix does (as a cloud computer provider):**
- User clicks "Start task" → Helix opens a tmux pane on the user's container, user's Claude starts
- User clicks "Write specs" → Helix pastes a prompt template into the terminal (like clipboard paste)
- User clicks "Run tests" → Helix pastes another prompt template
- Helix reads local JSONL files to show a richer view of what Claude is doing (like any file reader)

**What Helix does NOT do (not an agent harness):**
- Does NOT start Claude sessions without user action
- Does NOT chain prompts automatically (each step requires user initiation)
- Does NOT run headless batch jobs against the subscription
- Does NOT multiplex one subscription across multiple users
- Does NOT pool, proxy, or manage credentials
- Does NOT route API requests through Helix's backend — Claude talks directly to api.anthropic.com

The usage pattern is indistinguishable from a user on a VM who has shell aliases or saved snippets for common Claude prompts. The user is always in the driver's seat. Helix is the computer, not the driver.

#### Policy Risk Assessment

**Reading JSONL files:** No risk. These are local files on the user's own filesystem inside their container. Any tool can read local files — VS Code, backup scripts, monitoring tools, terminal emulators. No policy prohibits reading files Claude writes to disk on the user's machine.

**Prompt injection via tmux paste-buffer:** Low risk, with caveats. There's a spectrum:

| Action | Risk | Helix equivalent |
|--------|------|---|
| User types in terminal | None | — |
| User pastes text into terminal | None | This is what paste-buffer does |
| User clicks Helix button → paste-buffer sends a prompt | Low — clipboard paste triggered by user action | **Helix operates here** |
| Helix auto-sends prompts without user action | Higher — looks like programmatic automation | **NOT what Helix does** |

Helix stays at the "user clicks button → clipboard paste" level. Every prompt is user-initiated.

**The economic argument is the strongest defence.** Anthropic's stated concern is that third-party tools bypass prompt cache optimizations and consume disproportionate compute. A user running interactive Claude Code in a tmux session has the exact same compute profile as a user running it in any terminal. The Claude CLI manages its own prompt caching, context window, and session state. Helix isn't wrapping the API, isn't bypassing caching, isn't multiplexing subscriptions. It's providing a computer for the user to run Claude on.

**If Anthropic ever questions it, the defence is:** "Helix provides cloud computers for agents to run on — Linux containers with a desktop environment. The user installed Claude Code on their machine, logged in with their own account, and runs it in their terminal. We read the local session files to show a richer UI overlay, the same way VS Code or any terminal emulator reads local files. Every Claude session is started by a user action, every prompt is sent by a user action. We don't wrap the API, we don't manage credentials, we don't route requests through our backend. Claude talks directly to api.anthropic.com from the user's container."

**Risk escalates if** Helix crosses the line from "cloud computer" to "agent harness":
- Fully autonomous agent loops (prompts sent without user clicks) — this is orchestration, not computing
- Multiple concurrent Claude sessions per user — this is multiplexing, not personal computing
- Print mode (`-p`) instead of interactive mode — this is programmatic API usage, not terminal usage
- Any credential management by Helix itself — this is proxying, not providing a computer
- Auto-chaining prompts based on Claude's output — this is an agent loop, not a user-driven flow

Keep these as lines not to cross without legal review or Anthropic partner approval. The positioning — "computers for agents to run on" — only holds if every Claude interaction traces back to a user action.

#### Risk: Low policy risk, medium engineering effort.

The JSONL tailing gives Helix *more* data than the ACP integration (thinking blocks, token usage, subagent transcripts), in a structured format. The main engineering effort is building the sync daemon and mapping JSONL events to Helix's UI — see the feature parity mapping above for the complete mapping. The auth story is trivial — user logs in on their own computer, same as any machine.

### Option E: Hybrid — CLI for Auth, ACP for UX

Use the Claude Code CLI for authentication (which validates the subscription), but continue using the ACP integration for the IDE experience. This would require investigation into whether the CLI's auth session can be shared with the Agent SDK.

**Implementation:**
- User runs `claude login` in the container terminal (creates `~/.claude/` auth files)
- The Agent SDK / ACP integration picks up the same auth session
- Need to verify: does the Agent SDK respect CLI auth files, or does it use its own auth path?

**Pros:**
- Preserves UX of ACP integration
- User authenticates via official CLI flow

**Cons:**
- May not work — the Agent SDK might have its own auth enforcement separate from CLI
- Even if it works technically, it may still violate the policy (the requests still go through the SDK)
- Anthropic could detect and block this at any time

**Risk: High.** This is likely the same thing Anthropic is blocking — it's just a different path to the same prohibited outcome.

## Is Zed an Approved Partner?

**No confirmed formal partnership.** No public announcement of a Zed-Anthropic partnership was found. However, there is strong evidence of an active working relationship:

**Evidence of close collaboration:**
- Anthropic employee `blois` was **assigned to and fixed** [issue #205](https://github.com/anthropics/claude-agent-sdk-typescript/issues/205), which was explicitly about the Zed ACP integration (`@zed-industries/claude-agent-acp`). They didn't say "this isn't a supported use case" — they fixed the bug.
- Multiple ACP-related issues ([#117](https://github.com/anthropics/claude-agent-sdk-typescript/issues/117), [#254](https://github.com/anthropics/claude-agent-sdk-typescript/issues/254), [#261](https://github.com/anthropics/claude-agent-sdk-typescript/issues/261), [#265](https://github.com/anthropics/claude-agent-sdk-typescript/issues/265)) are actively triaged by Anthropic, all referencing `claude-agent-acp`.
- Zed has a [dedicated Claude Code page](https://zed.dev/acp/agent/claude-code) on their ACP registry, describing it as *"Anthropic's Claude integrated through Zed's SDK adapter"*.
- The `@zed-industries/claude-code-acp` package has 189K+ npm downloads.

**However:**
- Zed's own ACP page already instructs users to use API key auth: *"If this is your first time using Claude Code, you'll be prompted to add your Anthropic API key."*
- No public blog post or announcement from either Zed or Anthropic describes a formal partnership or approved status.
- Zed may itself be in the same grey area as Helix, or may have a private agreement we can't see.

**Bottom line:** Zed appears to be a **de facto** partner (Anthropic actively supports their integration) but whether they have formal "previously approved" status for subscription OAuth is unknown. This is worth asking Zed directly — they may already have clarity on this, and if they have an agreement, Helix (as a Zed consumer) may be able to piggyback on it.

## How to Contact Anthropic

The legal page provides a direct link for auth questions:

> **For questions about permitted authentication methods for your use case, please [contact sales](https://www.anthropic.com/contact-sales).**

Full URL: `https://www.anthropic.com/contact-sales?utm_source=claude_code&utm_medium=docs&utm_content=legal_compliance_contact_sales`

**Key people:**
- **Boris Cherny** — Head of Claude Code at Anthropic. Announced the April 4 enforcement on X. He's the decision-maker on this policy.
- **Anthropic Sales team** — The docs explicitly route auth questions through sales. This is the official path.

**Parallel channel: Ask Zed directly.** Since Helix uses Zed's ACP integration, Zed may have already navigated this question. They may have a partner agreement, or they may be able to introduce Helix to their Anthropic contact. Nathan Sobo (Zed CEO) or the Zed team would be the people to ask.

## Recommendation: Two Modes

Helix provides cloud computers. Users choose how to run Claude on them.

### Mode 1: "Pure Claude" (Subscription auth — tmux + JSONL)

For users with Claude Pro/Max subscriptions. The user runs Claude Code CLI directly in their Helix container — Helix just provides the computer. Helix offers prompt templates via clipboard paste (tmux paste-buffer) and powers a richer UI by reading the local JSONL session files. User logs in once with `claude auth login`; dotfiles are preserved across sessions via standard backup/restore (like any cloud dev environment).

**Auth:** User's own subscription. User authenticates directly with Anthropic — Helix never sees or touches credentials.
**UX:** Terminal-based Claude with Helix UI overlay powered by JSONL file tailing.
**Policy:** Unambiguously compliant — user running CLI on their own cloud computer. No different from running Claude in Codespaces or a remote VM.

### Mode 2: "Zed ACP" (API key auth — full IDE integration)

For users with Anthropic API keys. Uses the existing Zed ACP → Agent SDK integration for the richer IDE experience (inline diffs, tool approvals in UI, etc.). User provides an `ANTHROPIC_API_KEY` which Helix injects into the container.

**Auth:** API key via console.anthropic.com. Per-token billing.
**UX:** Full Zed ACP integration — richer than pure CLI mode.
**Policy:** Fully compliant — API key auth is the documented path for Agent SDK.

### Dotfile Persistence Across Sessions

Users shouldn't have to reconfigure their environment every time they start a new Helix session. Helix backs up and restores the user's dotfiles across container lifecycles — a general-purpose feature of any cloud dev environment, not Claude-specific. This covers `~/.claude/`, `~/.gitconfig`, `~/.ssh/`, `~/.config/`, shell rc files, and anything else the user has in their home directory.

Claude auth persistence is a natural consequence: the user logs in once, their `~/.claude/` directory is preserved like any other dotfile. No Claude-specific auth code. This is the same approach used by Codespaces (dotfiles repo), Gitpod (user settings sync), and DevPod (persistent volumes).

### Summary

| | Pure Claude (Mode 1) | Zed ACP (Mode 2) |
|---|---|---|
| **Auth** | Claude subscription (Pro/Max) | Anthropic API key |
| **Cost model** | Flat-rate subscription | Per-token billing |
| **Integration** | tmux + JSONL tailing | Zed ACP + Agent SDK |
| **UX richness** | Terminal + Helix overlay | Full IDE integration |
| **Policy risk** | None — user on their cloud computer | None — API key is documented path |

**In parallel:** Still pursue contacting Anthropic/Zed about partner approval. If approved, Mode 2 could also support subscription auth, collapsing both modes into one.

## Key Learnings

1. **The Agent SDK is explicitly covered by the subscription ban.** The docs say "including the Agent SDK" and "including agents built on the Claude Agent SDK." No ambiguity.
2. **"Unless previously approved" exists.** Anthropic has a partner exception path — this is the most important finding for Helix.
3. **The ban is about routing, not code ownership.** Even though the SDK is Anthropic's code, the policy cares about *who built the product* that's using it, not whose code makes the API call.
4. **Enforcement is real and recent.** April 4, 2026 enforcement confirmed by Boris Cherny. OpenClaw/OpenCode broken. This isn't theoretical.
5. **The economic rationale matters.** Anthropic says third-party tools bypass prompt cache optimizations and consume more compute. If Helix can demonstrate it doesn't cause this problem (or is willing to work with Anthropic on it), that strengthens the partner approval case.
6. **Team and Enterprise plans may be different.** The legal page mentions Team and Enterprise under OAuth but the restriction language focuses on "Free, Pro, or Max plan credentials." Enterprise customers with commercial agreements may have more flexibility — worth investigating.
