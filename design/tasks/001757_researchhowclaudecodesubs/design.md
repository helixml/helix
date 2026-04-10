# Design: Claude Code Subscription Auth — Options for Helix

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

Helix's model is closer to a **cloud dev environment** (like Codespaces, Gitpod, or a remote VM) than a **third-party Claude wrapper** (like OpenClaw). The user isn't using an alternative Claude frontend — they're using Zed IDE inside a container that Helix provides. If the user SSHed into a VM and ran `claude` from the terminal with their own subscription, that would be unambiguously fine.

But the integration goes through the Agent SDK rather than the CLI directly, which crosses the policy line.

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

Instead of going through Zed ACP → Agent SDK, run the Claude Code CLI directly in a tmux session inside the container. Inject prompts via `tmux send-keys` and sync state back to Helix by tailing the session JSONL files.

This is the approach the reviewer favoured. Detailed technical investigation below.

#### Architecture

```
Helix Container (user's Linux VM)
├── tmux server
│   └── pane 0: claude (interactive mode, user's own subscription)
├── Helix sync daemon
│   ├── INPUT:  tmux send-keys → inject prompts/approvals into claude
│   └── OUTPUT: tail -f ~/.claude/projects/<cwd>/<session>.jsonl → parse & relay to Helix UI
├── Auth: user runs `claude auth login` in their terminal (standard OAuth, same as any machine)
└── ~/.claude/ (persistent volume — auth + session data survives container restarts)
```

#### Authentication in the Container

**Helix is a Linux VM from the user's perspective.** The user has a terminal in their Helix session. They log in to Claude exactly the same way they would on any machine:

```bash
claude auth login
```

The CLI displays a URL. The user clicks it in their browser, authenticates with their Claude subscription (Pro/Max/etc.), and pastes the code back into the terminal. This is the standard headless OAuth flow — identical to SSHing into any remote machine and running `claude login`. **Helix doesn't manage, store, or proxy any credentials.** The user is an ordinary user running Claude Code on their own machine, which happens to be a Helix container.

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

```bash
# Create session with wide terminal (avoids line wrapping in output)
tmux new-session -d -s claude -x 220 -y 50

# Start claude in interactive mode
tmux send-keys -t claude "claude --dangerously-skip-permissions" Enter

# Send a prompt (use -l for literal text to avoid escape char issues)
tmux send-keys -t claude -l "Fix the bug in auth.py" 
tmux send-keys -t claude Enter

# Send a tool approval (y/n)
tmux send-keys -t claude "y" Enter

# Send Ctrl+C to interrupt
tmux send-keys -t claude C-c
```

