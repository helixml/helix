# Requirements: Finish Agent Questions (ACP Elicitations) — OpenAPI, Tests, E2E and Live Verification

## Background

The agent-questions feature (Claude Code's `AskUserQuestion` → ACP form elicitation →
rendered and answered in Helix) is **~70% built and already committed** on branch
`feature/002731-end-to-end-agent` in both repos:

- `helix` HEAD `0cc5290ce` (design doc) on `9ddbd5d74` "feat(api): record and answer agent
  questions from ACP elicitations" — 2492 insertions, 23 files.
- `zed` `8fbf40ad92` "feat(external_websocket_sync): mirror ACP elicitations to Helix and
  accept answers" — 629 insertions, 4 files. **Note: the local `zed` checkout is on `main`;
  the commit exists only as `origin/feature/002731-end-to-end-agent`.**

The approved design is `helix/design/2026-08-11-agent-questions-elicitation.md` plus
`helix-specs/design/tasks/002731_implement-the-feature/{requirements,design,tasks}.md`.
**None of that is to be re-implemented.** This task is the remaining 30%: making it build,
proving it with tests, and proving it live.

The Zed agent panel's own elicitation UI is known-broken and stays broken. Helix must work
without it. Do not fix it; do not rebase Zed onto upstream.

---

## User Stories

### 1. The frontend must build

**As a Helix developer**, I want the generated API client to contain the elicitation
endpoints, so that `yarn build` succeeds and the card can submit answers.

`frontend/src/services/elicitationService.ts` already imports
`TypesElicitationRespondResponse` and calls `client.v1SessionsElicitationsRespondCreate` /
`client.v1SessionsElicitationsDetail`, but `grep -c "v1SessionsElicitations"
frontend/src/api/api.ts` returns `0` — `./stack update_openapi` has never been run against
the swagger annotations added in `session_handlers.go:2435-2588`.

**Acceptance criteria**

- [ ] `./stack update_openapi` run; regenerated files (`api/pkg/server/swagger*`,
      `frontend/src/api/api.ts`, and any `docs/`-style artefacts the script touches)
      committed.
- [ ] `grep -c "v1SessionsElicitationsRespondCreate" frontend/src/api/api.ts` ≥ 1 and
      `TypesElicitationRespondResponse` is exported from `frontend/src/api/api.ts`.
- [ ] `cd frontend && yarn build` exits 0 with no TypeScript errors.
- [ ] No hand-editing of generated files.

### 2. Go handlers and the REST endpoint must be covered by tests

**As a maintainer**, I want unit tests for every elicitation path, so that a regression in
status reconciliation is caught before it silently kills a live question.

Tests go in a new `api/pkg/server/websocket_external_agent_elicitation_test.go`, following
the `suite.Suite` + gomock style of the existing 4285-line
`api/pkg/server/websocket_external_agent_sync_test.go` (gomock, **not** testify/mock — see
CLAUDE.md).

**Acceptance criteria** — a test exists and passes for each of:

- [ ] `handleElicitationRequested`: new question → row upsert + transcript entry + patch
      published + `agent_question` attention event emitted exactly once.
- [ ] `handleElicitationRequested` idempotency: a repeat of the same `elicitation_id`
      (heartbeat / reconnect re-announcement) does **not** re-emit the attention event.
- [ ] `handleElicitationResolved`: conditional transition to each terminal status
      (`accepted`/`declined`/`cancelled`/`completed`); entry mirrored; notification cleared.
- [ ] `handleElicitationResolved` with an unknown `elicitation_id`: dropped and logged, no
      error, no rows touched.
- [ ] `handleElicitationResync`: listed ids get `TouchAgentElicitations`; ids absent from
      the list are **not** reaped before the grace window, and **are** cancelled with reason
      `agent_no_longer_holds` after it.
- [ ] `handleElicitationResponseAck`: `noop` and `not_found` reconcile the row; `accepted`
      is a no-op locally.
