# Design: In-Place Agent Framework Switching on Running Sessions

## Summary

Add an in-place "switch agent" path that changes the agentic framework inside a
running session's existing Zed environment, instead of forking to a new
container/session. The switch reuses two mechanisms that already exist:

1. The **settings-sync daemon** to make the target agent's config available in
   the container (it already re-fetches `code_agent_config` on `config_changed`
   and rewrites `agent_servers` in `settings.json`).
2. The **external-agent WebSocket sync protocol**, where a `chat_message` with
   `acp_thread_id: null` already creates a *new* Zed thread bound to a supplied
   `agent_name`. Seeding that first message with the serialized prior transcript
   (the same serializer fork uses) repopulates the conversation.

The dropdown is rewired from the fork endpoint to a new switch endpoint. The
fork code stays intact.

## Two channels, and which one does what (critical mental model)

The user's request talks about "the Z settings daemon switching the agent". In
reality there are **two separate channels** to the container, and the switch
needs **both**:

| Channel | File | Role in the switch |
|---|---|---|
| **Settings-sync daemon WS** (`/api/v1/ws/user`, `config_changed`) | `api/cmd/settings-sync-daemon/main.go` | Pushes the *config* of the new agent into `settings.json` (`agent_servers`, model, MCP `context_servers`, credentials). Does **not** by itself switch a running thread. |
| **External-agent sync WS** | `api/pkg/server/websocket_external_agent_sync.go` ↔ `zed/crates/external_websocket_sync/` | Actually drives Zed threads. Sends `chat_message`/`open_thread` with `agent_name`. This is what creates the new thread bound to the new agent. |

Writing `settings.json` alone is necessary but **not sufficient**: a Zed thread
is bound to one agent at creation and cannot be re-pointed. So the switch =
(daemon makes new agent resolvable) **then** (external-agent WS creates a fresh
thread on the new agent + repopulates messages).

## Key facts established by research

- **Thread→agent binding is immutable.** `AcpThread.connection` is fixed at
  creation (`zed/crates/acp_thread/src/acp_thread.rs`). Switching frameworks
  always means a *new* thread. (Confirmed in `thread_service.rs`.)
- **`agent_name` is sent per message.** Helix puts `agent_name` in the
  `chat_message`/`open_thread` command data
  (`websocket_external_agent_sync.go`); Zed maps it
  (`thread_service.rs:~1405`): `claude`→`claude-acp`, `zed-agent`/none→native,
  others (`qwen`, `goose`) used as-is.
- **`acp_thread_id: null` ⇒ new thread.** Zed's `handle_chat_message`
  (`websocket_sync.rs:401`) creates a new thread when no thread id is supplied,
  bound to the supplied `agent_name`. On creation it emits `thread_created`,
  which Helix maps to the session (`handleThreadCreated`).
- **Configuring many `agent_servers` is cheap.** Subprocesses spawn lazily via
  `AgentConnectionCache` (`zed/crates/agent_servers/src/connection_cache.rs`);
  startup only validates/indexes the list.
- **MCP context servers are per-project, shared by all agents** in the Zed
  instance (`zed/crates/project/src/context_server_store.rs`) — they cannot be
  scoped per-agent. This is the decisive argument against "all agents at once".
- **Session already stores the relevant fields:** `Session.ZedThreadID`,
  `Session.Metadata.ZedAgentName`, `Session.ParentApp`
  (`api/pkg/types/types.go:433-434`). Agent name resolution lives in
  `getAgentNameForSession()` (`zed_config_handlers.go:~504`).
- **Transcript serialization already exists** in the fork path
  (`session_fork_handlers.go`, `maybePrependTranscript`, `fork_seed`).

## Decision: inject the selected agent on demand (not "all agents")

We inject only the **selected** agent's config when the switch happens, rather
than pre-loading every installation agent into Zed at startup.

Rationale:
- The thread→agent binding is immutable, so "all agents" does **not** avoid the
  new-thread + repopulate work — it only avoids pushing config at switch time.
- "All agents" cannot give each agent its own MCP toolset (context servers are
  per-project/shared), and creates credential/model collisions (e.g. claude
  `managed-settings.json` holds a single model). On-demand injection lets the
  daemon rewrite `agent_servers` **and** `context_servers` to match the agent
  the user actually chose.
- On-demand injection scales to installations with hundreds of agents with no
  change in behaviour.
- The "let users create a new thread in Zed's own UI with any agent" benefit of
  all-agents is a nice-to-have, undercut by the shared-MCP limitation, and can
  be revisited later as an opt-in.

(We note that pre-configuring all *custom* `agent_servers` is technically
feasible CPU-wise; we reject it on the config-collision/MCP grounds above, not
on performance grounds.)

## Flow: in-place switch

