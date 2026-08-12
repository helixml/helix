# Requirements: Live Verification of Agent Questions (ACP Elicitations) in the Helix UI

## Background

Agent questions via ACP elicitations are **already built** on branch
`feature/002731-end-to-end-agent` (helix PR #3009, zed PR #83). Claude Code's built-in
`AskUserQuestion` tool becomes an ACP form elicitation that blocks the turn;
`claude-agent-acp` emits `session/request_elicitation`; the branch mirrors it into Helix as
`elicitation_requested`, renders a card in the conversation, and sends the answer back with
`respond_elicitation` so the turn resumes.

Already green: the dockerized Zed sync e2e (incl. Phase 18), the Go elicitation handler
tests, `tsc` and `vite build`. **Never proven:** that a human can see and answer a question
in the running Helix UI.

This task is **verification only**. No features, no refactors, no new PRs, no merges. The
deliverable is evidence — screenshots plus a written report — that the loop works for a real
user, or plainly-stated evidence that it does not. Three previous sandboxes died before
reaching this step, which is why it is now its own task.

## User Stories

### 1. As a reviewer, I want proof the question reaches a human
**Acceptance criteria**
- The inner dev stack from this branch is up: `helix-api-1`, `helix-frontend-1`,
  `helix-postgres-1` all `Up` and `http://localhost:8080` returns `200`.
- A **spec task** (not a bare chat session) exists whose `config->>'zed_thread_id'` is a
  non-empty UUID, proving Zed's workspace setup completed and the sync WebSocket opened.
- The task's agent runs the **Claude Code harness**. If no Claude-backed agent exists in the
  inner Helix, that is reported immediately as a **blocker**, not worked around.
- A prompt such as *"Before you write any code, ask me which caching backend to use — Redis,
  in-memory, or none"* provokes a real `AskUserQuestion` call.
- Screenshot: the question card rendered in the Helix conversation UI with **every option
  visible**, each showing its label *and* description, plus the "Other" free-text field.

### 2. As a reviewer, I want proof the answer is accepted and the turn resumes
**Acceptance criteria**
- Screenshot: the answer submitted, the card moving to an answered state, and the agent
  producing a **next message that reflects the chosen option**.
- The elicitation row reaches status `accepted` with `resolution_reason = answered`.

### 3. As a reviewer, I want proof the answer is part of the permanent record
**Acceptance criteria**
- Screenshot after a **full page reload**: the answered card still shows what the user
  chose, in conversation order — not just live DOM state.

### 4. As a reviewer, I want proof the session keeps working afterwards
**Acceptance criteria**
- A normal follow-up message in the same thread gets a normal reply.
- A **second** question is provoked and answered in the same session; the first answer does
  not poison the second (correct options rendered, correct answer applied).

### 5. As a reviewer, I want proof of the skip path
**Acceptance criteria**
- A question is skipped/declined; the turn settles cleanly and continues (per the design,
  decline is not an abort — it yields `{action:"answered", answers:{}}`).
- Screenshot of the skipped card and the continuing turn.

### 6. As a reviewer, I want proof of the interrupt path
**Acceptance criteria**
- A question is left unanswered and the turn is interrupted from Helix.
- The card **stops being answerable** (locked / cancelled with a reason), rather than sitting
  there pretending it can still be answered. Screenshot required.

### 7. As a reviewer, I want proof the blocked state is visible and respected
**Acceptance criteria**
- While a question is pending, the task list / detail header shows the task is waiting on a
  human (screenshot).
- The auto-wake worker does **not** "recover" the blocked interaction — evidenced by API
  logs (`[AUTO_WAKE] Interaction is waiting on a human answer…`) and by the interaction
  staying `waiting` with the card still answerable.

### 8. As a reviewer, I want an honest, durable report
**Acceptance criteria**
- Results written to `design/2026-08-12-agent-questions-live-verification.md` in the helix
  repo on branch `feature/002731-end-to-end-agent`, committed and pushed.
- The report states **what was observed**, not what should happen. Any failure is reported
  plainly with the failing evidence.
- If the model refuses to call `AskUserQuestion`, that is stated explicitly. If a sync event
  is injected to exercise the UI instead, every such screenshot is **labelled "injected"**.
- A summary comment is posted on https://github.com/helixml/helix/pull/3009. **Nothing is
  merged.**

## Non-Goals

- Any code change to helix, zed, or the frontend. If a defect is found, it is *reported*, not
  fixed, in this task.
- Opening new PRs or merging PR #3009 / zed PR #83.
- Re-running or extending the automated e2e suite; it is already green.

## Constraints (three sandboxes died here)

- Commit and push **every meaningful chunk immediately** — screenshots and partial report
  sections included. A previous run lost a six-hour turn to a dropped connection.
- Never blind-`sleep` on a long operation. Poll in short loops (a few seconds) so a finish —
  or a death — is noticed at once.
- Use `http://localhost:8080`, the inner dev stack, for everything. **Never** `api:8080`,
  which is the outer production stack.
- Stack boot takes several minutes; `curl` returning `000` early is "still booting", not
  "broken".

## Open Questions

1. **Blocked indicator surface.** Grepping the branch, the blocked-on-human signal is carried
   by the `agent_question` attention event (`api/pkg/types/attention_event.go:56`, copy
   "Agent is waiting on your answer") and by the auto-wake gate
   (`Store.HasLiveAgentElicitation`). There is no dedicated `blocked_on_user` badge field in
   `frontend/src`. Is the in-app bell / attention entry the intended evidence for story 7, or
   is a task-list badge expected to exist?
2. **Claude-backed agent availability.** Whether the inner dev stack has a Claude Code
   harness agent configured (and a working Anthropic key) is unknown until the stack is up.
   Assumption: if not, this is reported as a blocker and the remaining UI paths are exercised
   via labelled injection. Confirm that fallback is wanted rather than a hard stop.
3. **Provoking a second question reliably.** Whether the model will call `AskUserQuestion`
   twice on request is model behaviour, not a code guarantee. Assumption: retry the prompt a
   bounded number of times, then report "would not call the tool" plainly.
4. **Interrupt semantics.** Assumed expected outcome is the elicitation resolving to
   `cancelled` with reason `interrupted`. Confirm no separate user-visible copy is required.
5. **Report location.** Assumed the report lands in the **helix** repo at
   `design/2026-08-12-agent-questions-live-verification.md` (alongside
   `design/2026-08-11-agent-questions-elicitation.md`), not in helix-specs.
