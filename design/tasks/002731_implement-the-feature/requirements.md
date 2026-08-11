# Requirements: End-to-End Agent Questions via ACP Elicitations

## Background

Claude Code (and any ACP agent) can ask the user a question mid-turn. Claude does this
with its built-in `AskUserQuestion` tool; `claude-agent-acp` converts that into an ACP
elicitation (`session/request_elicitation`, mode `form`) and blocks the turn until the
client answers.

That question never reaches a Helix user today. Verified root cause:

- `crates/external_websocket_sync/` has **no concept of elicitations**. The entry mapping
  in `thread_service.rs` (the `cx.subscribe(thread_entity, …)` handler, arms at
  `thread_service.rs:914`, `:1051`, `:1112`) matches only `UserMessage | AssistantMessage |
  ToolCall` and `_ => return`s everything else. `acp_thread::AgentThreadEntry::Elicitation`
  (`crates/acp_thread/src/acp_thread.rs:407`) is silently dropped.
- `grep -rn "elicitation" api/` in the helix repo returns nothing — no event, no storage,
  no endpoint, no UI.

Consequence: Helix shows only the dead tool-call stub the adapter emitted just before the
elicitation, the interaction sits in `waiting` forever, and interrupting it kills the turn
with `Tool permission request failed: Tool use aborted`.

Real occurrences: interaction `int_01kzr7qjt696r60a1wydpscgq8` (19 minutes of silence, dead
turn) and the deterministic reproduction on 2026-08-11 (spec task
`spt_01kzr91dxm98jtyfybj22keeyg`, session `ses_01kzr91dywnbx4kazscdnm0s6z`), where the whole
stored user-visible surface was:

```
**Tool Call: Which colour should I use?**
Status: Pending

Which colour should I use?
```

The attached screenshot (`attachments/zed-elicitation-no-fields.jpg`) shows the Zed agent
panel rendering the same elicitation with a card, a message, Submit/Decline/Cancel buttons
and **no option controls and no text field** — confirming the Zed panel is useless for
answering and Helix must be self-sufficient.

Verified as reachable from `external_websocket_sync` without touching `agent_ui`:
`AcpThread::elicitation(&id)` → `&Elicitation { id, request, status }`
(`acp_thread.rs:3607`), carrying the full `acp::CreateElicitationRequest` (message,
`ElicitationMode::Form(mode).requested_schema`, `tool_call_id`);
`AcpThread::respond_to_elicitation(&id, response, cx)` (`acp_thread.rs:3548`);
`AcpThreadEvent::ElicitationRequested/ElicitationResponded` (`acp_thread.rs:2197`);
status changes additionally emit `EntryUpdated(ix)`. Nothing needed is missing.

### Schema facts re-verified against the deployed adapter (0.66.0)

Read directly from
`.zed-state/local-share/external_agents/registry/npx/claude-acp/node_modules/@agentclientprotocol/claude-agent-acp/dist/elicitation.js`
(three checked copies on this machine, all `"version": "0.66.0"`). Two details in the task
brief are **wrong** and the implementation must follow the source, not the brief:

| Brief says | Adapter 0.66.0 actually uses |
|---|---|
| `_meta: {"claudeCode/optionPreview": {...}}` | `_meta: {"_claude/askUserQuestionOption": {preview}}` (`elicitation.js:91`) |
| `_meta: {"claudeCode/customAnswer": {...}}` | `_meta: {"_askUserQuestionCustomAnswer": {questionId, isCustomAnswer}}` (`elicitation.js:98`) |

Everything else in the brief checks out (`elicitation.js:115-166`): field keys
`question_<i>` / `question_<i>_custom`; `title` = the question's `header`; `description` =
the question text **only** when there are 2+ questions (with one question the prompt is in
`message`, and with several `message` is the literal string `"Please answer the following
questions."`); options are `{const: label, title: label, description?}`; multi-select is
`{type:"array", items:{anyOf:[…]}}`; **nothing is ever `required`**, so skipping is legal;
`toolCallId` is set when present.

Because the meta key names have already changed once, the frontend parser must key off the
**value shape** (`isCustomAnswer === true`, or a `questionId` naming a sibling property),
not off any literal meta-key string.

### Response semantics, read from the adapter (`applyAskElicitationResponse`, `elicitation.js:180-210`)

