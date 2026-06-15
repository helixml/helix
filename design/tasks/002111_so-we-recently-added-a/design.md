# Design: In-Place Agent Switching via New Zed Threads

## Summary

Add an in-place "switch agent" path that changes the agentic framework inside a
running session's existing Zed environment by **creating a new Zed thread** bound
to the selected agent — instead of forking to a new container/session. The
switch reuses machinery that already exists and adds three pieces the meeting
called for: **daemon-owned Zed lifecycle**, an **agent-config-loaded WS event**,
and a **performance spike** that decides all-agents vs. selective config.

The dropdown is rewired from the fork endpoint to a new switch endpoint, with
toggle/"switch" language (no "fork" jargon). The fork code stays intact.

## Two channels, and which one does what (mental model)

There are **two separate channels** to the container; the switch needs both:

| Channel | File | Role |
|---|---|---|
| **Settings-Sync daemon** (`config_changed` over `/api/v1/ws/user`) | `api/cmd/settings-sync-daemon/main.go` | Rewrites `settings.json` (`agent_servers`, `context_servers`, model, credentials) **and now owns the Zed process lifecycle** (start/stop/restart). |
| **External-agent sync WS** | `api/pkg/server/websocket_external_agent_sync.go` ↔ `zed/crates/external_websocket_sync/` | Drives Zed threads; sends `chat_message`/`open_thread` with `agent_name`; will carry the new **agent-config-loaded** event. |

Writing `settings.json` alone never switches a running thread — a Zed thread is
bound to one agent at creation. So the switch = (daemon makes the target agent
available, restarting Zed if needed) **then** (external-agent WS creates a fresh
thread on the new agent + repopulates context).

## Key facts established by research

- **Thread→agent binding is immutable** (`AcpThread.connection` fixed at
  creation; `zed/crates/acp_thread/`, `thread_service.rs`). Switching always
  means a *new* thread; sessions can't be migrated between agents.
- **`agent_name` is sent per message.** Helix puts it in the command data
  (`websocket_external_agent_sync.go`); Zed maps it (`thread_service.rs:~1405`):
  `claude`→`claude-acp`, `zed-agent`/none→native, others (`qwen`,`goose`) as-is.
- **`acp_thread_id: null` ⇒ new thread** (`websocket_sync.rs:401`
  `handle_chat_message`). On creation Zed emits `thread_created`, which Helix
  maps to the session (`handleThreadCreated`).
- **`agent_servers` are cheap at startup; subprocesses spawn lazily**
  (`agent_servers/src/connection_cache.rs`).
- **MCP `context_servers` are per-project/shared and initialized up front**
  (`project/src/context_server_store.rs`); several spawn `npx`. This is the real
  startup cost the spike targets, and the reason per-agent MCP isolation isn't
  possible.
- **Session already stores `ZedThreadID`, `Metadata.ZedAgentName`, `ParentApp`**
  (`api/pkg/types/types.go:433-434`); agent resolution in
  `getAgentNameForSession()` / `buildCodeAgentConfig()` (`zed_config_handlers.go`).
- **Zed is currently launched by shell scripts** (`desktop/shared/start-zed-core.sh`
  → `start_zed_helix`); the daemon only writes config today. Moving lifecycle
  into the daemon is net-new.
- **Transcript serialization already exists** in the fork path
  (`session_fork_handlers.go`, `maybePrependTranscript`, `fork_seed`).

## Spike (do this first — gates the config strategy)

**Question:** does configuring Zed with ~100 agents (and the union of their MCP
`context_servers`) degrade startup time or resource use?

- Generate a settings.json with 100 `agent_servers` entries + representative MCP
  `context_servers`; measure cold Zed startup, process count (`npx` spawns),
  memory, and time-to-first-thread.
