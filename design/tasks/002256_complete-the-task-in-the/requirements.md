# Requirements: Surface WIP-Queue Block Reason and Fix the WIP Gate Formula

## Background

A spec task can sit in `queued_spec_generation` (or `queued_implementation`)
indefinitely with no sandbox, session, or agent — because the orchestrator is
deliberately holding it behind a WIP (work-in-progress) limit. The only record
of *why* is a server log line the user never sees, so the task detail page and
kanban card look dead/stuck ("taking forever to boot").

Two problems must be fixed together:

1. **The block reason is invisible** to the user (primary UX fix).
2. **The WIP gate formula is wrong** — `planningCount + reviewCount >= reviewLimit`
   compares the sum of two columns against the review limit alone, which makes
   the planning limit dead and lets a full Review column permanently starve
   planning.

## User Stories

### US-1 — See why a queued task hasn't started (detail page)
As a user viewing a queued spec task's detail page, I want a clear, specific
banner explaining why it hasn't started and what to do, so I don't think the app
is broken.

**Acceptance criteria**
- When a task is in `queued_spec_generation` or `queued_implementation` and the
  backend has computed a non-empty `queue_reason`, the detail page shows an
  info banner with that reason (e.g. MUI `<Alert severity="info">`).
- The reason text is specific and actionable, e.g. "Waiting for review capacity —
  8 specs are awaiting review (limit 2). Approve or clear reviews in the Review
  column to start planning."
- When `queue_reason` is empty (task can start / is starting), no block banner is
  shown; existing "Starting Desktop" behaviour is preserved.

### US-2 — See the reason on the kanban card
As a user scanning the board, I want a queued card to show (inline or as a
tooltip) why it's blocked, so a queued card doesn't look dead.

**Acceptance criteria**
- A card in a queued state with a `queue_reason` shows the reason compactly
  (inline text or tooltip) on the kanban board.
- Non-queued cards and queued cards with no reason are unchanged.

### US-3 — Reason is computed live and clears as the queue drains
As a user, I want the reason to update/clear automatically as capacity frees up,
so I can trust it reflects the current state.

**Acceptance criteria**
- `queue_reason` is a transient, non-persisted field recomputed on every read
  (not stored in the DB).
- It is populated in both `listTasks` (kanban) and `getTask` (detail page).
- When the blocking condition clears (e.g. Review column drained below limit), the
  next read returns an empty `queue_reason` and the task starts within ~10s.

### US-4 — WIP gate limits are each meaningful (Problem 2)
As a project owner, I want the planning and review WIP limits to each have an
independent, correct effect.

**Acceptance criteria**
- New planning does not start when planning is full: `planningCount >= planningLimit`.
- New planning does not start when review is already at/over its own limit:
  `reviewCount >= reviewLimit` (NOT `planningCount + reviewCount >= reviewLimit`).
- With defaults `planningLimit=3`, `reviewLimit=2`, up to 3 tasks can generate
  specs concurrently (planning limit is no longer dead).
- The dependency-not-ready block (`areBacklogDependenciesReady`) is still honoured
  and produces its own reason string.
- The same corrected formula is applied consistently everywhere the gate appears
  (`handleQueuedSpecGeneration` and the backlog→queued gate in `handleBacklog`).

### US-5 — Single source of truth + tests
As a maintainer, I want one reusable pure function computing the block reason so
the orchestrator and the read handlers can't diverge, covered by unit tests.

**Acceptance criteria**
- A pure function (e.g. `planningQueueReason(project, projectTasks, task) string`)
  returns "" when the task could start now, else a human-readable reason.
- `handleQueuedSpecGeneration` uses it; behaviour stays identical (non-empty →
  leave queued and log as today).
- Table-driven Go unit tests cover: planning-full, review-full,
  dependency-blocked, not-blocked (and the analogous implementation case).

## Non-Goals
- Redesigning the board or WIP-limit settings UI.
- Adding new WIP limit config fields.
- Changing where the automatic `spec_generation → spec_review` transition happens
  (the deeper "gate entry to Review" redesign) — out of scope; we fix the
  formula and make the stall visible instead.

## Open Questions
- **Reason copy wording** — proposed strings are in the design; confirm exact
  phrasing is acceptable (numbers are derived, not hardcoded).
- **Problem 2 semantics**: the brief leans toward gating planning on
  `reviewCount >= reviewLimit` (drop the prospective +planningCount reservation).
  This is the chosen approach. Confirm you're happy dropping the reservation
  rather than, say, gating on `reviewCount + planningCount >= reviewLimit +
  planningLimit`. Chosen approach makes each limit independent and never starves
  planning silently.
- **`queued_implementation` reason**: the implementation queue handler currently
  has no WIP gate of its own (it launches immediately once dependencies are
  ready); the only block cause there is dependency-not-ready. We will surface the
  dependency reason for it. Confirm there's no separate implementation WIP gate
  you expect surfaced (backlog→queued_implementation is gated in `handleBacklog`).
