# Agent elicitations: agent asks the user a question mid-turn

Date: 2026-09-12
Status: implemented and live-validated across Qwen, GLM, Claude, and Codex

## Problem

When a coding agent needs information mid-turn (clarification, a decision between
approaches, a preference), there is no way for it to pause and ask the user in
Helix. Today:

- The turn either guesses, or the model's `AskUserQuestion` / `ask_user_question`
  tool has nowhere to surface, stalls the turn, and dies to the auto-wake worker
  (`api/pkg/server/auto_wake_stuck_interactions.go`) which pokes it with
  "continue" after 180s — cancelling the pending question
  (Zed maps a follow-up while a permission request is pending to
  `RequestPermissionOutcome::InterruptedByFollowUp` → ACP `Cancelled`).
- Frontend has no question UI at all; tool calls are read-only
  (`CollapsibleToolCall.tsx`).
- Teams progress struct has dead `needs_input` fields that were never wired
  (`api/pkg/trigger/teams/agent_progress.go:16-27`).

Goal: the agent can ask; the user answers (options or free text); the turn
continues with the answer injected as the tool result. Match t3code's UX on the
frontend. Reuse Zed's existing ACP elicitation/permission machinery on the
backend; extend the websocket-sync protocol to carry questions and answers.

## What already exists

### Zed fork (helixml/zed, client side) — almost everything

Zed (ACP 2.0.0 with `unstable` feature) already implements both transports an
agent can use to ask:

1. **ACP elicitation** (`elicitation/create` + `elicitation/complete`):
   `crates/agent_servers/src/acp.rs` `handle_create_elicitation` (:4838) inserts
   an `AgentThreadEntry::Elicitation` into the thread with
   `ElicitationStatus::Pending { respond_tx }`
   (`crates/acp_thread/src/acp_thread.rs:407-437`); answers via
   `AcpThread::respond_to_elicitation` (:593-660); cancel via
   `cancel_elicitation`. Client capability `elicitation(form + url)` is
   advertised to agents (`client_capabilities_for_agent`, acp.rs:813). A pending
   elicitation blocks the turn (`is_waiting_for_confirmation`, acp_thread.rs:2488).
2. **Tool approval** (`session/request_permission`):
   `handle_request_permission` (acp.rs:4786) →
   `AcpThread::request_tool_call_authorization` (:3424) →
   `ToolCallStatus::WaitingForConfirmation { options, respond_tx, kind }` →
   `authorize_tool_call` (:3482).

The implementation gap in Zed was the **external-websocket-sync bridge**:
`crates/external_websocket_sync/src/thread_service.rs` subscribes only to
`NewEntry/EntryUpdated/Stopped/Error` (:898, :1035, :1087) — it never observes
`ElicitationRequested` or `ToolAuthorizationRequested`, and its `NewEntry`
mapper explicitly `_ => return`s on `Elicitation` entries (:905-913).
`SyncEvent` (src/types.rs:179-280) had no question event; the command set
(websocket_sync.rs:399-407) had no answer command. Consequently, headless external threads
stalled on any question (confirmed in `portingguide.md:820-821` —
`_request_elicitation_subscription = None`).

MCP-server elicitation is **not** supported by Zed's MCP client at all
(`crates/context_server/src/client.rs`, `docs/src/ai/mcp.md:15`); out of scope
for v1.

### Harness transports (what actually arrives at Zed)

| Harness | Transport | Structured payload | Answers ride |
|---|---|---|---|
| **Claude Code** (`claude-agent-acp` ≥ ~0.76) | ACP `elicitation/create` **form** | AskUserQuestion converted to form fields (enum per option, `const=label`, `title`, `description`; single question carried in `message`, multi-question one field per question; `multiSelect` → array field) | `ElicitationResponse {action: "accept", content: {field_key: label}}`; wrapper maps back to SDK `behavior:"allow", updatedInput:{questions, answers:{questionText: label}}` |
| **Qwen Code** (bundled `qwen-code-build`) | `session/request_permission` with `toolCall._meta: {toolName:"ask_user_question", qwenInteractionKind:"user_question", qwenQuestions:[{header, question, options[{label,description}], multiSelect}]}`; offered options are only `Submit(proceed_once)` / `Cancel` | full structured questions; `getDefaultPermission()` returns `"ask"` in ACP mode **regardless of yolo** | response `{outcome:{outcome:"selected",optionId:"proceed_once"}, answers:{<index>: "<answer>"}}` — the top-level `answers` map is a Qwen extension keyed by question index |
| **Codex** (`@agentclientprotocol/codex-acp` 1.11.0) | Codex App Server `item/tool/requestUserInput`, native in Plan mode and feature-gated in Default mode | one to three structured questions with options and an optional free-form answer | ACP form elicitation; accepted content is converted back to Codex `{answers: {question_id: {answers: [...]}}}` |
| opencode / goose | approvals only, no ask-user tool verified in the current adapters | — | — |