- **If acceptable** → **Strategy A: configure all agents up front.** Switching
  then needs *no* runtime reconfiguration or restart — just create a new thread
  with the already-configured agent (matches "Zed supports multiple agents
  simultaneously"). Note the constraint: a single shared `context_servers` set,
  so MCP tools are the union, not per-agent.
- **If too slow** → **Strategy B: selective/lazy.** The daemon writes only the
  selected agent's `agent_servers` + `context_servers` on switch, then performs
  a clean Zed restart (daemon-owned) before the new thread is created. Scales to
  any agent count; MCP tools follow the active agent.

The rest of the design works for **either** strategy; only the "make target
agent available" step differs.

## Flow: in-place switch

```
User picks new agent in dropdown (toggle UI)
        │
        ▼
POST /api/v1/sessions/{id}/switch-agent  { helix_app_id }      (NEW endpoint)
        │
   1. Validate: session running (not paused); target app has a zed_external
      assistant; target runtime is Zed-compatible
      (zed_agent/claude_code/qwen_code/goose_code).
   2. Cancel any in-flight turn on the current thread (cancel_current_turn).
   3. Serialize the current thread's transcript (reuse fork serializer).
   4. Update session: ParentApp = target app, Metadata.ZedAgentName = target
      runtime's ZedAgentName(); clear ZedThreadID + the acp_thread_id↔session
      mapping so the next message opens a NEW thread.
   5. Publish config_changed ───────────► Settings-Sync daemon
          Strategy A: target already configured → no-op / quick verify.
          Strategy B: rewrite settings.json for target agent, then daemon
                      stops+restarts Zed cleanly.
   6. Daemon emits "agent_config_loaded" over the WS sync protocol when the
      target agent is resolvable (and Zed is back up, in Strategy B).
        │
   7. On that event, Helix queues a handoff chat_message over external-agent WS:
          { acp_thread_id: null, agent_name: <new>, message: <transcript seed> }
        │
        ▼
Zed creates a NEW thread bound to the new agent in the SAME container,
emits thread_created → Helix maps new acp_thread_id ↔ session + persists
ZedThreadID; the agent processes the transcript, then continues.
```

The previous Zed thread is left intact in the container's thread store
("isolated per agent"); no container restart, no re-clone.

## Daemon-owned Zed lifecycle

Move Zed start/stop/restart out of `start-zed-core.sh` into the Settings-Sync
daemon (Go), so the daemon can deterministically restart Zed after a config
rewrite. Concretely:

- The daemon gains a supervisor: launches the Zed process, tracks its PID,
  exposes start/stop/restart, and restarts on demand for a switch (Strategy B)
  or on crash.
- Hot reload stays for cheap changes (theme — already works). A **full clean
  restart** is used for agent switches in Strategy B because it avoids the
  edge cases of live agent reconfiguration.
- The desktop shell scripts shrink to "start the daemon"; the daemon owns Zed.
- Sequencing on switch: rewrite config → restart Zed → wait for Zed to
  reconnect the WS → emit `agent_config_loaded` → Helix sends the handoff.

## New WS-sync event: `agent_config_loaded`

Extend the external-agent sync protocol with an event (Zed→Helix or
daemon→Helix) signalling the target agent's config is loaded and resolvable.
Helix awaits this event before creating the new thread, so a custom
`agent_server` (`qwen`/`goose`) is never referenced before it exists. For
registry/native agents (`claude`/`zed-agent`) the agent is resolvable
immediately and the event can fire fast (or be short-circuited).

This replaces the polling/ack idea from the earlier draft with the event-driven
mechanism the team specified.

## Components to change

### Backend (Go)
- **New endpoint** `POST /api/v1/sessions/{id}/switch-agent` (new file beside
  `session_fork_handlers.go`), swagger-annotated. Reuses the fork transcript
  serializer but performs the in-place steps (no new session/container).
- **Session mutation helper**: set `ParentApp`, `Metadata.ZedAgentName`; clear
  `ZedThreadID` and the acp_thread_id↔session mapping in
  `ExternalAgentWSManager`.
- **Handoff seed**: reuse `maybePrependTranscript` / a `fork_seed`-style
  synthetic interaction so the new thread's first turn carries the transcript.
- **Wait on `agent_config_loaded`** before queueing the handoff message.
- Confirm `getZedConfig`/`buildCodeAgentConfig` resolve the new agent from
  `ParentApp` after the switch (expected: yes — verify).

### Settings-Sync daemon (Go)
- Add the **Zed process supervisor** (start/stop/restart, crash restart).
- On `config_changed` for a switch: Strategy A verify / Strategy B rewrite +
  restart.
- Emit the **`agent_config_loaded`** event when the agent is resolvable.

### Zed (Rust)
- Verify `chat_message` + `acp_thread_id: null` creates a new thread on the
  supplied agent (dispatch `websocket_sync.rs:400`) — likely no new command
  type. Add the `agent_config_loaded` emission if it's sourced from Zed rather
  than the daemon.
- Bump `ZED_COMMIT` in `sandbox-versions.txt` if Zed is touched (per repo rule).

### Frontend (React)
- Rewire `ForkAgentControl` (`frontend/src/components/session/ForkAgentControl.tsx`):
  dropdown confirm calls the new `switch-agent` mutation, not `useForkSession`.
- **Switch/toggle copy** — remove all "fork" language; remove the "child clones
  fresh" + commit/push-before-fork warnings (workspace is preserved). Keep the
  `AGENT_TYPE_ZED_EXTERNAL` filter and paused-session guard.
- Add the generated API client method (`./stack update_openapi`).

## Edge cases
- **Switch mid-turn**: cancel current turn before resetting the thread binding.
- **Switch to same agent**: no-op (dropdown already guards equality).
- **Paused session**: disallow (switch only on a live session).
- **Strategy B restart fails / daemon offline**: bounded wait, surface a clear
  error, leave the session on its current agent (no partial switch).
- **Transcript size**: reuse the fork serializer's existing cap (≈64KB).
- **Switch from Zed's native UI**: treat as a new thread too, to keep UX
  consistent (per meeting); Helix maps whatever `thread_created` reports.

## Why this is achievable incrementally
The two hard parts — making an agent resolvable in the container and creating a
new thread seeded with prior messages — are what the daemon and fork path
already do. New work is the spike, the daemon's lifecycle supervisor, one WS
event, a new endpoint that does the fork's transcript/handoff steps **in place**,
and a frontend rewire. The spike result is the only branch point in the plan.
