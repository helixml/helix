# Duplicate `claude-agent-acp` spawn breaks chrome-devtools MCP

## Symptom

In a resumed spec task agent session (`spt_01kpx1cn84drg8mea69s6wbyvv`,
session `f163a349-f8a3-447b-9789-d7a36e6fb66b`), the model says
"No such tool available: mcp__chrome-devtools__navigate_page" for every
chrome-devtools call. Other stdio MCPs (`github`, `drone-ci`) and HTTP MCPs
(`helix-desktop`, `helix-session`) work fine. Zed's own UI shows
chrome-devtools as connected with all tools listed — but those are Zed's
context_server tools, not the agent's.

The same session worked perfectly the day before (2026-04-23 11:30–17:02,
dozens of chrome-devtools calls succeeded). The break appeared 16 minutes
after the container was restarted today.

## Root cause

Two concurrent `claude` processes are running, both with
`--resume f163a349-f8a3-447b-9789-d7a36e6fb66b`, each spawned by its own
`npm exec @agentclientprotocol/claude-agent-acp@0.30.0` wrapper:

```
npm exec @agent(7966) → claude(8028)  children: github, drone-ci          ← active, broken
npm exec @agent(7973) → claude(8041)  children: chrome-devtools, github, drone-ci
```

`claude` 8028 — the one the model is actually talking to — has no
chrome-devtools-mcp child process at all. It successfully spawned `github`
and `drone-ci` stdio MCPs but chrome-devtools never started (or crashed
silently during init). The other claude (8041) has all three but is unused.

A fresh `claude` spawned manually with the exact same `--mcp-config` JSON
in the same container reports `chrome-devtools: connected` with 29 tools.
So chrome-devtools-mcp 0.23.0, the stdio transport, and the SDK all work
fine when there's only one claude per session.

The chrome-devtools-mcp at PID 6781 visible in `ps` is owned by Zed itself
(parent chain via PID 6315, `npm exec` directly under Zed's process tree),
not by either claude. That's why Zed shows the tools.

## Earlier dedup work, and why it isn't catching this case

Prior fixes for related dedup races (all still in place):

- zed `826d32faae` (Apr 2) — `THREAD_LOAD_IN_PROGRESS` lock in
  `crates/external_websocket_sync/src/thread_service.rs` to prevent
  workspace restore + `open_thread` from both spawning load tasks.
- helix `d66bfd20e` (Mar 20) — `findSessionByZedThreadID` dedup in
  `websocket_external_agent_sync.go` to stop creating multiple Helix
  sessions for the same Zed thread.

Six more zed-side commits between Apr 2 and now strengthened the load lock
(`d470dac687`, `48de0cf877`, `f3a2622736`, `40a88fde39`, `48700037d0`,
`22f94a8bbb`). The `48700037d0 Add logging to observe whether thread load
guard fires in practice` commit is itself a tell — someone was already
unsure the guard was catching duplicates.

The current guard is a single mutex (post-`40a88fde39`), with a
"wait then fast-path" branch when `open_thread` arrives during panel
restoration. So duplicate load of the *same* `acp_thread_id` should be
deduped.

## Hypotheses for the regression

1. **Two different `acp_thread_id`s targeting the same claude session.**
   The lock dedups per thread ID, not per `--resume` session ID. If two
   distinct Zed threads both pass `--resume f163a349-...` to claude, both
   pass the lock independently and both spawn ACP wrappers.
2. **A spawn path that bypasses the load lock entirely.** Some path
   creates an `AgentConnection` (and therefore spawns
   `claude-agent-acp`) without going through `open_existing_thread_sync`
   or `load_agent_thread`.
3. **Sequential, not concurrent, spawns.** Workspace restore acquires and
   releases the lock; some later trigger (websocket reconnect, retry,
   etc.) re-spawns after the lock is gone. Lock as written only catches
   *concurrent* loads.

## Next debug step

Existing `[THREAD_SERVICE]` eprintln logs (added in `48700037d0`) are
sufficient. After reproducing, look for either:

- Two `🔒 Acquired thread load lock for <id>` lines with **different**
  `acp_thread_id` values that resolve to the same `agent_id` — confirms
  hypothesis 1.
- An ACP wrapper spawn (visible in `ps -ef | grep claude-agent-acp`)
  with **no** preceding `🔒 Acquired thread load lock` log line —
  confirms hypothesis 2.
- Two `🔒 Acquired` lines for the **same** `acp_thread_id` separated by
  a `🔓 Released` — confirms hypothesis 3.

## Workaround (until fixed)

Restart the spec task agent. A fresh container gets a single claude /
single ACP wrapper and chrome-devtools tools work normally.

## Files of interest

- `crates/external_websocket_sync/src/thread_service.rs` (zed) — load lock
  implementation, lines 55–95 and 1575–1700.
- `crates/external_websocket_sync/src/websocket_sync.rs` (zed) —
  `handle_open_thread` at line 499.
- `api/pkg/external-agent/zed_config.go` (helix) — MCP config that lists
  `chrome-devtools` as a stdio MCP (line 246).
- `api/pkg/server/websocket_external_agent_sync.go` (helix) —
  `findSessionByZedThreadID` dedup, lines 755 / 1028 / 2145.
