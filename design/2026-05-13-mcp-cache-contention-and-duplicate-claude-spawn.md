# MCP cache contention and duplicate Claude spawn in spec-task containers

**Status**: PR #2418 shipped (npx-cache fixes + Fix 1a + Fix 2). Fix 1b deferred — see "Why Fix 1b was deferred" below.

**Reporters**: lukemarsden + claude-code (live debugging session 2026-05-12 → 2026-05-13)

**Spec-task example**: `https://meta.helix.ml/orgs/helix/projects/prj_01kg02vqqyg178c1n2ydscn5fb/tasks/spt_01kqc4ev5rt9rknk6g8dbkzj9a`

## Problem statement

Spec-task containers fail to register `chrome-devtools` and `github` MCP
servers in Zed. Tools surface in the agent panel as `Error: No such tool
available: mcp__chrome-devtools__*` / `mcp__github__*`. Agents call those
tools, get the error, and either give up or work around them.

Zed log:

```
ERROR [project::context_server_store] chrome-devtools context server failed
        to start: Context server request timeout
ERROR [project::context_server_store] github context server failed to start:
        Context server request timeout
```

Both fail at exactly the 180s `context_server_timeout` mark — every container
restart, every MCP toggle, deterministic.

## Investigation timeline

### Bottom layer: npm `_npx/<hash>` cache contention

