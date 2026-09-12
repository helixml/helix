# Subagent activity in task chat

## Goal

Show coding-agent delegation as first-class activity in the task chat and in an
Agents view beside the desktop. The view must remain available after a turn or
session finishes.

## Reference behavior

T3 Code normalizes provider-specific task events, folds them into a shared
subagent runtime, and renders that runtime in both the message timeline and an
Agents panel. Helix follows the same split between event interpretation and UI
surfaces, but uses persisted `Interaction.response_entries` as its source of
truth.

## Data flow

Zed's ACP thread already retains the authoritative tool-call ID, provider tool
name, status, and child session ID. External WebSocket sync now sends those
fields with every tool-call update:

- `tool_call_id`
- `tool_call_name`
- `subagent_id`

The API accumulator preserves the metadata across streaming overwrites and
stores it in `response_entries`. Entry patches carry the same fields to the
live client. Existing stored sessions remain compatible: the frontend also
recognizes the historical Codex display labels (`Start subagent ...`,
`Interact with subagent ...`, and related variants).

The frontend folds stored interactions plus the current streamed interaction
into stable runs. Spawn entries render inline; all subagent actions render in
the task's Agents view. Zed mirrors each child session's tool calls into the
parent interaction with namespaced message IDs, so the panel shows the child's
live commands and tool results without duplicating its assistant prose into the
parent answer. A running parent interaction keeps a completed spawn tool
visually active until the turn reaches a terminal state.

## Verification

- Zed protocol serialization and compile tests
- API accumulator and WebSocket handler tests
- frontend parser, timeline, toolbar, type-check, and production build
- live task with a real subagent while running, after completion, and after a
  full page reload
