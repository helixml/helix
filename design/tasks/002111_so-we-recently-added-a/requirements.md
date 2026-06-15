# Requirements: In-Place Agent Switching via New Zed Threads

## Background

We recently shipped mid-session agent switching as **fork-and-pause**
(`POST /sessions/{id}/fork`, driven by the `ForkAgentControl` dropdown). When a
user picks a different agent, Helix forks the session into a brand-new
session + brand-new desktop container, pauses the parent as a frozen
checkpoint, and seeds the child with the parent's transcript.

The fork gives the new agent a clean slate, but at a high cost: the entire
container is restarted and the workspace is re-cloned. Anything that happened
in the environment up to that point (uncommitted edits, running processes,
installed tools, scratch files) is lost or has to be laboriously duplicated.

Per the team meeting, we are adding a **second, default path**: keep the *same*
environment and just switch the agent that Zed ("Z") is using inside it, by
**creating a new Zed thread** bound to the selected agent. The session's
container, workspace, and history all stay put.

## Decisions from the meeting (authoritative)

These supersede earlier assumptions:

1. **Switch = new Zed thread, not a new container.** Selecting an agent creates
   a new Z thread with that agent; the existing container/session is reused.
2. **Threads are isolated per agent.** Sessions are **not** migrated between
   agents (the agent runtime owns session persistence). Each switch creates a
   new thread; existing threads remain intact but bound to their own agent.
   Helix copies/restreams the right context into the new thread transparently.
3. **Keep "fork" as a separate, clearly-distinct feature.** The container-clone
   path stays in the codebase. Only the dropdown is rewired to the switch path.
4. **No "fork" jargon in the UI.** Present this as a simple agent toggle.
5. **The Settings-Sync daemon owns the Zed process lifecycle.** Starting,
   stopping, and restarting Zed moves out of shell scripts into the Go daemon,
   so config rewrites + clean restarts during a switch are reliable.
6. **Prefer configuring Zed with all agents up front** (so switching needs no
   runtime reconfiguration) — *gated on a performance spike*. If startup
   degrades, fall back to partial/lazy/selective syncing.
7. **Extend the WebSocket sync protocol with an event** that signals when an
   agent's configuration has loaded, so the switch is event-driven (no polling,
   fewer races).

## Goal

Decouple the agent dropdown from the fork behaviour. Switching an agent on a
running session should create a new Zed thread with the selected agent inside
the existing environment, repopulate it with the prior conversation's context,
and feel like a simple toggle — no container teardown, no new Helix session.

## User Stories

### US-1: Toggle agent without losing the environment
As a user mid-session, when I pick a different agent from the dropdown, the
agent changes inside my current environment so my in-progress work (files,
processes, git state) is preserved.

**Acceptance criteria:**
- Switching does **not** fork, create a new session id, or restart/replace the
  desktop container; the session keeps the same id, URL, and container.
- Uncommitted edits and workspace state survive the switch (no re-clone).
- The UI uses toggle/switch language — never "fork".

### US-2: New thread per agent, with prior context
As a user, switching creates a new Zed thread for the selected agent that has
the previous conversation as context.

**Acceptance criteria:**
- A new Zed thread is created bound to the selected agent; the previous thread
  is left intact (not migrated).
- The new thread's first turn carries the prior transcript (reusing the fork
  transcript serializer).
- The Helix chat panel shows prior interactions followed by the new agent's
  turns — no duplicates, no lost messages.
- The session's stored agent (`ParentApp` / `Metadata.ZedAgentName`) and
  `ZedThreadID` reflect the new agent and new thread.

### US-3: Daemon-managed Zed lifecycle + config
As the system, the Settings-Sync daemon makes the target agent available and
controls Zed's process lifecycle so the switch is clean and reliable.

**Acceptance criteria:**
- Zed start/stop/restart is driven by the daemon (Go), not the desktop shell
  scripts.
- The daemon rewrites Zed's config for the switch and, where a clean restart is
  needed, stops and restarts Zed deterministically.
- A new WS-sync event signals when the agent config has loaded; the switch
  proceeds on that event rather than a timer.

### US-4: Switching is fast and scales with agent count
As a user in an installation with many agents, switching stays responsive.

**Acceptance criteria:**
- A spike measures Zed startup/resource impact with ~100 agents configured
  (including their MCP servers). Findings recorded.
- If all-agents config is acceptable, switching requires no per-switch
  reconfiguration. If not, the partial/lazy/selective fallback is implemented so
  startup stays acceptable.

### US-5: Fork path preserved
As a developer, the container-clone fork code remains intact for callers that
want a clean slate.

**Acceptance criteria:**
- `POST /sessions/{id}/fork` and all `fork_*` handlers/markers remain and keep
  working.
- The dropdown no longer calls the fork endpoint by default.

## Research findings feeding the spike (see design.md for detail)

- **`agent_servers` entries are CPU-cheap at startup** — Zed validates/indexes
  them but spawns the agent subprocess **lazily** on first thread use
  (`AgentConnectionCache`). So the agent list itself is not the startup cost.
- **The real startup cost is MCP `context_servers`.** They are **per-project /
  shared across agents** in Zed, initialized up front, and several spawn `npx`
  processes. Configuring all agents *and unioning their MCP servers* is what the
  spike must measure. This per-project sharing also means "all agents" cannot
  give each agent its own isolated toolset.
- **Thread→agent binding is immutable** — confirming the new-thread approach is
  the only viable one (sessions can't be migrated between agents).
- **`chat_message` with `acp_thread_id: null` already creates a new thread**
  bound to the supplied `agent_name`, so the switch reuses existing machinery.

## Out of Scope

- Migrating a single Zed thread to a different agent (not supported by Zed).
- Changing how the fork path itself works.
- Per-agent isolated MCP toolsets while multiple agents are simultaneously
  configured (blocked by Zed's per-project context servers; revisit later).
