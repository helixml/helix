# Requirements: In-Place Agent Framework Switching on Running Sessions

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

We want a **second, default path**: keep the *same* environment and just switch
the agent that Zed ("Z") is using inside it. The session's container, workspace,
and history all stay put; only the agentic framework changes.

## Goal

Decouple the agent dropdown from the fork behaviour. When a user switches the
agent on a running session, Helix should switch the agent **in place** inside
the existing Zed environment — without restarting the container or creating a
new Helix session — while preserving the conversation by repopulating a fresh
Zed thread with the prior thread's messages.

The fork path and its handlers remain in the codebase, available for callers
that still want a clean-slate fork. Only the dropdown's wiring changes.

## User Stories

### US-1: Switch framework without losing the environment
As a user mid-session, when I pick a different agent from the dropdown, I want
the agent to change inside my current environment so that all my in-progress
work (files, processes, git state) is preserved.

**Acceptance criteria:**
- Picking a new agent does **not** fork the session, create a new session id,
  or restart/replace the desktop container.
- The session keeps the same id, URL, and desktop container.
- Uncommitted edits and workspace state survive the switch (no re-clone).
- The conversation continues in the same session view.

### US-2: Conversation continuity across the switch
As a user, I want the new agent to see the full conversation that happened
before the switch, so it can continue the work with context.

**Acceptance criteria:**
- After the switch, the new agent's first turn has the prior thread's
  transcript as context (reusing the existing fork transcript serializer).
- The Helix chat panel continues to show the prior interactions followed by the
  new agent's turns (no duplicate or lost messages).
- The session's stored agent (`ParentApp` / `Metadata.ZedAgentName`) and
  `ZedThreadID` reflect the new agent and the new Zed thread.

### US-3: Make the new agent available in the Zed environment
As the system, I must ensure the target agent's configuration (runtime,
model, provider, base URL, credentials, MCP tools) is present in the Zed
container before the new thread is created, so the new thread can bind to it.

**Acceptance criteria:**
- The settings-sync daemon writes the target agent's `agent_servers` /
  managed-settings into the container before Helix asks Zed to create the new
  thread (ordering is coordinated; no race where Zed can't resolve the agent).
- Switching is supported between the external-agent runtimes that make sense to
  run in Zed (`zed_agent`, `claude_code`, `qwen_code`, `goose_code`). Agents
  that don't map to a Zed runtime are excluded from the dropdown (current
  `zed_external` filter is retained).

### US-4: Fork path preserved
As a developer, I want the fork-and-pause code to remain intact so that a
clean-slate fork is still possible for callers that want it.

**Acceptance criteria:**
- `POST /sessions/{id}/fork` and all `fork_*` handlers/markers remain and keep
  working.
- The dropdown no longer calls the fork endpoint by default; it calls the new
  in-place switch path.

## Research Questions (answered — see design.md for detail)

1. **Is it efficient to pre-configure *all* installation/project agents in Zed
   at startup?**
   Configuring many entries in `agent_servers` is **CPU-cheap**: Zed validates
   and indexes them at startup but only spawns an agent subprocess **lazily**,
   on the first thread that targets it (`AgentConnectionCache`). So a long list
   does not cost startup CPU. **However**, "all agents" has real drawbacks:
   per-agent credentials/model can collide (e.g. claude managed-settings holds
   one model), and **MCP `context_servers` are shared per-project in Zed, not
   per-agent** — so it cannot cleanly give each agent its own toolset.

2. **All-agents vs. inject-the-selected-agent?**
   Because the thread→agent binding is immutable, *both* approaches still
   require creating a new thread and repopulating it. The all-agents approach
   only removes the "push new config before switch" step — at the cost of the
   MCP/credential collisions above. We therefore recommend **on-demand
   injection of the selected agent** (the daemon already detects
   `code_agent_config` changes), which scales to any agent count and lets MCP
   tools follow the active agent. See design.md §"Decision".

3. **How do we switch the Zed thread to a new framework while keeping prior
   messages?**
   Reuse the existing mechanism: `chat_message` with `acp_thread_id: null`
   already creates a *new* Zed thread bound to the supplied `agent_name`. The
   in-place switch is effectively "fork minus the new container/session":
   reset the session's thread binding, set the new agent, and send a handoff
   message seeded with the serialized prior transcript.

## Out of Scope

- Migrating a single Zed thread's internal state to a different agent (Zed
  binds a thread to one agent immutably — not supported, not attempted).
- Changing how the fork path itself works.
- Letting an agent have a *different* MCP toolset while other agents are
  simultaneously configured (blocked by Zed's per-project context servers).
