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

## Confirmation via `[ACP_SPAWN]` logging

Added `log::info!` lines in `crates/agent_servers/src/acp.rs` around
`Child::spawn` (the actual `claude-agent-acp` spawn site). On a freshly
rebuilt container, `~/work/.zed-state/local-share/logs/Zed.log` showed:

```
12:50:02  [ACP_SPAWN] About to spawn ACP wrapper agent_id=AgentId("claude-acp") ...
12:50:02  [ACP_SPAWN] Spawned ACP wrapper pid=8203 agent_id=AgentId("claude-acp")
12:50:02  [ACP_SPAWN] About to spawn ACP wrapper agent_id=AgentId("claude-acp") ...
12:50:02  [ACP_SPAWN] Spawned ACP wrapper pid=8211 agent_id=AgentId("claude-acp")
```

**Same `agent_id` both times, same second** — confirms hypothesis 1 (same
agent connected twice), not hypothesis 2 (spawn bypasses load lock) or 3
(sequential acquire/release/acquire).

The race traces back to `crates/external_websocket_sync/src/thread_service.rs`
having THREE call sites for `server.connect()`:

| Line | Function                       | Triggered by                          |
|------|--------------------------------|---------------------------------------|
| 1121 | `create_new_thread_sync`       | new thread (not relevant to resume)   |
| 1465 | `load_thread_from_agent`       | panel restoration / open path         |
| 1676 | `open_existing_thread_sync`    | WebSocket `open_thread` command       |

For a resumed session, two of these (typically 1465 + 1676) fire near-
simultaneously. Each `connect()` independently spawns a fresh ACP wrapper,
fresh `claude --resume <session>`, and fresh MCP children. One wins the
npx race for `chrome-devtools-mcp`, the other doesn't. Worse: both
claudes scribble on the same on-disk session file (`~/.claude/projects/.../<session>.jsonl`).

The existing `THREAD_LOAD_IN_PROGRESS` lock added in `826d32faae` doesn't
help here — it dedups *thread loads*, not the upstream `connect()` calls
that happen first.

## Fix: process-global `AgentConnectionCache`

Implemented in `crates/agent_servers/src/connection_cache.rs` (new):

- GPUI `Global` keyed by `(Project entity_id, AgentId)`.
- Stores `Shared<Task<Result<Rc<dyn AgentConnection>, LoadError>>>` so
  concurrent callers for the same key share one in-flight connect Task.
- Once the underlying task resolves, the `Shared` caches the value;
  subsequent callers get it without re-spawning anything.
- Failed connects evict the entry so retry can re-attempt.

All four call sites that previously called `server.connect()` now go
through the cache:

- `external_websocket_sync::thread_service::create_new_thread_sync` (line ~1121)
- `external_websocket_sync::thread_service::load_thread_from_agent` (line ~1465)
- `external_websocket_sync::thread_service::open_existing_thread_sync` (line ~1676)
- `agent_ui::AgentConnectionStore::start_connection`

Net result: one `AgentConnection` (and therefore one `claude-agent-acp`
wrapper, one `claude --resume`, one set of MCP children) per
`(project, agent_id)` regardless of which path triggered first. The
multi-window-Zed scenario still gets one connection per project.

### Verification

After the fix, `Zed.log` should show one of:

- One `[ACP_DEDUP] No cached connection, calling server.connect()` followed
  by one `[ACP_SPAWN] About to spawn` — the only-caller case.
- One `[ACP_DEDUP] No cached connection` + one or more
  `[ACP_DEDUP] Reusing connection` for the same `(project, agent)` key,
  but still only one `[ACP_SPAWN] About to spawn`.

If two `[ACP_SPAWN] About to spawn` lines for the same agent_id appear,
the bug has resurfaced.

E2E suite (`crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`)
passes with the change in place (zed-agent round, in-memory store).

### Caveats

- The first caller's `AgentServerDelegate::new_version_available` watch
  channel is the only one wired to `server.connect()`. Subsequent
  callers' delegates are dropped unused. Best-effort UI signal — the core
  load behaviour is unaffected.
- `external_websocket_sync` is bound to a single `Entity<Project>` at
  startup (`init_with_project`). Multi-workspace Zed isn't supported by
  this protocol today regardless of the cache; the protocol carries no
  workspace identifier so whichever project initialises the sync first
  owns all incoming `open_thread` commands. Per-project cache keying is
  correct for both single- and (hypothetical) multi-workspace cases.

## Files of interest

- `crates/agent_servers/src/connection_cache.rs` (zed, new) — the cache.
- `crates/agent_servers/src/agent_servers.rs` (zed) — re-exports the cache.
- `crates/agent_servers/src/acp.rs` (zed) — `[ACP_SPAWN]` logs.
- `crates/external_websocket_sync/src/thread_service.rs` (zed) — three
  rewritten call sites.
- `crates/agent_ui/src/agent_connection_store.rs` (zed) — delegates to
  the cache.
- `api/pkg/external-agent/zed_config.go` (helix) — MCP config that lists
  `chrome-devtools` as a stdio MCP.
- `api/pkg/server/websocket_external_agent_sync.go` (helix) —
  `findSessionByZedThreadID` dedup (still useful, addresses a different
  layer).
