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

### Spike result (measured — see `spike/RESULTS.md`)

Ran the real Zed binary headless under Xvfb with crafted `settings.json`:

- **100 `agent_servers`: essentially free.** Zero subprocesses spawned, RSS flat
  (328→329 MB), CPU within noise. They spawn lazily on first use.
- **100 MCP `context_servers`: the real cost.** 100 processes at startup, ~3.9 GB
  RSS (≈13× baseline) — and those were do-nothing stubs; real MCPs cost more.

**DECISION (reviewer-confirmed): Strategy B — fully selective. Configure ONLY the
current agent in Zed.**

The reviewer chose *not* to list all agents in Zed, even though the spike showed
it's cheap. Rationale: listing all agents would invite users to switch from
Zed's own UI, where the MCP surface would silently NOT follow them (Zed's
`context_servers` are per-project/shared — only one agent's MCP surface can be
live at a time, and it can't be scoped per-thread). Rather than ship that
footgun, Zed only ever holds the current agent's `agent_servers` + its MCP
`context_servers`.

Consequences:
- **The Helix dropdown is the only switch path.** No Zed-native multi-agent
  picking is exposed/encouraged.
- **On switch, the daemon rewrites Zed's config to the new agent (agent_servers +
  MCP context_servers) and cleanly restarts the Zed process** — a restart is the
  reliable way to swap the instance-wide MCP surface. Then a new thread is
  created on the new agent and repopulated with the prior transcript.
- This is closest to today's behaviour (the daemon already configures exactly one
  agent); net-new work is daemon-owned Zed lifecycle + the switch endpoint +
  new-thread-with-transcript repopulation.
- "Configure all agents" / the hybrid are explicitly **not** built.

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
          Rewrite settings.json for target agent (its agent_servers + its MCP
          context_servers), then daemon stops+restarts Zed cleanly so the new
          MCP surface comes up correctly.
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

## Daemon-driven Zed restart (IMPLEMENTED — simplified from "daemon owns launch")

The meeting framed this as "move Zed start/stop/restart out of the shell scripts
into the daemon." During implementation we found the desktop **already runs Zed
under an auto-respawn loop** — `run_zed_restart_loop` in
`desktop/shared/start-zed-core.sh`: `while true; do /zed-build/zed …; sleep 2; done`.

So we did NOT migrate launch ownership (a risky rewrite of desktop boot).
Instead the daemon **restarts Zed by killing the editor process**
(`pkill -x zed`); the existing loop respawns it ~2s later, and the new process
reads the freshly-written `settings.json`. This achieves the goal — the daemon
deterministically controls *when* Zed restarts — with minimal blast radius.

- `restartZed()` in the daemon (`api/cmd/settings-sync-daemon/main.go`) runs
  `pkill -x zed` (exact name match → only the editor, not the bash loop).
- Hot reload still handles cheap changes (theme — unchanged). A **full restart**
  is used only for agent switches, because Zed's per-project MCP `context_servers`
  can't be hot-swapped per-thread.
- Sequencing on switch: API publishes `config_changed{field:"agent"}` → daemon
  `syncFromHelix()` rewrites `settings.json` for the new agent → daemon
  `restartZed()` → loop respawns Zed with the new agent + MCP surface → Zed
  reconnects the external-agent WS.

(If a future need arises for true daemon-owned launch — e.g. start/stop without
a respawn loop — it can be added later; the restart-loop approach covers the
switch use case fully.)

## Coordination: reuse agent-reconnect instead of a new `agent_config_loaded` event

The earlier draft proposed a new `agent_config_loaded` WS event to gate
new-thread creation. During implementation we found this is **unnecessary** —
the existing reconnect flow already provides the gate:

- The switch handler creates a **Waiting** `fork_handoff` interaction.
- `pickupWaitingInteraction` only delivers Waiting interactions **on agent
  (re)connect** (`websocket_external_agent_sync.go:413`), never to an
  already-connected agent. So the handoff is NOT delivered to the old agent.
- The Zed restart forces a disconnect+reconnect. When the NEW Zed connects, it
  re-fetches `zed-config` (now the new agent), and `pickupWaitingInteraction`
  delivers the handoff to it: `agent_name` comes from `getAgentNameForSession`
  (new `ParentApp`), `ZedThreadID` is empty → new thread, and
  `maybePrependTranscript` injects the transcript.

This means **no new protocol message and no Zed-side change** are required for
the happy path. The custom-agent-not-yet-resolvable race the event was meant to
prevent is handled by ordering: the daemon writes `settings.json` *before*
killing Zed, so by the time Zed respawns and reconnects, the new `agent_server`
is already in its config.

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

## Speed optimization (v2): no-restart fast path + restart fallback

The first working version restarted the Zed *process* on every switch (~5s) and
delivered the handoff on reconnect. Live testing showed the switch took ~15s.
Verified that Zed **hot-reloads** `agent_servers` and `context_servers` from a
`settings.json` change without a process restart (`AgentServerStore` +
`ContextServerStore` both observe `SettingsStore`; `context_server_store.rs:446`
→ `maintain_servers`). So the restart is unnecessary in the common case.

**New flow (fast path):**
1. API `switch-agent`: mutate session, seed fork_seed + Waiting handoff, publish
   `config_changed{field:"agent"}`, start a restart-fallback goroutine.
2. Daemon: rewrite `settings.json` (Zed hot-reloads agent + MCP), then **POST
   `/sessions/{id}/agent-config-applied`** (no `pkill`).