Manual reproduction with the exact protocol version, env, and shell-wrapping
Zed uses (`sh -c "npx -y @modelcontextprotocol/server-github"` with NPM_CONFIG_CACHE
pointing at Zed's cache, `clientInfo.name = "Zed"`, `protocolVersion = "2025-11-25"`)
returns `initialize` in **<2 seconds**. So the MCP server itself is fine,
the protocol is fine, the shell-wrapping is fine.

Looking at `/home/retro/.local/share/zed/node/.../cache/_logs/`, every `npm
exec` does a "reify mark retired" rename dance:

```
17 verbose shrinkwrap failed to load node_modules/.package-lock.json missing from lockfile: node_modules/chrome-devtools-mcp
24 silly reify mark retired [
24 silly reify   '/home/retro/work/.zed-state/local-share/node/node-v24.11.0-linux-x64/cache/_npx/15c61037b1978c83/node_modules/chrome-devtools-mcp',
24 silly reify   '/home/retro/work/.zed-state/local-share/node/node-v24.11.0-linux-x64/cache/_npx/15c61037b1978c83/node_modules/.bin/chrome-devtools-mcp',
24 silly reify   '/home/retro/work/.zed-state/local-share/node/node-v24.11.0-linux-x64/cache/_npx/15c61037b1978c83/node_modules/.bin/chrome-devtools'
24 silly reify ]
25 silly reify moves {
25 silly reify   '/home/retro/work/.zed-state/local-share/node/node-v24.11.0-linux-x64/cache/_npx/15c61037b1978c83/node_modules/chrome-devtools-mcp': '/home/retro/work/.zed-state/local-share/node/node-v24.11.0-linux-x64/cache/_npx/15c61037b1978c83/node_modules/.chrome-devtools-mcp-avtYTuFI',
   ...
```

`npm` renames the existing package directory, reinstalls, then renames it
back. When two `npx` invocations target the same package against the same
cache directory in parallel, the renames race. The losing process gets a
half-renamed install, JSON-RPC `initialize` never returns, the loser blocks
forever.

`drone-ci-mcp` was the only MCP that consistently worked — because
`@helix/drone-ci-mcp` is **globally installed** at `/usr/bin/drone-ci-mcp`
(via Dockerfile.ubuntu-helix), so `npm exec drone-ci-mcp` resolves the
binary via PATH and skips the `_npx` cache entirely.

### Middle layer: who spawns the parallel `npx` invocations?

Each Claude ACP session ships a `--mcp-config` JSON listing every MCP server
the agent should connect to, and Claude spawns one child process per server.
When multiple Claude sessions exist in the same container, each spawns its
own independent copies of every MCP — they all hit the same `_npx` cache.

`ps -eo pid,ppid,cmd` snapshot from a live container with two Claude ACP
sessions:

```
13169 npm exec @agentclientprotocol/claude-agent-acp@0.0.0 - 0.33.1
  └─ 13180 claude --resume 2b97182c-78a4-4910-89e8-27dde68600cb --mcp-config '{...4 stdio MCPs...}'
  │    ├─ npm exec chrome-devtools-mcp@latest      → races with 13442
  │    ├─ npm exec @modelcontextprotocol/server-github → races with 13443
  │    └─ npm exec drone-ci-mcp ...                → finds /usr/bin/drone-ci-mcp, OK
  │
  └─ 13503 claude --session-id e637e8be-... --mcp-config '{...same 4 MCPs...}'
       ├─ npm exec chrome-devtools-mcp@latest      → races with 13260
       ├─ npm exec @modelcontextprotocol/server-github → races with 13261
       └─ npm exec drone-ci-mcp ...                → finds /usr/bin/drone-ci-mcp, OK
```

Plus Zed itself spawns its own copies for the agent panel's
`context_servers`, so the worst-case concurrency is:

- **Zed**: 5 MCP processes (chrome-devtools, github, drone-ci, helix-session, helix-desktop)
- **Each Claude session**: 5 more MCP processes
- **3 Claude sessions × 5 + Zed's 5 = up to 20 MCP spawns concurrently**

The `_npx` rename race scales with how many Claude sessions exist.

### Top layer: where do the multiple Claude sessions come from?

The user reports they never manually clicked "New Chat" yet
`spt_01kqc4ev5rt9rknk6g8dbkzj9a` accumulated **10 different
`spec_task_zed_threads` rows**, each tied to a separate `helix_session_id`:

```sql
SELECT helix_session_id, COUNT(*) FROM spec_task_zed_threads z
JOIN spec_task_work_sessions w ON z.work_session_id = w.id
WHERE z.spec_task_id = 'spt_01kqc4ev5rt9rknk6g8dbkzj9a'
GROUP BY helix_session_id;

         helix_session_id        | count
---------------------------------+-------
 ses_01kqc4exe1276gn3xymyqxxvn4 |     1   ← real, 12 interactions
 ses_01krash8dg1bnhnmmhwnssdk1d |     1   ← real, 8 interactions
 ses_01kqc97mvjbkx1x903cm43htyh |     1   ← phantom "New Chat", 0 interactions
 ses_01kqzpqxh2gym8fyc87pn618hy |     1   ← phantom "New Chat", 0 interactions
 ses_01kr4kqzqgftr3c74ybr6s7zta |     1   ← phantom "New Chat", 0 interactions
 ses_01kraska10y388fk70qmaaxwp8 |     1   ← phantom "New Chat", 0 interactions
 ses_01krbenb4rtpa3rb73vawmt4zx |     1   ← phantom "New Chat", 0 interactions
 ses_01krbm1wcdz86hmag0fj1cgcz8 |     1   ← phantom "New Chat", 0 interactions
 ses_01krbnmb2h24z6p4e1dakvprzm |     1   ← phantom "New Chat", 0 interactions
 ses_01kredh1g92zta5sqes6y4bx6b |     1   ← phantom "New Chat", 0 interactions
```

8 of 10 are zero-interaction "New Chat" rows. Each accumulated on a
container restart.

### Root cause: Zed's draft thread fires `UserCreatedThread` on container restart

Each container restart of a long-running spec task triggers up to **three
distinct paths** that can spawn a Claude:

| Path | Trigger | Code | Calls Claude how |
|---|---|---|---|
| **A** | Zed workspace restore from `/home/retro/work/.zed-state/` | `agent_ui::agent_panel::restore` (panel deserialization) → `conversation_view.rs:1184-1193` with `resume_session_id=Some(saved_thread_id)` | `connection.load_session(saved_thread_id)` → `claude --resume <saved_thread_id>` |
| **B** | Helix sends `open_thread` WS message | `session_handlers.go:2073-2117` → `external_websocket_sync::websocket_sync::handle_open_thread` → `thread_service::open_existing_thread_sync` | `connection.load_session(helix_thread_id)` → `claude --resume <helix_thread_id>` |
| **C** | Agent panel `activate_draft` (the empty input box) | `agent_panel.rs:1923-1977` → `ensure_draft` → `create_agent_thread(None, ...)` → `conversation_view.rs:1184-1217` with `resume_session_id=None` | `connection.new_session()` → `claude --session-id <fresh>` |

**Dedup behavior:**
- A and B share the same `THREAD_LOAD_IN_PROGRESS` lock (`thread_service.rs:66`,
  acquired at `conversation_view.rs:1178` for A, at `thread_service.rs:1844`
  for B). If they target the same thread_id, only the first does the load
  and the second finds the thread in the registry on retry.
- C does **not** take the lock (`conversation_view.rs:1176-1181` only
  acquires it `if resume_session_id.is_some()`). C is independent of A/B.

**Worst-case spawn count per container restart:**

- A loads stale thread X → Claude #1 (`--resume X`)
- B loads current thread Y (different from X) → Claude #2 (`--resume Y`)
- C creates the draft thread → Claude #3 (`--session-id Z`)

The case observed in `spt_01kqc4ev5rt9rknk6g8dbkzj9a`'s yesterday container
was 2 Claudes — A and B presumably dedup'd to one (`--resume 2b97182c`),
plus C contributed the draft (`--session-id e637e8be`).

### Why the draft creates a phantom Helix session

`conversation_view.rs:1336-1351`:

```rust
#[cfg(feature = "external_websocket_sync")]
{
    if !is_resume {
        let thread_entity = &current.read(cx).thread;
        let acp_thread_id = thread_entity.read(cx).session_id().to_string();
        let title = thread_entity.read(cx).title().unwrap_or_default().to_string();
        let title_opt = if title.is_empty() { None } else { Some(title) };
        if let Err(e) = external_websocket_sync::send_websocket_event(
            external_websocket_sync::SyncEvent::UserCreatedThread {
                acp_thread_id,
                title: title_opt,
            }
        ) {
            log::error!("Failed to send UserCreatedThread WebSocket event: {}", e);
        }
    }
}
```

When the draft thread (path C) initializes, it goes through the `!is_resume`
branch and emits `UserCreatedThread` to Helix. Helix's
`handleUserCreatedThread` (`websocket_external_agent_sync.go:3870-3970`)
duly creates a fresh `helix_session` + `spec_task_work_session` +
`spec_task_zed_threads` row. The user never typed anything in this thread —
Zed created it speculatively as the empty input box.

**Timing dependency** (this is why the bug is intermittent):

Fresh container Zed log (`ses_01krg5fg354ctav92baw4yx8ev`):

```
08:58:54 ERROR [agent_ui::conversation_view] Failed to send UserCreatedThread WebSocket event: WebSocket service not initialized
08:58:59 INFO  [external_websocket_sync::thread_service] 🆕 Creating new ACP thread for request
08:59:08 INFO  [agent_servers::acp] [ACP_SESSION_LOCK] acquired slot (new_session cwd=/home/retro/work)
```

In this container the draft fired `UserCreatedThread` **before the WS was
connected** — the event was logged-and-dropped, no phantom session created.
For the long-running task, the WS connects faster (warm container) and the
event lands → phantom session created.

Whether the bug bites a particular container is timing-dependent: WS connect
vs panel restoration ordering.

## Cumulative effect on MCP startup

For a long-running spec task whose container has been restarted many times:

- N phantom Helix sessions accumulate in `spec_task_zed_threads`
- Zed's saved workspace state may reference any of them
- Each restart: A loads whatever was last open, B loads what Helix asks for,
  C creates a fresh draft → up to 3 Claudes
- Each Claude spawns 5 MCP processes (independently of Zed's 5)
- → 20 concurrent `npm exec` against the shared `_npx` cache → race → 180s
  timeout for the bigger packages

The MCP timeout symptom and the phantom-thread-accumulation symptom are
**the same root cause manifesting at different layers**.

## Fixes

### Shipped (PR #2418, merged)

1. **Pre-install `chrome-devtools-mcp` and `@modelcontextprotocol/server-github`
   globally** in `Dockerfile.ubuntu-helix` (next to existing
   `@helix/drone-ci-mcp`). When the global binary is in PATH, `npm exec
   <pkg>` finds it and skips the `_npx` cache → no rename race.
2. **`zed_config.go`** — point chrome-devtools at `/usr/bin/chrome-devtools-mcp`
   directly instead of `npx chrome-devtools-mcp@latest`.
3. **`simple_sample_projects.go`** + **`GitHubMcpSkill.tsx`** +
   **`AddLocalMcpSkillDialog.tsx`** + **`examples/project.yaml`** +
   **`docs/helix-apply.md`** — switch hardcoded GitHub MCP config from
   `npx -y @modelcontextprotocol/server-github` to the global
   `mcp-server-github` binary.
4. **`/usr/local/bin/npx` shim** (`desktop/shared/helix-npx.sh`) — gives
   each `npx` invocation its own NPM_CONFIG_CACHE so user-provided MCPs
   that genuinely need `npx` don't race. **Caveat**: Zed prepends its own
   bundled `~/.local/share/zed/node/.../bin` BEFORE `/usr/local/bin` in the
   PATH it gives Claude, so this shim is bypassed for Zed-launched MCPs.
   The shim still helps for npx invocations that resolve via the system
   PATH, and serves as defense-in-depth.

### Proposed (this design doc)

#### Fix 1a: Stop emitting `UserCreatedThread` for the panel's draft thread (Zed, small)

In `conversation_view.rs:1336` and `acp/thread_view.rs:1004`, the
`UserCreatedThread` WS event is sent for any non-resume `new_session`,
including the panel's permanent empty-input draft. The user never asked for
that thread to exist — Zed created it as scaffolding for the input box.

**Change**: defer `UserCreatedThread` emission until the user actually
sends their first message in that thread. Concretely, instead of firing in
the load-task completion handler at conversation_view.rs:1336, fire from
the `prompt` handler the first time it runs for a thread that has not yet
been registered with Helix.

This is in helixml/zed, not upstream Zed (the emission is gated behind the
`external_websocket_sync` feature).

**Effect**:
- ✅ No more phantom "New Chat" `helix_session` rows accumulating per
  restart.
- ❌ Path C still spawns a Claude eagerly. The draft thread's
  `connection.new_session()` call at `conversation_view.rs:1214` runs
  inside the load_task (lines 1140-1217), which fires synchronously when
  ConversationView is created. The `UserCreatedThread` emission happens
  AFTER that load_task returns — so suppressing the emission stops the
  phantom Helix session but does **not** stop the spawn. See Fix 1b.

#### Fix 1b: Lazily call `new_session()` for the draft thread (Zed, bigger)

The draft thread's `connection.new_session()` runs eagerly in
`ConversationView::initial_state`'s spawned load_task as soon as the panel
restores. This is what spawns the extra Claude process and brings up its
5 MCP children.

**Change**: when `resume_session_id.is_none()` (draft path), don't run
`connection.new_session()` in the load_task. Instead, store the connection
and resume_session_id=None in the ConversationView's state in a
"pending-new-session" form, and trigger the actual `new_session()` call
the first time the user submits a message in the draft.

**Caveat**: this changes the semantics of "draft thread is connected and
ready to receive input." Some UI affordances may rely on the connection
being live (e.g. autocomplete that hits the agent, model selectors that
query the connection). Need to enumerate those and decide whether they're
acceptable losses for a not-yet-used thread.

**Effect**:
- ✅ Path C no longer spawns a Claude on container restart. Best-case spawn
  count drops from 2 to 1 (just A or B for the active thread). Worst-case
  drops from 3 to 2.
- ✅ MCP startup contention drops correspondingly: 5 fewer concurrent
  `npm exec` invocations against the `_npx` cache per restart.

This is the bigger, structural fix and probably needs upstream Zed
discussion since the "draft thread always has a live connection" assumption
exists in upstream code too.

#### Fix 2: Helix-side dedup guard in `handleUserCreatedThread` (Helix)

Belt-and-braces. Even if Fix 1 lands, an old Zed binary in a long-lived
container will keep sending the event. In `handleUserCreatedThread`
(`websocket_external_agent_sync.go:3870`), before creating a new
`helix_session`, check whether the spec_task already has an active
`work_session` with no interactions. If so, log and skip — refuse to
register the phantom thread.

#### Fix 3: Pass `HELIX_ACP_THREAD_ID` env var into the container (Helix + Zed)

Eliminates path-A-vs-path-B divergence by giving Zed an authoritative
source of "which thread to load" that doesn't depend on saved state.
Helix sets `HELIX_ACP_THREAD_ID=<current_thread>` in the container env;
Zed's panel restoration prefers this env var over its saved state when
deciding which thread to resume. Saved-state references to other threads
become inert.

#### Fix 4: Make Zed forward MCP servers to Claude via the ACP wrapper instead of having both spawn independent copies (Zed, larger)

This is the structural fix to the MCP-doubling problem. The ACP protocol
forwards `mcp_servers` configs to Claude (via `acp.rs:3268
mcp_servers_for_project`), but Claude then spawns its own MCP children.
A more efficient design would multiplex through a single MCP-server pool
managed by Zed, exposing the running MCPs to Claude over the ACP RPC link
rather than re-spawning them.

This is a non-trivial Zed protocol change and is out of scope for the
immediate fix, but worth tracking.

## Why Fix 1b was deferred

Fix 1b (lazy `new_session()` for the draft thread) requires a meaningful
refactor: a new `ServerState` variant (`PendingDraftSession` between
`Loading` and `Connected`), a placeholder `ThreadView` that can render
the empty input editor without backing thread, plumbing for the message
editor's first-send to trigger the deferred `new_session()`, and an audit
of every UI affordance in the agent panel that today reads
`active_thread()` (model selector, mode toggle, tool-permission panel,
agent capabilities query, …).

That work is ~4–8 hours of careful change to upstream-touching code with
real potential to break other features. The user-visible symptoms — MCP
init timeouts and phantom "New Chat" session accumulation — are fully
resolved by PR #2418's npx-cache fixes (global installs + per-spawn cache
shim) plus Fix 1a (suppress speculative `UserCreatedThread`) and Fix 2
(Helix-side dedup safety net). The remaining cost without Fix 1b is one
"wasted" Claude process per container restart that nobody types into,
spawning five MCP children. With the npx cache contention eliminated
those children come up cleanly; they're just memory and CPU overhead the
user never sees benefit from.

Fix 1b becomes the right work when:
- We need to reduce per-container memory/CPU footprint (e.g. pushing more
  spec tasks onto the same hardware), or
- We touch the agent panel's draft-thread architecture for unrelated
  reasons and can fold this in.

Until then, Fix 1a covers the user-facing symptom and Fix 1b stays as a
roadmap item.

## Recommended order

1. ✅ **Shipped in PR #2418** — npx cache contention fixes (global installs +
   per-spawn cache shim) + **Fix 1a** (suppress speculative
   `UserCreatedThread`) + **Fix 2** (Helix-side dedup safety net).