- **Custom beats selection, per question**: if `question_<i>_custom` is a string that is
  non-empty after `.trim()`, it is the answer and `question_<i>` is never read. So the
  "none of the above" case — custom filled, `oneOf` field left unset — is valid and must
  work in the Helix UI.
- Answers are keyed by the **question text**, not by `question_<i>`; multi-select values are
  joined with `", "`.
- A question with neither field set is simply omitted from `answers` — partial submission
  is legal, not an error.
- **`decline` is not an abort**: it yields `{action:"answered", answers:{}}`. The tool call
  succeeds, the model is told the user skipped, and the turn continues.
- `cancel` (and any action the adapter doesn't understand) aborts the tool call. That is the
  interrupt/teardown outcome, not a user button.

## User Stories

### US-1 — See the question
**As a** Helix user whose agent is running a task,
**I want** the agent's question rendered in the task/session conversation with all its
options,
**so that** I can answer instead of watching the turn die.

Acceptance criteria:
- [ ] A pending elicitation renders inline in the conversation at the position where the
      tool-call stub appears today, in correct order relative to surrounding entries.
- [ ] The card shows: the elicitation `message`, per question the short `header` chip
      (`title`), the question text, and every option as a clickable control showing its
      **label and its description**.
- [ ] The `question_<i>_custom` "Other" free-text input is rendered and submittable, and is
      submittable **alone** (the "none of the above" case).
- [ ] 1–4 questions in one elicitation are all rendered; multi-select questions
      (`{"type":"array","items":{"anyOf":[…]}}`) render as multi-select.
- [ ] The card is rendered generically from the JSON Schema — no matching on the literal
      key `question_0` and no matching on a literal `_meta` key name. Shapes the adapter
      also emits (MCP elicitation forwarding, the refusal-fallback consent dialog) and
      unknown shapes degrade to a generic form and never crash the conversation view.
- [ ] The question survives a page reload, an **API restart**, and a container restart (it
      is persisted, not held in memory).

### US-2 — Answer the question
**As a** Helix user,
**I want** to submit my answer from the Helix UI,
**so that** the agent's blocked turn resumes with that answer.

Acceptance criteria:
- [ ] Submitting sends the answer through the generated TS API client (no hand-rolled
      `fetch`/`api.post`) and immediately reflects the answered state.
- [ ] One Submit covers all questions in the elicitation (one elicitation = one ACP request
      = one response).
- [ ] Submitted content mirrors the adapter's precedence exactly: a non-empty trimmed
      custom answer wins over that question's selection; unanswered questions are omitted.
- [ ] After answering, the card shows what the user chose — it is part of the permanent
      conversation record.
- [ ] The agent's turn resumes and its next message reflects the chosen answer.
- [ ] A **Decline** control is available. Per the adapter this means "skipped, empty
      answers" and the turn continues normally — the UI copy must not imply the turn was
      aborted. There is no user-facing "Cancel" button; `cancel` is a teardown/interrupt
      outcome only.
- [ ] Only a user authorised on that session/task may answer. The session is taken from
      the URL only; nothing in the request body identifying a session is trusted.
- [ ] Answering an elicitation that is already answered/declined/cancelled, or whose thread
      is gone, is a clean no-op with a reported error — no panic, no wedged turn.

### US-3 — Two surfaces that agree
**As a** user with both Zed and Helix open,
**I want** the two views of a question to agree,
**so that** I never answer a question that is already resolved.

Acceptance criteria:
- [ ] Status transitions (pending → accepted / declined / cancelled / completed) are
      mirrored from Zed into Helix, and Helix stops offering to answer within the same
      live session — no manual refresh.
- [ ] A question nobody answers settles cleanly when the turn is interrupted from Helix,
      and the Helix UI stops offering to answer it.
- [ ] **A reconnect alone never resolves a question.** An API restart (the commonest cause
      of a reconnect — the desktop container, the Zed process and the `respond_tx` all
      survive it) leaves a pending question pending and still answerable.
- [ ] A question is reconciled to a terminal state only on positive evidence from Zed:
      it is absent from Zed's post-reconnect resync (after a grace window), or a
      `respond_elicitation` is acked `not_found`. A genuinely dead question (real container
      restart) therefore still stops being answerable, and the card reads "expired — the
      agent restarted".

### US-4 — Blocked, not running — and I get told
**As a** user who is not staring at the task list,
**I want** a notification when an agent asks me something, plus a task-list badge that
distinguishes "blocked on me" from "running",
**so that** I find out without having to watch.

A question nobody notices is the same as no question, so the notification is **required**,
not a nice-to-have. It is also cheap: `AttentionService.EmitEvent`
(`api/pkg/services/attention_service.go:59`) already fans one emit out to the in-app
notification bell, a threaded Slack reply on the project's channel (`notifySlack`, `:180`),
and the org spec-task event sink. The work is a new event type plus its title/description/
emoji cases and a per-event dismissal — no new delivery surface.

Acceptance criteria:
- [ ] A pending question raises an `agent_question` attention event via
      `attentionService.EmitEvent`, with the **elicitation id as the idempotency qualifier**
      so resync re-announcements and retries cannot spam the user with duplicates.
- [ ] The notification reaches the bell **and** the project's Slack thread where a Slack
      trigger is configured, because both fall out of the same emit.
- [ ] The notification text names the task and says the agent is waiting for an answer —
      new cases in `buildTitle` (`:281`), `buildDescription` (`:302`) and `eventEmoji`
      (`:331`); it must not fall through to a generic default.
- [ ] The event is dismissed when the elicitation reaches **any** terminal status
      (answered, declined, cancelled, expired), so a resolved question stops nagging.
      Dismissal is per-event (keyed by the elicitation id), not the existing task-wide
      `DismissTaskAttentionEvents`.
- [ ] The "user is already active in this session" suppression that
      `agent_interaction_completed` applies (`websocket_external_agent_sync.go:3200-3242`)
      is **not** copied here. A question needs an answer whether or not the user is looking
      at the session.
- [ ] The task list and the task detail header show "waiting for your answer" (or
      equivalent), consistent with existing status surfaces (Lucide icons, existing badge
      components).
- [ ] The stuck-interaction auto-wake worker
      (`api/pkg/server/auto_wake_stuck_interactions.go`) does not treat a
      waiting-on-a-human interaction as a hung agent, and a unit test proves it.

### US-5 — Follow-up while a question is pending
**As a** user looking at a question card with the composer right below it,
**I want** typing a normal message instead of answering to do something sensible and
visible,
**so that** my message is never silently swallowed.

Acceptance criteria:
- [ ] Sending a normal chat message while a question is pending **delivers the message**.
      It must not sit in the prompt queue undispatched. (Today it would: `processPromptQueue`
      defers any non-interrupt prompt while the newest interaction is `state=waiting`
      — `websocket_external_agent_sync.go:3392` — and a pending question keeps it `waiting`
      indefinitely, with auto-wake now deliberately gated off.)
- [ ] The behaviour mirrors Zed rather than inventing a third rule: `AcpThread::run_turn`
      unconditionally calls `cancel_inner(RequestPermissionOutcome::InterruptedByFollowUp)`
      (`acp_thread.rs:3785`), which calls `cancel_outstanding_elicitations`
      (`acp_thread.rs:3987`, `:4065`). So a follow-up cancels the outstanding elicitation
      and the new turn proceeds.
- [ ] The card locks and reads "you replied instead" (or equivalent) once the resulting
      `cancelled` status arrives.

### US-6 — Nothing else breaks
**As a** developer,
**I want** the loop to be provably non-poisoning,
**so that** a session that asked a question behaves normally afterwards.

Acceptance criteria:
- [ ] After an answer lands, a normal follow-up message in the same thread is delivered and
      answered normally.
- [ ] A second question in the same session works — the first answer does not poison it.
- [ ] `cd frontend && yarn build` clean; `go build ./pkg/...` clean; `cargo build --features
      external_websocket_sync -p zed` clean; CI green on both PRs.

## Definition of Done (evidence required)

Per `CLAUDE.md`, live external-agent testing is mandatory for anything touching
session/thread lifecycle. Seeded rows and unit tests alone are **not** acceptance evidence.

1. [ ] Live spec task in the inner Helix; agent calls `AskUserQuestion`.
2. [ ] Question appears in the Helix task UI with all its options — **screenshot** saved to
       this task's `screenshots/`.
3. [ ] Answered in the Helix UI; the turn resumes and the next message reflects the answer —
       **screenshot / pasted transcript**.
4. [ ] Next-operation test: follow-up message delivered and answered; a second question in
       the same session works.
5. [ ] Follow-up-instead-of-answering test: with a question pending, send a normal message;
       it is delivered, the turn proceeds, and the card locks as cancelled.
6. [ ] Custom-answer test: submit only the "Other" free-text field with no option selected;
       the agent receives the typed text.
7. [ ] Decline path: the turn continues with empty answers and settles cleanly.
8. [ ] Unanswered question + interrupt from Helix settles cleanly and stops being
       answerable. (Answering from the Zed desktop is **not** a test path — that panel is
       broken. Status transitions are still implemented properly.)
9. [ ] **API-restart test**: with a question pending, restart the API (Air rebuild);
       after reconnect the question is still shown and still answerable, and answering it
       still resumes the turn.
10. [ ] Task list/detail header shows "blocked on human answer" while pending, **and** the
        notification arrives — bell entry present (and the Slack thread reply where a Slack
        trigger is configured), then dismissed once the question is resolved. Screenshot the
        notification.
11. [ ] Go unit tests for the new handler paths, in the style of
        `api/pkg/server/websocket_external_agent_sync_test.go`.
12. [ ] New E2E phase in the Zed WebSocket sync e2e suite (elicitation-request → answer →
        turn-resumes), driven by a **synthetic** injection at the Zed test seam rather than
        by the model choosing to call a tool, and the **full dockerized e2e run green**
        (`crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`). "Compiles" and
        "follows the pattern" are not evidence.
13. [ ] `design/2026-08-11-agent-questions-elicitation.md` written in the helix repo.
14. [ ] Cross-repo merge order followed exactly (see design doc).

## Out of Scope

- Disabling `AskUserQuestion` (upstream puts it in `disallowedTools`; we are doing the
  opposite).
- A Helix MCP `ask_user` tool as a substitute — the ACP elicitation channel already exists.
- Fixing the Zed agent-panel rendering of elicitation forms. It stays broken.
- Rebasing Zed onto upstream. The pinned `ZED_COMMIT` already has full elicitation support.
- Teaching the org-layer activation spawner (`api/pkg/org/infrastructure/runtime/helix/
  spawner.go`) about blocked-on-human. Auto-wake is the only gate in this task; the spawner
  timeout is recorded as a known follow-up in the design doc.

## Resolved decisions (from spec review)

1. Storage: **hybrid** — authoritative `agent_elicitations` row plus an inline
   `ResponseEntry` of type `elicitation`, with single-writer discipline so the two cannot
   drift.
2. Attention surface: **in scope** — `agent_question` attention event, raised on pending,
   cleared on any terminal status.
3. Submit granularity: **one Submit per elicitation**, covering all its questions.
4. Controls: **Submit + Decline only**; `cancel` stays a teardown/interrupt outcome.
5. Restart handling: outcome unchanged (a dead question must not stay answerable), but the
   **trigger is positive evidence from Zed**, never the reconnect itself.
6. E2E: the required CI phase is **synthetic**; the model-driven path is covered by the live
   inner-Helix test, where a flake costs nothing.
7. Org spawner: **out of scope**, recorded as a follow-up.

## Open Questions

1. **"Decline" wording.** Per the adapter, decline means "skipped, empty answers, turn
   continues". Assumption: label the control **"Skip"** with helper text "the agent
   continues without your answer", since "Decline" reads like refusing the whole turn. Say
   if you want the literal word "Decline" kept for consistency with the Zed panel.
2. **Resync grace window.** Reconciling a pending row that is absent from Zed's resync needs
   a grace window to absorb ordering races (resync arriving before a thread finishes
   loading). Assumption: 60 s, and only for rows already older than that. Confirm the
   number, or say it should be configurable via env like
   `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` is.
3. **Optimistic card lock on follow-up.** When the user sends a normal message with a
   question pending, should the card lock immediately (optimistic, reconciled by the
   `cancelled` event), or wait for the event so the UI never shows a state the backend
   hasn't confirmed? Assumption: lock immediately with a "replying instead…" state, since
   the Zed behaviour is deterministic.
