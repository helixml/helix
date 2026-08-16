# Execution controls + interrupt for org-chart chat sessions

Date: 2026-08-13

> Superseded for SpecTasks by
> `2026-08-15-spec-task-code-agent-config-migration.md`: SpecTasks now own a
> complete `CodeAgentExecutionConfig`. Session-owned overrides remain valid for
> general chat and org-agent sessions.

> Superseded for new general-session edits by
> `2026-08-16-code-agent-config-picker.md`: general sessions now persist a
> complete config while retaining their parent Agent identity. Overrides are
> read only for historical sessions. The implementation and verification below
> describe the superseded first iteration.

## Problem

`/orgs/{org}/chat/session/{id}` (the org-chart bot chat, `pages/Session.tsx`
with `orgChatView`) runs the same external coding agent as a spec task, but its
composer had neither an interrupt button nor any way to change the model,
provider, or reasoning effort. The only escape was the agent's settings page,
which does not restart the running ACP thread — so a model change silently did
nothing until the next thread.

Spec tasks already have all of this in `SpecTaskExecutionControls`, mounted as
the composer's `leadingActions`. The requirement was parity, reusing the same
composer rather than building a second one.

## Why the spec-task endpoint could not just be reused

`PATCH /spec-tasks/{id}/execution-config` stores the overrides on the SpecTask
row. An org bot session has no SpecTask, so there was nowhere to put them.

Everything *downstream* of storage was already session-scoped:
`cancelActiveTurn`, `switchAgentInPlaceForNextTurn`, and the `getZedConfig` read
that materialises the model into the sandbox all key on a session. Only the
storage location differed between the two surfaces.

## Design

**One mechanism, two storage locations.** `api/pkg/server/execution_config.go`
holds the shared logic; each surface supplies a `codeAgentConfigTarget`
describing its current identity and how to persist a new one:

- SpecTask surface → writes the task-owned execution configuration.
- Session surface → writes `Session.Metadata.CodeAgentOverrides` (new).

`applyCodeAgentExecutionConfig` then does the same thing for both: validate,
no-op check, drain in-flight turns, persist, resolve the runtime, switch the
agent in place, and roll the persist back if the switch fails.

**One source of truth per session.** A session that belongs to a SpecTask
delegates to the task — `sessionExecutionConfigSurface` resolves the owner, and
the session endpoint writes through to the task row. This matters because
`getZedConfig` reads the task first; a session-level override on a task session
would be silently ignored. Task-driven sessions therefore never carry their own
overrides, and the session endpoint is safe to call from any chat surface
(spec-task execution sessions are reachable from this same org-chat URL via the
executions history).

**Kind restriction moved to the caller.** Spec tasks may only run
`coding_agent` apps. An org chart's bots run on `org_agent` apps, which have a
`zed_external` assistant but would fail that check. `requiredAgentKind` is now
per-surface, and it is only enforced when the Agent actually changes — resending
the current agent id alongside a model edit is not an agent switch.

**`SpecTaskExecutionConfig` → `AgentExecutionConfig`.** The DTO describes a
coding identity, not a task; both surfaces return it. It also now echoes the
override set it was resolved from, so a composer can round-trip an edit without
knowing which record stores it.

## API

| | |
|---|---|
| `GET /api/v1/sessions/{id}/execution-config` | → `types.AgentExecutionConfig` |
| `PATCH /api/v1/sessions/{id}/execution-config` | `{agent_id?, code_agent_overrides}` → `types.SessionExecutionConfigUpdateResponse` |

Rejected: non-`zed_external` sessions (400), paused sessions (409 — reconfigure
the active descendant, same rule as switch-agent). Embed keys cannot reach
either route: the allowlist's `/api/v1/sessions/` rule matches a single trailing
segment only, which is the documented intent (an embed user must not reconfigure
the agent they are talking to).

## Frontend

`Session.tsx` mounts the existing `SpecTaskExecutionControls` as the composer's
`leadingActions` for external-agent sessions, and wires `onCancel` to
`v1SessionsCancelCreate` — the same props `AgentChat` passes on the spec-task
page. No new composer.

Two adjustments to the shared control:

- `onSandboxResourceOverridesChange` is now optional; the compute control is
  hidden when a surface has no resizable sandbox of its own. Session-level
  sandbox sizing is **not** part of this change.
- The picker's agent list must include the session's own Agent even when it is
  not a coding agent, otherwise an org bot's own model and reasoning are not
  editable (`selectCodingAgents` deliberately excludes `org_agent`).

## Verification

Backend: 7 new tests in `session_execution_config_handlers_test.go` (in-process
`NewTestServer` + memorystore), plus the existing spec-task and switch-agent
suites, which cover the refactor.

Live, against the running dev stack on the org bot session in the bug report
(`ses_01kzgrzwybcw3qcg8bj9jqbsyv`, "Software Engineer" @ unmanned-org):

- `GET` returned the bot's identity (zed_agent / ds4-flash-node06 /
  deepseek-v4-flash / medium).
- Changing reasoning Medium → High **from the composer control**: persisted
  `{"reasoning_effort":"high"}` on the session row, cleared the Zed thread and
  opened a new one, seeded `fork_seed` + `fork_handoff`, and the live agent
  completed the handoff turn. `GET /zed-config` then reported
  `reasoning_effort: high`.
- Resetting to Default cleared the override and `/zed-config` returned to
  `medium`. Both switches produced a fresh thread.
- The model picker lists the bot's agent with its full provider/model set.

Not observed live: the Stop button rendered mid-turn on this page. The browser
session was signed in as an admin who is not the session owner, so
`checkOwnership` blocks sending from it. The composer's props were verified on
the live page instead (`onCancel` present, `isCancelling`, `sendMode: direct`),
and the render-and-click contract is covered by the existing
`RobustPromptInput` tests.