```
User picks new agent in dropdown
        │
        ▼
POST /api/v1/sessions/{id}/switch-agent  { helix_app_id }      (NEW endpoint)
        │
   1. Validate: session running (not paused), target app has a
      zed_external assistant, target runtime is Zed-compatible.
   2. Serialize current thread transcript (reuse fork serializer).
   3. Update session: ParentApp = target app, Metadata.ZedAgentName =
      target runtime's ZedAgentName(); clear ZedThreadID / the
      acp_thread_id↔session mapping so the next message opens a NEW thread.
   4. Publish config_changed  ──────────────► settings-sync daemon
        │                                          re-fetches zed-config,
        │                                          rewrites agent_servers +
        │                                          context_servers + creds,
        │                                          writes settings.json
        │                                       (Zed reregisters agents)
   5. Wait for daemon to confirm settings applied (see "Ordering").
        │
   6. Queue a handoff chat_message over external-agent WS:
        { acp_thread_id: null, agent_name: <new>, message: <transcript seed> }
        │
        ▼
Zed creates a NEW thread bound to the new agent in the SAME container,
emits thread_created → Helix maps new acp_thread_id ↔ session,
agent processes the transcript then continues the conversation.
```

The old Zed thread is simply abandoned (left in the container's thread store);
no container restart, no re-clone.

## Ordering / race handling

The new thread must not be created before `settings.json` carries the new
agent, or Zed can't resolve a custom `agent_server` (`qwen`/`goose`). Options,
in order of preference:

1. **Daemon ack.** Have the switch flow wait for a signal that the daemon has
   written the new config. The daemon already exposes `/reload` and a health
   endpoint; add a lightweight "config applied (version N)" signal the API can
   poll/await before sending the handoff message. (Preferred — deterministic.)
2. **`claude`/`zed-agent` fast path.** `claude-acp` is a registry agent and
   `zed-agent` is native, both resolvable immediately — for those targets the
   wait can be skipped.
3. **Retry-on-unresolved.** If Zed reports the agent isn't resolvable yet, it
   retries briefly (bounded). Fallback only; prefer the ack.

Pick #1 as the primary mechanism, with #2 as an optimisation.

## Components to change

### Backend (Go)
- **New endpoint** `POST /api/v1/sessions/{id}/switch-agent`
  (`api/pkg/server/`, sibling to `session_fork_handlers.go`). Reuses the fork
  transcript serializer but does the in-place steps (no new session/container).
- **Session mutation helper**: set `ParentApp`, `Metadata.ZedAgentName`, clear
  `ZedThreadID` and the in-memory acp_thread_id↔session mapping in the
  `ExternalAgentWSManager`.
- **Handoff message**: reuse `maybePrependTranscript` / a `fork_seed`-style
  synthetic interaction so the new thread's first turn carries the transcript.
- **Config-applied signal** in the daemon + an API wait, or the
  `claude`/`zed-agent` fast path.
- Ensure `getZedConfig`/`buildCodeAgentConfig` already key off `ParentApp`
  (they do — `zed_config_handlers.go`), so the daemon re-fetch returns the new
  agent automatically.

### settings-sync daemon (Go)
- Already re-syncs on `config_changed` and detects `code_agent_config` changes
  (`checkHelixUpdates`). Add the "config applied version" signal for the API to
  await. No change to the agent-config generation logic itself.

### Zed (Rust) — minimal / possibly none
- The `chat_message` + `acp_thread_id: null` path already creates a new thread
  on the supplied agent. Verify the existing dispatch
  (`websocket_sync.rs:400`) needs no new command type. If a cleaner explicit
  "switch_agent" command is preferred over reusing `chat_message`, add it as a
  thin wrapper — but the reuse path is the lower-risk default.
- If a custom agent_server isn't yet registered when the thread is created, rely
  on the ordering ack above rather than adding Zed-side retry.

### Frontend (React)
- Rewire `ForkAgentControl` (`frontend/src/components/session/ForkAgentControl.tsx`):
  the dropdown's confirm action calls the new `switch-agent` mutation instead of
  `useForkSession`. Update copy ("Switch agent in this session" instead of
  "fork into a new conversation"). The eligible-agents filter
  (`AGENT_TYPE_ZED_EXTERNAL`) stays.
- Keep the dirty-workspace warning *informational only* — in-place switch
  preserves the workspace, so uncommitted changes are no longer lost. The
  commit/push checkbox and the "child clones fresh" warnings should be removed
  or reworded; there is no fresh clone anymore.
- Add a generated API client method (`./stack update_openapi`).

## Edge cases

- **Switch while the agent is mid-turn**: cancel the current turn
  (`cancel_current_turn`) before resetting the thread binding, to avoid
  orphaned streaming into a thread we're about to abandon.
- **Switch to the same agent**: no-op (the dropdown already guards
  `newAppId === currentAppId`).
- **Paused session**: disallow (same guard as today — switch only on a live
  session).
- **Daemon offline / config never applied**: bounded wait, then surface a clear
  error and leave the session on its current agent (no partial switch).
- **Transcript size**: reuse the fork serializer's existing cap (≈64KB).

## Why this is low-risk

The two hard parts — making an agent resolvable in the container, and creating a
new thread bound to a chosen agent seeded with prior messages — are exactly what
the daemon and the fork path already do. The new work is mostly *wiring*: a new
endpoint that performs the fork's transcript/handoff steps **in place** instead
of against a fresh session, plus an ordering ack and a frontend rewire.
