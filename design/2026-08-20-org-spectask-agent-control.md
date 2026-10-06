# Org spec-task agent control

## Problem

Org-chart Bots can create and review spec tasks, but cannot send follow-up
messages to the coding agents inside those tasks. Their lifecycle surface also
only exposes initial workflow start and desktop stop; it cannot explicitly
resume or restart an existing task desktop.

## Design

Extend the existing `runtime.SpecTasks` port and MCP adapters with four new
operations:

- `send_spectask_agent_message` queues a durable follow-up on the task's
  canonical planning session. Normal messages wait for the current turn;
  `interrupt=true` cancels the current turn before delivery.
- `list_spectask_agent_messages` reads the latest user/agent turns from that
  session in chronological order. Its projection excludes system prompts,
  tool payloads, and usage data.
- `start_spectask_agent` resumes an existing stopped planning session while
  preserving its conversation and workspace.
- `restart_spectask_agent` uses the canonical session-container restart path,
  preserving healthy conversation state, resetting positively wedged threads,
  and retrying crash-marked prompts.

`stop_spectask_agent` remains the stop operation. `start_spectask_planning`
remains distinct: it starts a backlog task's workflow and is not a desktop
resume API.

The Helix runtime continues to enforce Bot membership, explicit project access,
same-organization project ownership, and task-to-project ownership before any
operation reaches the server workflow adapter. The adapter delegates to the
same queue, resume, stop, and restart primitives used by the REST/UI surfaces.

The project-manager Bot prompt grants all four message/lifecycle tools so newly
created PM Bots can use the registered MCP APIs.

## Verification

- Runtime noop, application-service identity forwarding, MCP argument/response,
  registration, project scoping, message delivery, and lifecycle delegation
  tests.
- Existing session restart tests continue to own the detailed stop/recreate,
  thread-preservation, and subsequent-message behavior of the shared primitive.
- Go package build and focused test suites.
