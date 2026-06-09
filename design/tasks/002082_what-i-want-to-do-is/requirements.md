# Requirements: Validate Fork-and-Pause Branch Behaviour

> **Context:** The branch `feature/001806-high-leverage-for-us-to` (commits `d2612ed3a` through `562b1f38f`) implements mid-session agent switching via fork-and-pause (designed in task [002081](../002081_kickoff-mid-session/)). Unit + HTTP integration tests pass in-process, but two validation gaps remain explicitly open in that task:
> - **Phase 10 (manual UI verification)** — not done; needs a human against a running inner Helix.
> - **Phase 9 deferred item (docker-based E2E with a real agent)** — not done; covers the cross-agent recall claim that in-process tests cannot prove.
>
> **This task** is the validation effort: prove the branch's claims hold against a running stack before it merges.

## User Stories

### 1. Confirm the fork endpoint works against a live stack
**As a** reviewer of this branch,
**I want** to exercise `POST /sessions/{id}/fork` against a running Helix API,
**So that** I have evidence the handler chain works in a real process — not just against the in-memory store the Go unit tests use.

**Acceptance Criteria:**
- A scripted smoke test (curl or short Go script) hits the live `/api/v1/sessions/{id}/fork` endpoint on a Helix dev stack and asserts:
  - 200 + `new_session_id` returned for a valid fork
  - parent's `Metadata.Paused == true` after the call (via `GET /sessions/{id}`)
  - parent's `Metadata.PausedReason == "forked_to:<child_id>"`
  - child has a `fork_seed` interaction with non-empty `ResponseMessage`
  - child's `Metadata.ParentSessionID == <parent_id>`
- Same script confirms 409 on `POST /sessions/{paused_id}/messages` with the paused reason in the body.

### 2. Confirm the fork UX works end-to-end in the browser
**As a** human reviewer,
**I want** to walk the fork flow through the actual chat panel in a browser,
**So that** I confirm the dropdown→navigate→banner→badge→divider chain renders and links wire up.

**Acceptance Criteria:**
- The chat-panel agent dropdown lists more than one available agent and triggering a different one fires `POST /sessions/{id}/fork`.
- On success, the chat panel re-mounts on the child session id without leaving the spec-task page.
- The parent session, when revisited, shows the `PausedBanner` with a "forked to" link that navigates to the child.
- The child session shows the `ForkBadge` with a link back to the parent.
- The child's timeline shows the `fork_seed` divider with the parent-id caption and an expandable disclosure containing the raw transcript.
- The chat input on the paused parent is disabled with the "fork to continue" placeholder.
- Submitting a message via the API directly to the paused parent returns the 409 error surfaced in the UI (snackbar or similar).
- Screenshots of each surface (dropdown, banner, badge, divider, disabled input) are captured for the PR.

### 3. Confirm the cross-agent recall claim
**As a** reviewer,
**I want** evidence that the new agent on the forked session actually receives the parent transcript and can act on it,
**So that** the "no context loss across fork" claim is grounded in observed behaviour, not just "the seed string was non-empty".

**Acceptance Criteria:**
- A forked session's first user message arrives at the agent with the parent transcript prepended (verified via API logs showing `transcript_len > 0`).
- A simple semantic check: in the manual walkthrough, the parent agent is told a single distinctive fact ("my favourite colour is octarine"); after forking to a different agent and asking "what's my favourite colour?", the new agent answers correctly.
- The recall test is run for at least one cross-agent pair (e.g., zed_agent → claude-code).

### 4. Document gaps as follow-up
**As a** team planning the merge,
**I want** any defects discovered during validation captured as concrete follow-up items,
**So that** known issues are tracked rather than lost.

**Acceptance Criteria:**
- Each failed scenario produces either a fix on this branch or a documented follow-up issue with reproducer steps.
- The validation outcome is summarised in a short report appended to the PR description (or this task's design.md) so reviewers don't have to re-walk the matrix.

## Out of Scope
- Re-running the in-process unit + HTTP integration tests on the branch (already green per commit `562b1f38f`; CI re-runs them on push).
- Writing the full docker-based E2E harness in `zed-repo/crates/external_websocket_sync/e2e-test/` — that's the deferred Phase 9 item and a project of its own; this task scopes "validation by humans + scripts", not "automate the full LLM loop".
- Validating any of the v1-out-of-scope items (manual /pause endpoint, /duplicate, container reaping, shared containers, fork-tree view).
- Performance / load testing of fork (single-user correctness is the v1 bar).
- Re-validating the 001806 in-place mutation work (superseded; never shipped).
