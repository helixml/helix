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
`AcpThreadEvent::ElicitationRequested/ElicitationResponded` (`acp_thread.rs:2197-2198`);
status changes additionally emit `EntryUpdated(ix)`. Nothing needed is missing.

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
- [ ] The `question_<i>_custom` "Other" free-text input is rendered and submittable.
- [ ] 1–4 questions in one elicitation are all rendered; multi-select questions
      (`{"type":"array","items":{"anyOf":[…]}}`) render as multi-select.
- [ ] The card is rendered generically from the JSON Schema — no matching on the literal
      key `question_0`. Shapes the adapter also emits (MCP elicitation forwarding, the
      refusal-fallback consent dialog) and unknown shapes degrade to a generic form and
      never crash the conversation view.
- [ ] The question survives a page reload and an API/container restart (it is persisted,
      not held in memory).

### US-2 — Answer the question
**As a** Helix user,
**I want** to submit my answer from the Helix UI,
**so that** the agent's blocked turn resumes with that answer.

Acceptance criteria:
- [ ] Submitting sends the answer through the generated TS API client (no hand-rolled
      `fetch`/`api.post`) and immediately reflects the answered state.
- [ ] After answering, the card shows what the user chose — it is part of the permanent
      conversation record.
- [ ] The agent's turn resumes and its next message reflects the chosen answer.
- [ ] A decline path is available; the turn settles cleanly and the UI reflects it.
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
- [ ] A pending question that dies with the agent process (container restart) is not left
      answerable — it is reconciled to a terminal state on reconnect.

### US-4 — Blocked, not running
**As a** user scanning the task list,
**I want** a task blocked on a human answer to be visibly distinct from a running task,
**so that** I know it is waiting for *me*.

Acceptance criteria:
- [ ] The task list and the task detail header show "waiting for your answer" (or
      equivalent), consistent with existing status surfaces (Lucide icons, existing badge
      components).
- [ ] The stuck-interaction auto-wake worker
      (`api/pkg/server/auto_wake_stuck_interactions.go`) does not treat a
      waiting-on-a-human interaction as a hung agent, and a unit test proves it.

### US-5 — Nothing else breaks
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
5. [ ] Decline/cancel path settles cleanly.
6. [ ] Unanswered question + interrupt from Helix settles cleanly and stops being
       answerable. (Answering from the Zed desktop is **not** a test path — that panel is
       broken. Status transitions are still implemented properly.)
7. [ ] Go unit tests for the new handler paths, in the style of
       `api/pkg/server/websocket_external_agent_sync_test.go`.
8. [ ] New E2E phase in the Zed WebSocket sync e2e suite (elicitation-request → answer →
       turn-resumes) and the **full dockerized e2e run green**
       (`crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`). "Compiles" and
       "follows the pattern" are not evidence.
9. [ ] `design/2026-08-11-agent-questions-elicitation.md` written in the helix repo.
10. [ ] Cross-repo merge order followed exactly (see design doc).

## Out of Scope

- Disabling `AskUserQuestion` (upstream puts it in `disallowedTools`; we are doing the
  opposite).
- A Helix MCP `ask_user` tool as a substitute — the ACP elicitation channel already exists.
- Fixing the Zed agent-panel rendering of elicitation forms. It stays broken.
- Rebasing Zed onto upstream. The pinned `ZED_COMMIT` already has full elicitation support.

## Open Questions

1. **Storage shape.** The design proposes a hybrid: an authoritative `agent_elicitations`
   row (for status races, auth, and cheap "which tasks are blocked" queries) plus an
   inline `ResponseEntry` of a new type `elicitation` on the interaction (for in-order
   conversation rendering and the permanent record). Entry-only is simpler but makes the
   task-list badge and the two-clients-answering race awkward. Confirm the hybrid, or say
   "entry only".
2. **Attention/notification surface.** Should a pending question also raise an
   `AttentionEvent` (proposed new type `agent_question`, mirroring the existing
   `org_message` "ask_human" inbox) so the notification bell rings, or is the task-list
   badge enough for v1?
3. **Submit granularity.** Assumption: one Submit button per elicitation covering all 1–4
   questions (one elicitation = one ACP request = one response). Confirm, rather than
   per-question submit.
4. **Decline vs cancel in the Helix UI.** Assumption: expose **Submit** and **Decline**
   only; `cancel` is produced by interrupt/teardown, not by a user button (the Zed panel
   offers all three, but a user-facing "Cancel" is confusing next to "Decline").
5. **Pending question at container restart.** Assumption: the `respond_tx` dies with the
   agent process, so the question can never be answered; on reconnect (`agent_ready`) any
   still-pending elicitation for that session is reconciled to `cancelled` and the card
   renders as "expired — the agent restarted". Confirm this over attempting any replay.
6. **E2E determinism.** The new phase drives a real Claude Code agent with a prompt that
   instructs it to call `AskUserQuestion`. This is model-dependent. If it proves flaky, is
   a synthetic elicitation injected at the Zed test seam an acceptable fallback for the
   phase, with the model-driven path covered only by the live inner-Helix test?
7. **Org-layer activation timeout.** `auto_wake_stuck_interactions.go` is named in the task,
   but the activation spawner
   (`api/pkg/org/infrastructure/runtime/helix/spawner.go`) has its own timeout that can
   spawn a decoy `waiting` interaction. Should that path also learn about
   blocked-on-human, or is auto-wake the only gate in scope?