Key facts verified in the shipped artifacts:
- `claude-agent-acp` 0.23.1 hard-disabled AskUserQuestion
  (`disallowedTools = ["AskUserQuestion"]`); **0.76.0 (current npm) intercepts
  it and surfaces it as a form elicitation, before and immune to
  bypassPermissions** ("a request that still reaches this callback is
  deliberately bypass-immune"). Helix configures `default_mode:
  bypassPermissions` (settings-sync-daemon main.go:352), so with a current
  wrapper the question WILL reach Zed. The wrapper version floats with Zed's
  network agent registry (`AgentRegistryStore`), not pinned by Helix.
- Qwen's ask_user_question works only when the ACP host relays
  `session/request_permission` (`"Cannot ask user questions in non-interactive
  mode without ACP support…"`). Helix's `--yolo` + `default_mode: yolo`
  (settings-sync-daemon main.go:248-275) does NOT suppress it: the "ask"
  permission is per-tool. The request is emitted with
  `_meta.qwenInteractionKind = "user_question"`; before
  https://github.com/helixml/zed/pull/95 Zed ignored that metadata and nothing
  could answer it headlessly.
- Codex has a structured `request_user_input` protocol and answer operation.
  T3 Code demonstrates the direct App Server architecture: render
  `item/tool/requestUserInput`, answer the server request, and resume the turn.
  In Plan mode this is native; Default mode additionally requires the
  under-development `default_mode_request_user_input` feature. Sources:
  https://developers.openai.com/codex/app-server/ and
  https://github.com/pingdotgg/t3code/pull/6432.
- The current Zed registry installs `@agentclientprotocol/codex-acp` 1.11.0,
  whose `CodexElicitationHandler` already converts the App Server request to
  ACP form elicitation. Support landed upstream in April 2026:
  https://github.com/agentclientprotocol/codex-acp/commit/2798159140a128ef2375eca1c9336cb2179b6960.
  Helix enables the Default-mode feature in Codex's generated config. Zed also
  recognizes codex-acp's `_meta.codex.isOtherAnswer` companion property so it
  renders one question and returns custom text through the correct field.

### t3code reference (pingdotgg/t3code) — the model to copy

t3code normalizes all providers to two event pairs in its runtime event stream
(`packages/contracts/src/providerRuntime.ts`):

- `request.opened` / `request.resolved` — approvals, `CanonicalRequestType`:
  `command_execution_approval | file_read_approval | file_change_approval |
  apply_patch_approval | exec_command_approval | mcp_elicitation_approval |
  tool_user_input | dynamic_tool_call | auth_tokens_refresh | unknown`,
  payload `{requestType, detail?, appName?, options[] (decision+label+warning), args?}`.
- `user-input.requested` / `user-input.resolved` — **structured questions**,
  payload `{questions: UserInputQuestion[], responseMode?: "message"}` with
  `UserInputQuestion { id, header, question, options: [{label, description,
  value?}], allowCustomAnswer?, multiSelect? }` and answers as
  `Record<questionId, unknown>`.

Domain commands (`packages/contracts/src/orchestration.ts`):
`thread.approval.respond {requestId, decision}`,
`thread.user-input.respond {requestId, answers, attachmentsByQuestionId?}`,
`thread.user-input.dismiss {requestId}` (dismiss releases the composer without
messaging the agent; provider-blocked questions can't be dismissed). Decisions:
`accept | acceptForSession | acceptAlways | decline | cancel`.

Provider layer (`apps/server/src/provider/Services/ProviderAdapter.ts`):
adapters implement `respondToRequest` + `respondToUserInput`. Claude path:
SDK `canUseTool` intercepts AskUserQuestion, emits `user-input.requested`,
blocks on a Deferred until the user answers, then returns
`behavior:"allow", updatedInput:{questions, answers:{questionText: label}}`
(`apps/server/src/provider/Layers/ClaudeAdapter.ts:4263-4400`). Question `id`
MUST equal the full question text (Claude SDK ≥2.1.121 looks answers up by
question text — t3code issue #2388). Abort while pending settles as
aborted/deny. OpenCode path: native question requests normalized
(`OpenCodeAdapter.ts` ~1832); full-access mode auto-replies.

Projection: pending approvals/questions persist in the thread projection
(`ProjectionPendingApprovalStatus` = pending/resolved) and replay to clients on
connect.

Frontend (`apps/web/src/components/chat/`):
- `ComposerPendingUserInputPanel.tsx` — compact collapsible banner attached to
  the composer: one question at a time, flat option rows, numeric shortcuts,
  a 200ms optimistic single-select auto-advance, multi-select, and a 1/N
  counter. Current t3code uses the main composer for custom answer text and
  per-question attachments (≤8 files).
- `ComposerPendingApprovalPanel.tsx` — compact approval strip (kind label,
  detail, 1/N counter) for the approval flow.
- `MessagesTimeline.tsx` — answered questions render inline in the thread
  (QuestionAnswerHistory with question text, answer text, attachments).
- Pending question counts into the composer "needs confirmation" state; sending
  from the composer advances/replaces the pending question.

## Design

### Principle

The turn does not change its basic Helix lifecycle: it stays
`InteractionStateWaiting` until the agent finishes. A pending question is
**state on the in-flight interaction**, not a new interaction state — same
choice t3code makes (questions are activities + pending projection, not turn
states). This keeps the prompt queue, streaming, resume and orphan paths
working unchanged, with two carve-outs (auto-wake and resume) below.

### Protocol (Zed ↔ Helix websocket sync) — new

Events (Zed → Helix), added to `SyncEvent` and the Go switch in
`api/pkg/server/websocket_external_agent_sync.go:741-774`:

- `question_requested`:
  `{thread_id, request_id, turn_request_id, source: "elicitation"|"permission", questions: [UserQuestion], tool_call_id?}`
  with `UserQuestion { id, header, question, options: [{label, description}],
  multi_select, allow_custom_answer }` (t3code's shape, snake_cased).
  Emitted when a form elicitation arrives, or when a permission request carries
  `_meta.qwenInteractionKind == "user_question"` (parsed by Zed — see below).
  Generic permission approvals (allow/reject cards) are NOT questions and stay
  out of v1.
- `question_resolved`:
  `{thread_id, request_id, turn_request_id, outcome: "answered"|"cancelled", answers?}`
  Emitted when the question is answered, cancelled (incl.
  cancelled-by-follow-up, turn cancel, agent death), or expired.

Commands (Helix → Zed), added to `ExternalAgentCommand`:

- `respond_question`: `{request_id, answers: Record<string,string>}` — answers
  keyed by question id.
- `cancel_question`: `{request_id}`.

Zed-side handling per source:

- **Elicitation (Claude)**: `respond_question` →
  `AcpThread::respond_to_elicitation(id, ElicitationResponse{action:"accept", content: {field_key: answer}})`;
  `cancel_question` → `cancel_elicitation`. Emitted on
  `ElicitationRequested` events (subscribe in thread_service) plus the existing
  entry pipeline. Elicitation entries must also flow through `NewEntry` mapping
  (replace the `_ => return` skip) so answered Q&A shows in thread history and
  survives reconnect.
- **Qwen permission**: `respond_question` → respond to the pending
  `session/request_permission` with
  `{outcome: {outcome:"selected", optionId:"proceed_once"}, answers: {<index>: answer}}`
  (index-keyed, matching Qwen's answer keys);
  `cancel_question` → `authorize_tool_call` with the cancel outcome
  (`InterruptedByFollowUp`-equivalent / ACP `Cancelled`).
  Parsing happens in `handle_request_permission` (acp.rs): detect the meta,
  build `UserQuestion[]`, keep the respond_tx, emit the sync event.

Claude 0.76 emits an additional `_askUserQuestionCustomAnswer` schema property
beside every real question. The bridge filters those helper properties from the
normalized question list and remembers the companion field id. A selected
option is returned through the question's enum field; free text is returned
through its companion custom-answer field.

`turn_request_id` is the durable Helix interaction correlation ID. Keeping it
separate from the question's id avoids guessing the active interaction from a
thread when a reconnect overlaps a follow-up turn.

Reconnect/resume: Zed keeps native pending responders in its thread service and
re-emits `question_requested` for all still-pending requests after the external
WebSocket reconnects. Helix re-attaches idempotently by `request_id`; resolved
request ids in question history cannot be resurrected by a stale replay. If
`agent_ready.active_turns` authoritatively reports that the turn was lost (for
example after a Zed process restart), Helix archives the stale question as
cancelled before re-delivering the turn.

### Helix API

1. **Types** (`api/pkg/types/`): `PendingQuestion {RequestID, ThreadID, TurnRequestID, Source,
   Questions []UserQuestion, AskedAt}`; add `Interaction.PendingQuestion
   *PendingQuestion` (JSONB). No new `InteractionState`.
2. **Sync handlers** (`websocket_external_agent_sync.go`):
   - `question_requested`: attach to the open `Waiting` interaction (match by
     `thread_id`/active turn like `message_added` does), persist, publish an
     `interaction_update` event to the frontend. If no active interaction
     matches (resumed turn), create/attach per the existing resume rules.
   - `question_resolved`: clear the pending question; persist the resolved
     outcome for audit; the agent's own `tool_call` completion / elicitation
     result arrives via the normal entry pipeline afterwards — the frontend
     renders the answer summary from the question payload + answers, no new
     rendering source needed.
3. **REST endpoints** (response path — REST, not ws, for auth/retry/axios
   client conventions):
   - `POST /api/v1/interactions/{interaction_id}/questions/{request_id}/respond`
     `{answers}` → forwards `respond_question`.
   - `POST .../cancel` → forwards `cancel_question`.
   Auth via existing `loadAuthorizedInteraction`-style checks. Idempotent:
   answering an already-resolved question is a no-op 200.
4. **Auto-wake carve-out** (`auto_wake_stuck_interactions.go`): a pending
   question suppresses only the generic continue wake sent to a connected
   agent. A disconnected session still takes the bounded cold-start recovery
   path so Zed can reconnect and report whether it owns the turn.
5. **Terminal cleanup**: cancellation, agent errors, completion, and orphan
   reaping archive any remaining pending question as cancelled. The orphan
   reaper performs that update in the same transaction that moves the turn to
   `interrupted`, so terminal interactions never expose a phantom prompt.
6. **Teams progress** is unchanged. `AgentProgressUpdate` has presentation
   fields for input, but there is no producer for that type anywhere in the
   current tree; introducing a separate Teams progress pipeline is not part of
   the elicitation transport.

### Frontend (mirror t3code)

- New `PendingQuestion` state from the `interaction_update` event.
- **Question banner** (`components/session/PendingQuestionCard.tsx`) attached
  immediately above the composer while the latest interaction has a pending
  question. It uses the same compact disclosure header, flat option rows,
  number-key shortcuts, optimistic 200ms single-select advance, multi-select,
  dismiss action, and 1/N counter as t3code. Helix v1 keeps custom text and the
  multi-select submit action in the banner because its busy composer still owns
  follow-up/interrupt semantics; this is a behavioral difference, not a new
  visual language.
- **Timeline history**: resolved questions replace their matching raw tool call
  with the t3code work-log treatment (`User input submitted · <answer>` or
  `User input dismissed`). Expanding the row shows `QuestionAnswerHistory`.
  Providers that do not emit a matching tool call get the same synthesized
  work-log row from the resolved payload.
- **Composer stays usable**: sending a message while a question is pending
  cancels the pending question (Zed's InterruptedByFollowUp path) — same as
  t3code. Show the pending badge.
- Answer via the REST endpoints; optimistic UI; reconcile on
  `question_resolved` / `interaction_update`.

### What is explicitly out of scope (v1)

- MCP elicitation (Zed MCP client doesn't support it; our MCP surface is
  ListTools+CallTool only).
- URL elicitations (auth flows; no headless browser).
- Answer attachments (t3code has them; defer until the base flow works).
- Generic tool-approval cards (allow/reject for shell commands etc.) — can
  reuse this pipe later (`source: "permission"` non-question kinds), but
  approval policy is a separate product decision.
- opencode/goose ask-user (no structured ask-user path was verified in the
  current adapters).

## Implementation plan (PRs)

**PR 1 — Zed: sync protocol plumbing**

https://github.com/helixml/zed/pull/95

Merged as `baff1b4a4a33364538e9bf8957988faf515cf266`.

- `external_websocket_sync`: subscribe `ElicitationRequested` (+ resolved),
  emit `question_requested`/`question_resolved`; parse Qwen
  `_meta.qwenInteractionKind` in permission handling (acp.rs) into the same
  event; implement `respond_question`/`cancel_question` commands; include
  pending questions in resume/`active_turns`; let `Elicitation` entries flow
  through `NewEntry` mapping.
- Tests cover Claude/Qwen normalization, Qwen's extended permission response,
  cancellation, and reconnect replay. The existing dockerized external-sync
  smoke test must pass with the resulting Zed binary.

**PR 2 — Helix: API, frontend, and Zed pin**

https://github.com/helixml/helix/pull/3220

Merged as `d583d519bfc131a914214dd1a7b45c1dcd572881`.

- Protocol types + handler switch entries; `PendingQuestion` on interaction;
  persistence + websocket publish; REST respond/cancel endpoints; auto-wake
  carve-out; and resume re-attach. Teams progress remains unchanged because it
  has no producer in the current runtime.
- Question banner, timeline history, respond/cancel wiring, and pending-state
  behavior.
- Pin the Zed commit in `sandbox-versions.txt` per the two-repository merge
  order.

**Codex follow-up**

https://github.com/helixml/zed/pull/96

https://github.com/helixml/helix/pull/3221

- Enable `features.default_mode_request_user_input` in the Codex config written
  by settings-sync-daemon.
- Normalize codex-acp's form metadata for free-form `Other` answers in Zed.
- Keep the provider-neutral Helix question API and frontend unchanged.

## Validation

The feature was built into the inner desktop image and exercised against the
real providers through `http://localhost:8080`:

| Harness | Live result |
|---|---|
| Qwen Code | Two-question permission request (single + multi-select) answered; turn resumed and persisted the Q&A history. Cancellation also settled the agent turn and persisted `outcome: cancelled`. |
| GLM | Same Qwen ACP permission transport answered; turn resumed and persisted the Q&A history. |
| Claude Code | Form elicitation answered through `claude-agent-acp` 0.76; turn resumed and persisted the Q&A history. |
| Codex | `@agentclientprotocol/codex-acp` 1.11.0 on desktop image `d86af2` emitted a two-question Default-mode form elicitation. Selecting `Vue` and `On-prem` resumed the turn with `FRAMEWORK=Vue; TARGET=On-prem`. A second request answered through Codex's custom `Other` companion field with `SQLite`; the bridge exposed one question (not a duplicate helper question), resumed with `DATABASE=SQLite`, and persisted both resolved histories. Session: `ses_01m2dhtb34s52fmpw3cwek4sn5`. |

Automated validation includes the focused Go store/server tests, frontend
component tests, Rust normalization/serialization tests, production builds,
and the dockerized Zed external-sync smoke test. Destructive reconnect and the
180-second auto-wake soak remain manual resilience checks rather than release
gates; their state transitions are covered by focused tests.

## Risks / open questions

- **claude-agent-acp version float**: the question path needs the elicitation
  support (~0.76+). Version comes from Zed's cloud registry at runtime. Mitigate:
  verify the installed version at test time; if needed, pin the registry entry
  in our fork's registry data or vendor the wrapper like `qwen-code-build`.
- **Qwen `answers` extension**: the bundled Qwen 0.22.0 ACP path reads
  `output.answers` from the raw permission response. The bridge therefore uses
  the normal `session/request_permission` method with a locally extended
  response type and serializes `answers` at the response top level. Putting it
  under `_meta` does not satisfy the shipped client.
- **ACP version negotiation**: Zed's Rust crate is ACP 2.0.0-unstable; the npm
  wrappers speak protocolVersion 1 at initialize. Live Claude and Codex sessions
  confirmed that `elicitation.form` survives the negotiated connection, so keep
  those checks in the provider matrix when either wrapper or ACP is upgraded.
- **Codex Default-mode maturity**: OpenAI still marks
  `default_mode_request_user_input` under development, and the App Server marks
  Default-mode requests non-blocking. Keep the live timeout/resume behavior in
  the provider test matrix. Plan mode remains the stable blocking path.
- **Invalid Codex feature configuration**: settings-sync-daemon merge-preserves
  an existing `[features]` table, but fails closed if `features` is a scalar or
  array. In that case the Codex agent server is withheld instead of replacing
  malformed user configuration; its log names the setting that must be fixed.
- **Auto-wake vs legit pauses**: the 180s "agent went quiet = stuck" heuristic
  is now wrong in a new way; the pending-question check must cover both
  elicitation- and permission-sourced pauses.