- [ ] **Reconnect does not cancel**: `handleAgentReady` on a session with a pending
      elicitation leaves its status `pending` (the single most important rule in the
      design — an API restart leaves Zed's `respond_tx` alive).
- [ ] REST endpoint `POST /api/v1/sessions/{id}/elicitations/{id}/respond`:
      401/403 for a user not authorised to the session; 404 for an unknown
      `elicitation_id`; 403 (or 404, whichever the handler emits — assert the actual
      behaviour) when the elicitation belongs to a different session; 409 when the
      elicitation is already terminal; 409 when no agent is connected.
- [ ] **Two clients answering at once**: the first `pending→submitting` transition wins,
      the second gets 409, and `sendCommandToExternalAgent` is called exactly once.
- [ ] **Answer after cancel**: conditional update affects 0 rows → 409.
- [ ] Send-failure rollback: when `sendCommandToExternalAgent` fails after the claim, the
      row is rolled back to `pending`.
- [ ] **Empty / unmappable `request_id`** falls back to `handleMessageAdded`'s existing
      resolution (streaming context → newest waiting interaction for the thread).
- [ ] `TestAutoWake_SkipsInteractionBlockedOnUserQuestion` — `maybeAutoWake` skips an
      interaction with a live elicitation.
- [ ] `processPromptQueue` dispatches (does not defer) a non-interrupt follow-up when the
      newest interaction is `waiting` **and** blocked on a live elicitation.
- [ ] `CGO_ENABLED=1 go test ./pkg/server/ -count=1` passes (CGo needed for tree-sitter;
      `sudo apt-get install -y gcc libc6-dev` if missing).

### 3. The protocol must be covered end-to-end

**As a maintainer**, I want a dockerized e2e phase that drives an elicitation through the
real production Helix handlers against a real Zed binary.

**Acceptance criteria**

- [ ] A new phase in
      `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go` asserts:
      `elicitation_requested` (with a **non-empty** `requested_schema`) is recorded →
      `respond_elicitation` is issued → `elicitation_resolved(accepted)` is applied →
      `message_completed` arrives for the same turn.
- [ ] The phase is driven through the **synthetic** seam (`srv.ProcessSyncEvent`, as
      Phase 10 does for `user_created_thread`). A comment states *why*: a CI phase must not
      depend on the model choosing to call `AskUserQuestion`, which is non-deterministic and
      would make the suite flaky.
- [ ] The header phase-list comment block and any phase-count constants/timeouts are
      updated.
- [ ] `memorystore` implements the 8 `AgentElicitation` store methods (see design §Gap 3) —
      without this the e2e nil-panics.
- [ ] **`./run_docker_e2e.sh` has actually been run to green** and its output is quoted in
      the final report. Per `CLAUDE.md`: "IF YOU TOUCH THE E2E TESTS, YOU MUST RUN THEM. NO
      EXCEPTIONS." "It compiles" and "it follows the pattern" are not evidence.

### 4. CI must build the right Zed

**As CI**, I need `sandbox-versions.txt` to pin the Zed commit containing the elicitation
code.

**Acceptance criteria**

- [ ] `ZED_COMMIT` bumped from `eae9b051e17440aa8a9e6bb4035277dcc5d926d3` to the final zed
      commit on `feature/002731-end-to-end-agent`, taken from `git rev-parse HEAD` **before**
      pushing zed.

### 5. The feature must be proven live, with screenshots

**As the reviewer**, I want evidence from the inner Helix that a real user can see and
answer a real agent question — not unit tests.

Screenshots go to
`helix-specs/design/tasks/002750_finish-agent-questions/screenshots/NN-description.png`.

**Acceptance criteria** — each either demonstrated with a screenshot, or reported plainly as
not working:

- [ ] An agent in a live spec-task session calls `AskUserQuestion`.
- [ ] The question renders in the **Helix** UI with every option (label + description)
      visible — screenshot.
- [ ] Answering it in Helix resumes the turn, and the reply reflects the chosen answer —
      screenshot.
- [ ] **Next operation**: a normal follow-up message after answering is delivered and
      answered.
- [ ] A **second** question in the same session works independently.
- [ ] The **decline / Skip** path settles cleanly (turn continues with empty answers).
- [ ] A question left unanswered and then **interrupted from Helix** settles to a terminal
      status and stops being answerable (the card locks; a further POST returns 409).

---

## Out of Scope

- Fixing Zed's own agent-panel elicitation UI.
- Rebasing Zed onto upstream.
- Re-implementing any of the committed Rust/Go/React code.
- `api/pkg/org/infrastructure/runtime/helix/spawner.go`'s independent activation timeout
  (recorded as a known follow-up in the design doc).
- The Rust unit tests and `./script/clippy` run listed as unchecked in the 002731 tasks —
  see Open Questions.

---

## Open Questions

1. **Phase number collision.** The brief says "add Phase 17", but Phase 17 already exists on
   the branch (`main.go:1644`, "Queue interrupt (interrupt=true)"), and Phase 16 is the
   queue-defer test. The new elicitation phase is therefore specced as **Phase 18**.
   Confirm this rather than renumbering the existing phases.

2. **What the synthetic seam should assert about Zed.** Injecting `elicitation_requested`
   via `srv.ProcessSyncEvent` means the elicitation does not exist in the live Zed process.
   When Helix then sends `respond_elicitation`, real Zed will answer
   `elicitation_response_ack{not_found}`. Two options:
   (a) assert the command was queued/delivered, then inject `elicitation_resolved(accepted)`
   synthetically — tests the full Helix handler chain and the command egress, but not Zed's
   apply path; or
   (b) additionally assert the real `not_found` ack, proving round-trip transport.
   This spec assumes **(a) plus the ack observed and tolerated**. Confirm.

3. **Rust-side tests.** The 002731 task list has unchecked items for Rust unit tests
   (entry→event mapping, resync emission, no-op/not-found) and `cargo build --features
   external_websocket_sync -p zed` + `./script/clippy`. The brief for this task does not
   mention them. Assumed **in scope only as a clippy/build check**, not new Rust unit tests,
   because a full Rust build here is very heavy. Confirm if the Rust unit tests are wanted.

4. **Zed binary for the e2e.** `helix/zed-build/zed` does not exist in this sandbox, so
   `run_docker_e2e.sh` will require `./stack build-zed dev` first (documented as ~3 min,
   realistically much longer on this shared box). Assumed acceptable. If a prebuilt binary
   exists elsewhere, point at it.

5. **Live-verification items dropped from the 002731 list.** That list also required an
   API-restart test, the custom-answer ("Other" only) path, a task-list/header badge check,
   and bell/Slack notification checks. This task's brief lists a shorter set. Assumed the
   brief's list is the required bar and the extras are **best-effort**; they will be
   attempted and reported, not treated as blockers. Confirm.

6. **Which agent/model to use for live verification.** `AskUserQuestion` is a Claude Code
   built-in, so the session must run the `claude` harness (not `zed-agent`/qwen). Assumed
   the inner Helix has a Claude-backed agent available; if not, this becomes the blocker for
   requirement 5.
