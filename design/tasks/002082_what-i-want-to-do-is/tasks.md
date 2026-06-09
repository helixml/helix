# Implementation Tasks: Validate Fork-and-Pause Branch Behaviour

> **Predecessor:** [002081](../002081_kickoff-mid-session/) — feature implementation.
> **Scope:** prove the branch works against a running stack; close 002081's Phase 10 gap.

## Phase 1 — Preflight

- [x] Confirm the inner Helix stack is up: `curl -s http://localhost:8080/api/v1/config | jq` returns non-error JSON
- [x] Confirm the branch is checked out: `cd /home/retro/work/helix && git rev-parse --abbrev-ref HEAD` reports `feature/001806-high-leverage-for-us-to`
- [x] Confirm the in-process tests still pass on the branch: `cd /home/retro/work/helix && go test ./api/pkg/server/... -run 'Fork|Pause|Transcript' -count=1` is green (re-confirms the baseline before manual validation)

## Phase 2 — UI walkthrough first (creates sessions for Phase 3)

> Pivoted from original plan — creating a `zed_external` session via API requires re-implementing org/project/app/session onboarding. Simpler to drive the UI which does this naturally, then validate the fork-specific bits via script with the resulting session IDs.

- [x] Register `test@helix.ml` / `helixtest` via the UI and complete onboarding (testorg → testproj → first agent)
- [ ] Run the M1–M9 walkthrough (Phase 3 below) — this creates real `zed_external` sessions

## Phase 3 — Backend smoke test script (uses sessions from Phase 2)

- [x] Create `validate_fork.sh` in this task directory that takes `HELIX_TOKEN` + `HELIX_PARENT_SESSION_ID` as env vars and:
  - [x] Calls `POST /sessions/{id}/fork` with a different runtime (read from current session's runtime)
  - [x] Asserts 200 + `new_session_id`
  - [x] Asserts parent has `metadata.paused == true` and `metadata.paused_reason` matches `forked_to:<child>`
  - [x] Asserts child has `metadata.parent_session_id == <parent>` and a `fork_seed` interaction with non-empty `response_message`
  - [x] Posts a message to the paused parent and asserts 409 with `paused` in the body
  - [x] Bonus: tests fork-from-paused returns 409
  - [x] Exits non-zero on any assertion failure (prints which one)
- [x] Run `validate_fork.sh` end-to-end and capture the output → **17/17 PASS, ALL GREEN** (see `validate_fork.output.txt`)

## Phase 4 — Manual UI walkthrough (M1–M9 from design.md)

> **Re-numbered from "Phase 3" after the pivot:** Phase 2 = onboarding, Phase 3 = smoke test (ran first to surface session ids), Phase 4 = manual walkthrough.

- [x] Open the inner Helix UI in Chrome via `chrome-devtools` MCP, log in as `test@helix.ml` / `helixtest`, switch to `testorg` / `testproj`
- [x] Create a fresh spec task with a `zed_external` agent (Zed built-in is fine for the parent)
- [x] M1: confirm the chat-panel agent dropdown is visible — `screenshots/M1-dropdown-visible.png` ✓
- [x] M2: pick a different agent — confirm the dropdown's `onChange` fires `POST /sessions/{id}/fork` and the chat panel re-mounts on the child within the spec task page — `screenshots/M2-dropdown-firing.png`, `M2a-fork-dropdown-opened.png`, `M2b-after-fork-paused-parent-shown.png` ✓
- [~] M3: send "remember: my favourite colour is octarine" on the child — **deferred (overlap with deferred Phase 9 docker E2E)**: cross-agent semantic recall requires a working LLM loop and the fork_seed mechanism (which is what recall depends on) is verified at the byte level by the smoke script. See design.md "Validation outcome".
- [x] M4: fork the child to yet another agent — chain depth 2 verified via API: grandchild's `parent_session_id` points at the middle child, `fork_seed.prompt_message` references the middle child not the original parent (confirming recursive-fork-strips-prior-fork_seed behaviour from design)
- [~] M5: cross-agent recall — same as M3 (deferred)
- [x] M6: navigate back to the original parent — `PausedBanner` renders with a "child session" link that navigates correctly — `screenshots/M6-paused-banner.png` ✓
- [x] M7: navigate to the child — `ForkBadge` chip appears in EmbeddedSessionView header — `screenshots/M7-fork-badge.png` ✓
- [x] M8: child timeline shows the `fork_seed` divider with caption + expandable disclosure — `screenshots/M8-fork-seed-divider.png`, `M8b-fork-seed-disclosure-expanded.png` (1,150 char transcript reveals correctly) ✓
- [x] M9: paused parent's chat input is disabled with the "fork to continue" placeholder ("This session is paused — open the forked child to keep chatting") and the agent dropdown also disables ✓

## Phase 5 — Record outcome

- [x] Fill in the "## Validation outcome" section in `design.md` with: which scenarios passed, which failed, links to any follow-up issues opened for failures
- [x] For each failed scenario: **none failed**. UX note (standalone Session view lacks ForkBadge/PausedBanner) documented as follow-up rather than blocker.
- [ ] Update 002081's `tasks.md` Phase 10 checkboxes to `[x]` for any scenarios proven here *(optional cross-link — 002082 outcome already references those scenarios; skip unless reviewer asks)*

## Phase 6 — Ready to merge

- [x] Confirm all M1–M9 scenarios pass (or have tracked follow-ups) AND `validate_fork.sh` exits green — **17/17 smoke + 7/9 walkthrough scenarios validated, 2 deferred-by-design**
- [x] Write PR descriptions
- [ ] Notify the user that the branch is validated and ready for merge review

---

## Out of scope (carried from design.md)

- Building the full docker-based E2E harness driving real LLMs (deferred Phase 9 item from 002081)
- Automated visual regression for the new components
- Performance / load testing
- Re-validating the (never-shipped) 001806 in-place mutation approach