**Key tmux considerations:**
- **`-l` flag is critical** — without it, special chars like `$`, `"`, `\` are interpreted as key names
- **Terminal width matters** — set `-x 220` or wider to prevent line wrapping in the TUI
- **Timing:** `send-keys` is fire-and-forget with no acknowledgement. Before sending, poll `capture-pane` or check the JSONL to confirm Claude is ready for input
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

#### Alternative: `--output-format stream-json` (Print Mode)

The CLI also supports structured I/O in print mode:
```bash
claude -p "Fix the bug" --output-format stream-json --input-format stream-json
```

This emits typed JSON events to stdout including partial messages, tool calls, and results. However, print mode (`-p`) is documented as the "SDK mode" and may be subject to the same Agent SDK restrictions on subscription auth. **Interactive mode is safer for subscription compliance** — it's the primary way individual users run Claude Code.

#### Pros

- **Unambiguously allowed** — user running CLI with their own subscription is the primary supported use case. Helix is just the VM.
- **No credential management by Helix** — user logs in themselves, Helix never touches their tokens
- **Rich structured data via JSONL tailing** — full access to thinking, text, tool calls, usage stats, subagent transcripts
- **No cost change for users** — subscription pricing preserved
- **Session files give more data than ACP** — thinking blocks, token usage, subagent transcripts all in JSONL

#### Cons

- Loses Zed ACP integration (inline diffs, tool approvals in UI, etc.)
- tmux injection is "good enough" but less robust than a proper API
- Need to build the JSONL tailing daemon and Helix UI sync layer
- Need persistent volume for `~/.claude/` to survive container restarts
- `--dangerously-skip-permissions` bypasses all safety checks — need to evaluate whether this is acceptable for users, or if Helix should detect and relay approval prompts

#### Interaction Model: User-Initiated Actions with Prompt Templates

Helix provides a guided development flow. Every claude session and every prompt is initiated by a user action (clicking a button, selecting a task). Helix sends prompt templates into the tmux session via `send-keys` — this is functionally identical to a user pasting a saved snippet, shell alias, or slash command into their terminal.

**What Helix does:**
- User clicks "Start task" → Helix opens a tmux pane, starts `claude`
- User clicks "Write specs" → Helix pastes a prompt template into the terminal
- User clicks "Run tests" → Helix pastes another prompt template
- JSONL tailing powers a richer view of what Claude is doing

**What Helix does NOT do:**
- Auto-start sessions without user action
- Chain prompts automatically (each step requires user initiation)
- Run headless batch jobs against the subscription
- Multiplex one subscription across multiple users
- Pool or proxy credentials

The usage pattern is indistinguishable from a user on a VM who has shell aliases or saved snippets for common Claude prompts. The user is always in the driver's seat.

#### Policy Risk Assessment

**Reading JSONL files:** No risk. These are local files on the user's filesystem. Any tool can read files — VS Code, backup scripts, monitoring tools. No policy prohibits reading files Claude writes to disk.

**tmux send-keys for prompt injection:** Low risk, with caveats. There's a spectrum:

| Action | Risk |
|--------|------|
| User types in terminal | None |
| User pastes text into terminal | None |
| User clicks Helix button → send-keys pastes a prompt | Low — equivalent to clipboard paste |
| Helix auto-sends prompts without user action | Higher — looks like programmatic automation |

Helix stays at the "user clicks button" level. Every prompt is user-initiated.

**The economic argument is the strongest defence.** Anthropic's stated concern is that third-party tools bypass prompt cache optimizations and consume disproportionate compute. A user running interactive Claude in a tmux session has the same compute profile as a user running it in any terminal. Helix isn't wrapping the API, isn't bypassing caching, isn't multiplexing subscriptions.

**If Anthropic ever questions it, the defence is:** "We provide Linux containers. The user installed Claude Code, logged in with their own account, and runs it in their terminal. We read the session files to show a richer UI. Every Claude session is started by a user action, every prompt is sent by a user action."

**Risk escalates if** Helix later adds:
- Fully autonomous agent loops (prompts sent without user clicks)
- Multiple concurrent Claude sessions per user
- Print mode (`-p`) instead of interactive mode
- Any credential management by Helix itself

Keep these as lines not to cross without legal review or Anthropic partner approval.

#### Risk: Low policy risk, medium engineering effort.

The JSONL tailing gives Helix nearly as much data as the ACP integration, in a structured format. The main engineering effort is building the sync daemon and mapping JSONL events to Helix's UI. The auth story is trivial — user logs in, same as any machine.

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

Helix offers two Claude integration modes, matching auth type to integration approach:

### Mode 1: "Pure Claude" (Subscription auth — tmux + JSONL)

For users with Claude Pro/Max subscriptions. Runs the Claude Code CLI directly in a tmux session. Helix provides prompt templates via send-keys, and powers a richer UI by tailing the JSONL session files. User logs in once with `claude auth login`; Helix persists `~/.claude/` across sessions (see below).

**Auth:** User's own subscription. Helix never touches credentials.
**UX:** Terminal-based Claude with Helix UI overlay powered by JSONL tailing.
**Policy:** Unambiguously compliant — user running CLI on their own machine.

### Mode 2: "Zed ACP" (API key auth — full IDE integration)

For users with Anthropic API keys. Uses the existing Zed ACP → Agent SDK integration for the richer IDE experience (inline diffs, tool approvals in UI, etc.). User provides an `ANTHROPIC_API_KEY` which Helix injects into the container.

**Auth:** API key via console.anthropic.com. Per-token billing.
**UX:** Full Zed ACP integration — richer than pure CLI mode.
**Policy:** Fully compliant — API key auth is the documented path for Agent SDK.

### Auth Persistence: Copy `~/.claude/` Across Sessions

Users should not have to run `claude auth login` every time they start a new Helix session. The auth state lives in `~/.claude/` (OAuth tokens, config). Helix needs to persist this across container lifecycles.

**Approach:** Helix backs up and restores the user's dotfiles across container sessions — a general-purpose feature, not Claude-specific. This covers `~/.claude/`, `~/.gitconfig`, `~/.ssh/`, `~/.config/`, shell rc files, and anything else the user has in their home directory. Claude auth persistence is just a natural consequence of dotfile backup/restore, the same way any cloud dev environment (Codespaces, Gitpod, DevPod) handles it.

No Claude-specific auth code needed. The user logs in once, their `~/.claude/` directory is preserved like any other dotfile, and it's there next session.

### Summary

| | Pure Claude (Mode 1) | Zed ACP (Mode 2) |
|---|---|---|
| **Auth** | Claude subscription (Pro/Max) | Anthropic API key |
| **Cost model** | Flat-rate subscription | Per-token billing |
| **Integration** | tmux + JSONL tailing | Zed ACP + Agent SDK |
| **UX richness** | Terminal + Helix overlay | Full IDE integration |
| **Policy risk** | None — user on their VM | None — API key is documented path |

**In parallel:** Still pursue contacting Anthropic/Zed about partner approval. If approved, Mode 2 could also support subscription auth, collapsing both modes into one.

## Key Learnings

1. **The Agent SDK is explicitly covered by the subscription ban.** The docs say "including the Agent SDK" and "including agents built on the Claude Agent SDK." No ambiguity.
2. **"Unless previously approved" exists.** Anthropic has a partner exception path — this is the most important finding for Helix.
3. **The ban is about routing, not code ownership.** Even though the SDK is Anthropic's code, the policy cares about *who built the product* that's using it, not whose code makes the API call.
4. **Enforcement is real and recent.** April 4, 2026 enforcement confirmed by Boris Cherny. OpenClaw/OpenCode broken. This isn't theoretical.
5. **The economic rationale matters.** Anthropic says third-party tools bypass prompt cache optimizations and consume more compute. If Helix can demonstrate it doesn't cause this problem (or is willing to work with Anthropic on it), that strengthens the partner approval case.
6. **Team and Enterprise plans may be different.** The legal page mentions Team and Enterprise under OAuth but the restriction language focuses on "Free, Pro, or Max plan credentials." Enterprise customers with commercial agreements may have more flexibility — worth investigating.