3. API `agent-config-applied`: deliver the handoff to the **live** Zed WS via
   `pickupWaitingInteraction` → new thread on the new agent (transcript
   prepended). No restart, no reconnect.

**Restart fallback:** the API goroutine waits `switchAgentLiveDeliveryTimeout`
(9s); if no new `ZedThreadID` appeared (live path failed — e.g. a brand-new
custom agent_server didn't register from the hot-reload, or the daemon callback
was lost), it publishes `config_changed{field:"agent_restart"}` → daemon
`restartZed()` → the reconnect path delivers the handoff. Keyed on `ZedThreadID`
so it's idempotent (a successful live switch always ends with a new thread id).

**MCP-unchanged fast path:** handled implicitly — Zed's `maintain_servers`
no-ops when the context-server set is unchanged (e.g. opus↔sonnet same MCPs), so
that case is near-instant; differing MCP sets reconcile live and the new thread's
first turn is gated by Zed's own `wait_for_tools_ready`.

**Handoff trimmed:** the handoff prompt now tells the agent NOT to summarise/
re-read the transcript — just emit a one-line ready ack — cutting the model's
output latency (the transcript is still prepended for context).

## Implementation Notes (as built)

Files changed:
- `api/pkg/types/types.go` — added `SessionMetadata.AgentSwitchedAt time.Time`.
- `api/pkg/server/transcript_serializer.go` — `maybePrependTranscript` now also
  fires when `AgentSwitchedAt` is set (not just forks via `ParentSessionID`).
- `api/pkg/server/session_switch_agent_handlers.go` — **new**. `switchAgent`
  HTTP handler + `switchAgentInPlace` + `publishAgentConfigChange`. Reuses
  `resolveForkTarget`, `serializeTranscript`, `agentDescriptor` from the fork path.
- `api/pkg/server/server.go` — registered `POST /sessions/{id}/switch-agent`.
- `api/cmd/settings-sync-daemon/main.go` — `restartZed()` (`pkill -x zed`);
  called from the config-event loop when `field=="agent"`.
- `frontend/src/services/sessionService.ts` — `useSwitchAgent`.
- `frontend/src/components/session/SwitchAgentControl.tsx` — renamed from
  `ForkAgentControl`, simplified (no workspace warnings / commit checkbox).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — uses the new control.
- Regenerated OpenAPI client (`./stack update_openapi`).

Key learnings that simplified the build:
- The Zed thread id is stored on `session.Metadata.ZedThreadID` (DB), not an
  in-memory map — clearing it is all that's needed to make the next message
  open a new thread.
- The desktop already runs Zed under `run_zed_restart_loop`, so the daemon
  restarts Zed by killing it — no launch-ownership migration.
- `pickupWaitingInteraction` only delivers on agent (re)connect, so the Waiting
  handoff naturally waits for the post-restart reconnect and is never delivered
  to the old agent. No new `agent_config_loaded` event needed.
- Zed needs no changes: `chat_message` + `acp_thread_id:null` already creates a
  new thread on the supplied `agent_name`.

Gotcha / not yet validated:
- The daemon change ships in the desktop image — needs `./stack build-ubuntu`
  and a new session to validate end-to-end. The inner-Helix desktop did not
  provision in this dev environment, so the live switch path (restart →
  reconnect → new thread → transcript) is covered by unit tests + code analysis,
  not yet by a full E2E run. Flagged for the reviewer.

## Bug found in live testing: model didn't actually switch (FIXED)

Symptom: after an opus→sonnet switch, Zed's native model dropdown still showed
Opus, and the agent's "I'm Sonnet" was just parroting the handoff text — the
**underlying claude_code inference stayed Opus.**

Root cause: `getZedConfig` resolves `code_agent_config` (which drives the
claude_code model via `/etc/claude-code/managed-settings.json`) from
**`specTask.HelixAppID` first**, then `session.ParentApp`. The switch updated
`session.ParentApp` (→ Zed's native `agent.default_model` became sonnet) but
**not** the spec task's `HelixAppID`, so the daemon's re-fetch still returned
Opus for `code_agent_config.Model` → `managed-settings.json` stayed Opus. (Tell:
the native field was `claude-sonnet-...-latest` while the stale one was the dated
`claude-opus-...-20251101` — two different resolution sources.)

The daemon was innocent — its `syncFromHelix` rewrites `managed-settings.json`
on every `config_changed`; it just wrote what the API returned.

Fix: `switchAgentInPlace` now calls `repointSpecTaskForSwitch` to update the
spec task's `HelixAppID` to the target app (mirroring the fork's
`repointSpecTasksToChild`), before publishing `config_changed`. Regression test
`TestSwitchAgentInPlace_RepointsSpecTaskHelixAppID` asserts on the resolved
config source (the real switch signal), not the agent's self-report.

**Testing lesson:** never trust "ask the model what it is" to confirm a switch —
the handoff text names the model, so it parrots. Confirm via the resolved config
(`managed-settings.json` in-container, or the spec-task/ParentApp resolution),
or the model field in the actual proxied LLM request.

## Why this is achievable incrementally
The two hard parts — making an agent resolvable in the container and creating a
new thread seeded with prior messages — are what the daemon and fork path
already do. New work is the spike, the daemon's lifecycle supervisor, one WS
event, a new endpoint that does the fork's transcript/handoff steps **in place**,
and a frontend rewire. The spike result is the only branch point in the plan.
