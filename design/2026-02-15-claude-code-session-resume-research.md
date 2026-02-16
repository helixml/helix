# Claude Code ACP Session List & Resume: Research

**Date:** 2026-02-15
**Status:** Research complete

## Summary

**Good news: `@zed-industries/claude-code-acp` already supports session list, load, resume, and fork.** No hacking needed -- unlike qwen-code where we had to build this ourselves, Zed's official Claude Code ACP wrapper already implements the full session management protocol.

The only thing we needed to do (and already did) was enable the `AcpBetaFeatureFlag` in our Zed fork (commit `4e870012b0`, 2026-02-08) which gates `session/list`, `session/load`, and `session/resume` in Zed's agent panel UI.

## What claude-code-acp Supports

From the source at https://github.com/zed-industries/claude-code-acp:

### Initialize Response (capabilities advertised)
```json
{
  "agentCapabilities": {
    "loadSession": true,
    "sessionCapabilities": {
      "fork": {},
      "list": {},
      "resume": {}
    }
  }
}
```

This is a superset of what qwen-code advertises (`loadSession: true`, `sessionCapabilities: { list: {} }` only).

### ACP Methods Implemented

| Method | Supported | Notes |
|--------|-----------|-------|
| `session/list` (`unstable_listSessions`) | Yes | Reads JSONL files from `~/.claude/projects/<encoded-path>/`, cursor-based pagination, title from first user message |
| `session/load` (`loadSession`) | Yes | Finds session file, replays history via `session/update` notifications |
| `session/resume` (`unstable_resumeSession`) | Yes | Resumes without full history replay |
| `session/fork` (`unstable_forkSession`) | Yes | Creates new session ID preserving history |

### Session Storage

Sessions stored at `~/.claude/projects/<encoded-path>/<sessionId>.jsonl` -- this is Claude Code's native session format. The ACP wrapper reads these directly, no custom storage needed.

## Comparison with qwen-code

| Feature | qwen-code (hacked) | claude-code-acp (native) |
|---------|--------------------|-----------------------|
| Session list | Custom implementation reading `~/.qwen/` JSONL | Native, reads `~/.claude/` JSONL |
| Session load | Custom with `setImmediate` deferred replay | Native with synchronous replay |
| Session resume | Not advertised | Yes |
| Session fork | Not supported | Yes |
| Method prefix | Unprefixed (`listSessions`) | `unstable_` prefixed (newer spec) |
| Pagination | Numeric mtime cursor | Base64-encoded offset cursor |

## What's Needed to Make It Work in Helix

1. **Zed feature flag** -- Already done (commit `4e870012b0`)
2. **Claude Code ACP wrapper** -- Already handled by Zed internally (`@zed-industries/claude-code-acp`)
3. **Session persistence** -- Already works. Claude Code stores sessions as JSONL at `~/.claude/projects/`. As long as the container persists between reconnects, sessions survive.

### Potential Issues to Verify

1. **Container lifecycle**: Sessions persist in `~/.claude/` inside the container. If the container is destroyed and recreated, sessions are lost. Need to verify that container reuse works correctly for `claude_code` sessions (same as it does for `qwen_code`).

2. **CLAUDE_DATA_DIR or similar env var**: qwen-code uses `QWEN_DATA_DIR` to override the data directory. Check if claude-code-acp respects a similar env var for controlling where session files go. This matters for bind-mounted persistent storage.

3. **`unstable_` method prefix**: The methods are prefixed with `unstable_` which means they may change. The Zed ACP code handles both prefixed and unprefixed variants, so this should be fine.

## Global Provider Question

**Can user providers (including Claude subscriptions) be shared globally?**

**Short answer: No for regular providers. Partially for Claude subscriptions.**

- **User providers** (`ProviderEndpointTypeUser`) are strictly private -- only accessible by their owner. No mechanism exists to promote them to global.
- **Global providers** (`ProviderEndpointTypeGlobal`) can only be created by admins.
- **Claude subscriptions** have org-level fallback: `GetEffectiveClaudeSubscription()` checks user-level first, then falls back to org-level subscriptions. So an admin can create an org-level Claude subscription that all org members can use.
- **No equivalent org-level fallback exists for regular API key providers.**