2. ⏸️ **Fix 1b** — deferred (see "Why Fix 1b was deferred" above).
3. ⏸️ **Fix 3** (`HELIX_ACP_THREAD_ID` env passthrough) — only worth doing
   if path A vs B divergence is observed in production after 1a+2. Likely
   not needed.
4. ⏸️ **Fix 4** (multiplex MCPs through ACP) — longer-term roadmap item
   (separate design doc).

## Files referenced

| File | Purpose |
|---|---|
| `Dockerfile.ubuntu-helix:887-905` | Global MCP installs + `/usr/local/bin/npx` shim |
| `desktop/shared/helix-npx.sh` | Per-spawn isolated NPM_CONFIG_CACHE shim |
| `api/pkg/external-agent/zed_config.go:285-314` | chrome-devtools binary path |
| `api/pkg/server/simple_sample_projects.go:680-695` | GitHub sample MCP config |
| `api/pkg/server/websocket_external_agent_sync.go:3870-3970` | `handleUserCreatedThread` |
| `api/pkg/server/session_handlers.go:2073-2117` | `sendOpenThreadCommand` (path B) |
| `frontend/src/components/app/GitHubMcpSkill.tsx:163-185` | Project Settings → GitHub MCP creator |
| `frontend/src/components/app/AddLocalMcpSkillDialog.tsx:295-380` | Project Settings → Local MCP creator |
| zed `crates/agent_ui/src/conversation_view.rs:1184-1351` | new_session vs load/resume; UserCreatedThread emit |
| zed `crates/agent_ui/src/agent_panel.rs:1923-1978` | `activate_draft`/`ensure_draft` (path C) |
| zed `crates/external_websocket_sync/src/thread_service.rs:66-92,1820-2099` | `THREAD_LOAD_IN_PROGRESS` + `open_existing_thread_sync` |
| zed `crates/agent_servers/src/acp.rs:429,693,746,1088-1131,1425-1450,3268` | ACP_SESSION_LOCK + mcp_servers forwarding |
