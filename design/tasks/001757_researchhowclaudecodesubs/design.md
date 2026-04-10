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
Helix Container
├── tmux server
│   └── pane 0: claude (interactive mode, subscription auth)
├── Helix sync daemon
│   ├── INPUT:  tmux send-keys → inject prompts/approvals into claude
│   └── OUTPUT: tail -f ~/.claude/projects/<cwd>/<session>.jsonl → parse & relay to Helix UI
└── Auth: user runs `claude login` or `claude setup-token` for subscription OAuth
```

#### Authentication in the Container

The CLI supports subscription auth via two paths:

1. **`claude auth login`** — Interactive OAuth flow. Opens a browser for login. In a headless container, this requires a URL to be displayed that the user clicks in their local browser, then a code is pasted back. The CLI already supports this headless flow.

2. **`claude setup-token`** — *"Generate a long-lived OAuth token for CI and scripts. Prints the token to the terminal without saving it. Requires a Claude subscription."* The user generates this on their local machine and provides it to Helix. This is the cleanest path for containers — no browser needed inside the container. The token gets saved to `~/.claude/` config.

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

#### Auth Token Management

The CLI has `claude setup-token` which generates a long-lived OAuth token for CI/scripts. This is the cleanest path for Helix containers:

1. User generates a setup token on their own machine: `claude setup-token`
2. User provides the token to Helix (stored securely)
3. Helix injects the token into the container's `~/.claude/` config at startup
4. Claude CLI in the container picks up the token — no interactive login needed

This avoids the browser-redirect problem entirely. The user authenticates on their own machine with their own subscription, and the container gets a derived token.

#### Pros (updated)

- Unambiguously allowed — CLI + subscription is the primary supported use case
- **Rich structured data via JSONL tailing** — full access to thinking, text, tool calls, usage stats
- No cost change for users
- `claude setup-token` solves the container auth problem cleanly
- Session files give more data than the ACP integration (e.g., thinking blocks, usage stats, subagent transcripts)

#### Cons (updated)

- Loses Zed ACP integration (inline diffs, tool approvals in UI)
- tmux injection is "good enough" but less robust than a proper API
- Need to build the JSONL tailing daemon and Helix UI sync layer
- TUI-based — harder to embed in a polished UI than structured ACP events
- `--dangerously-skip-permissions` bypasses all safety checks — need to evaluate risk

#### Risk: Low policy risk, medium engineering effort.

The JSONL tailing gives Helix nearly as much data as the ACP integration, in a structured format. The main engineering effort is building the sync daemon and mapping JSONL events to Helix's UI.

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

## Recommendation

**Build Option D (CLI + tmux + JSONL tailing) as the primary technical path.** This is the only approach that is both unambiguously compliant with Anthropic's policy AND preserves subscription-based auth for users. The JSONL session files provide rich structured data that can power a good Helix UI.

**In parallel:** Still pursue Option A (contact Anthropic/Zed about partner approval). If approved, Helix could switch back to the ACP integration for a richer UX. But don't block on this — Option D works today.

**Also support:** Option B (API key auth) as an alternative for users who prefer pay-per-token or need Team/Enterprise compliance.

**Avoid:** Option E (hybrid CLI auth + ACP) — too fragile, likely violates policy spirit.

## Key Learnings

1. **The Agent SDK is explicitly covered by the subscription ban.** The docs say "including the Agent SDK" and "including agents built on the Claude Agent SDK." No ambiguity.
2. **"Unless previously approved" exists.** Anthropic has a partner exception path — this is the most important finding for Helix.
3. **The ban is about routing, not code ownership.** Even though the SDK is Anthropic's code, the policy cares about *who built the product* that's using it, not whose code makes the API call.
4. **Enforcement is real and recent.** April 4, 2026 enforcement confirmed by Boris Cherny. OpenClaw/OpenCode broken. This isn't theoretical.
5. **The economic rationale matters.** Anthropic says third-party tools bypass prompt cache optimizations and consume more compute. If Helix can demonstrate it doesn't cause this problem (or is willing to work with Anthropic on it), that strengthens the partner approval case.
6. **Team and Enterprise plans may be different.** The legal page mentions Team and Enterprise under OAuth but the restriction language focuses on "Free, Pro, or Max plan credentials." Enterprise customers with commercial agreements may have more flexibility — worth investigating.
