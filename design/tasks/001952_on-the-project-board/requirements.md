# Requirements

## Problem

Each task card on the project board shows a small status row: a green dot, the label `In Progress`, and a ticking timer (e.g. `5m32s`). Today this row is shown for **every** task whose `status === "implementation"`, regardless of whether the agent is actually doing anything. The timer keeps counting from `started_at` until a human approves the implementation — even after the agent has long since finished writing code, pushed the branch, and gone idle waiting for review.

This misrepresents reality. A task that has been waiting 30 minutes for human approval looks identical to a task whose agent is actively typing.

## Scope

- **In scope:** the status indicator on a `TaskCard` (the dot + label + timer at `frontend/src/components/tasks/TaskCard.tsx:949-997`).
- **Out of scope:** the column header, the column label, the column the task lives in, the kanban layout, or any backend status transitions. The task stays in the `implementation` column / phase until approval.

## User Stories

**US-1 — Active agent**
As a user watching the board, when an agent is actively working on a task (streaming a response, running tools), I want the card to show a live "working" indicator with a ticking timer, so I can tell something is happening.

**US-2 — Idle agent**
As a user watching the board, when an agent has finished its turn and is idle (e.g. response is complete, no streaming), I want the card to show an "idle" indicator with **no ticking timer**, so I can tell the agent is not currently doing anything.

**US-3 — Sandbox down**
As a user watching the board, when the sandbox container for a task is absent / stopped, I want the card to clearly show the agent isn't running at all (not just idle), so I know I need to start it before anything can progress.

## Acceptance Criteria

1. Backend exposes a real, derived `agent_work_state` on every `SpecTask` returned by `GET /api/v1/spec-tasks` (and the project listing endpoint that powers the kanban). Today the field exists in the type but is never populated.
2. `agent_work_state` is one of `working`, `idle`, or `done`. Derivation rules:
   - `working` — sandbox is `running` AND the latest interaction on the planning session is in `Waiting` state (streaming / not yet complete).
   - `idle` — sandbox is `running` AND the latest interaction is `Complete` (or there is no in-flight interaction).
   - `done` — task has reached a post-implementation status (`implementation_review`, `pull_request`, `done`). Frontend already changes the dot for these phases so this is mostly for completeness.
   - When `SandboxState` is `absent` or `starting`, `agent_work_state` is left empty so the card can show a sandbox-state hint instead.
3. `TaskCard` only shows the **ticking** timer when `agent_work_state === "working"`. When `idle`, the timer disappears and the label changes from `In Progress` to `Idle` (still green dot, still in the In Progress column).
4. When the sandbox is absent / starting, the row shows the existing sandbox status hint (e.g. `Sandbox stopped`) instead of `In Progress` / `Idle`.
5. The dot color remains green for the `implementation` phase (no column or phase color changes).
6. State updates within ~30s of the agent transitioning between working and idle (matches the existing kanban polling cadence). No new websocket plumbing is required for v1.
7. No regressions for other phases (`planning`, `review`, `pull_request`, `completed`) — their existing dot/label rendering is untouched.

## Non-Goals

- Don't ship the broader "External Agent State Reconciliation" system from `helix/design/2025-12-22-external-agent-state-reconciliation.md`. That doc proposes a new `external_agent_activity` table and a reconciler loop; it was never implemented and is significantly larger than this fix. We do **not** need a new table — we can derive `agent_work_state` from existing data (`Session.Updated`, latest `Interaction.State`, `SandboxState`).
- Don't touch the `started_at` timestamp or how it is set.
- Don't change column WIP limits or movement rules.
