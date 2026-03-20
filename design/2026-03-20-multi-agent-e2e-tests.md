# Multi-Agent E2E Test Coverage

**Date:** 2026-03-20
**Status:** In progress

## Problem

The Zed WebSocket sync E2E tests currently only test with `zed-agent` (Zed's built-in native agent). We ship three agent types:

1. **zed-agent** — Zed's built-in agent (NativeAgent, uses local SQLite for persistence)
2. **claude** — Claude Code via `@zed-industries/claude-agent-acp` (ACP agent, own persistence)
3. **qwen** — Qwen Code via custom binary (ACP agent, forked for session persistence)

The bug fixed in PR #1967 (`sendChatMessageToExternalAgent` missing `agent_name`) only affected non-native agents (claude, qwen) but was never caught because E2E tests only exercised zed-agent.

## Root Cause of PR #1967

`sendChatMessageToExternalAgent` sent `chat_message` commands without `agent_name`. For Claude Code sessions, Zed received `agent_name=null`, defaulted to `NativeAgent`, and tried to load the thread from local SQLite — failing because Claude Code threads exist only in ACP persistence, not Zed's SQLite.

## Design: Multi-Agent Test Rounds

### Architecture

Refactor `testDriver` in `helix-ws-test-server/main.go` to run "rounds":
- Each round executes all 9 test phases with a specific `agentName`
- Between rounds, reset per-round state (threadIDs, completions, phase counters)
- Same WebSocket connection shared across rounds (single Zed instance)
- Request IDs are namespaced per round: `req-phase1-zed-agent`, `req-phase1-claude`
- Validation runs per round with round-specific assertions

### Round State

```go
type roundState struct {
    agentName           string
    threadIDs           []string
    completions         map[string][]string
    // ... per-phase state (phase8ThreadID, etc.)
}
```

### Agent Configuration

| Agent | How it works in E2E |
|-------|-------------------|
| zed-agent | Already works. Uses Anthropic API via `ANTHROPIC_API_KEY` env var |
| claude | Zed auto-installs via npm (`@zed-industries/claude-agent-acp`). Needs nodejs in Dockerfile. API key via `agent_servers.claude.env.ANTHROPIC_API_KEY` in settings.json |
| qwen | Needs custom binary. Not auto-installable. **Deferred** (see roadmap) |

### Dockerfile Changes

Add to `Dockerfile.runtime`:
```dockerfile
RUN apt-get install -y nodejs npm
```

### Settings Changes

Update `run_e2e.sh` settings.json:
```json
{
  "agent_servers": {
    "claude": {
      "env": {
        "ANTHROPIC_API_KEY": "<injected from env>"
      }
    }
  }
}
```

### Phase Flow

```
Round 1: zed-agent
  Phase 1-9 (all existing tests)
  Validation for zed-agent round

Round 2: claude
  Phase 1-9 (same tests, agent_name="claude")
  Validation for claude round

[Future] Round 3: qwen
  Phase 1-9 (same tests, agent_name="qwen")
```

### sendOpenThread Fix

The `sendOpenThread` helper in the test server also needs `agent_name` — without it, Zed defaults to NativeAgent for `open_thread` too (same class of bug as PR #1967).

## Roadmap

### Now (this PR)
- [x] Fix `sendChatMessageToExternalAgent` missing `agent_name` (PR #1967)
- [ ] Refactor test driver for multi-agent rounds
- [ ] Add nodejs to Dockerfile.runtime for Claude Code
- [ ] Configure Claude Code API key in E2E settings
- [ ] Run all 9 phases for both `zed-agent` and `claude`

### Soon
- [ ] Investigate upstream Qwen ACP session persistence
  - Our fork (`~/pm/qwen-code`) added `resume` and `list_sessions` support before ACP officially supported these capabilities
  - Check if upstream Qwen Code now supports `session_capabilities.resume` in the ACP protocol
  - If upstream supports it, we can drop our fork and use the official package
  - If not, we need to keep the fork or contribute upstream
- [ ] Add Qwen Code as a third E2E test round (needs binary in container)

### Later
- [ ] Codex agent E2E testing
- [ ] Cross-agent thread migration tests (switch agent mid-session)
