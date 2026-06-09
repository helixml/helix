# Implementation Tasks: Validate Fork-and-Pause Branch Behaviour

> **Predecessor:** [002081](../002081_kickoff-mid-session/) — feature implementation.
> **Scope:** prove the branch works against a running stack; close 002081's Phase 10 gap.

## Phase 1 — Preflight

- [x] Confirm the inner Helix stack is up: `curl -s http://localhost:8080/api/v1/config | jq` returns non-error JSON
- [x] Confirm the branch is checked out: `cd /home/retro/work/helix && git rev-parse --abbrev-ref HEAD` reports `feature/001806-high-leverage-for-us-to`
- [~] Confirm the in-process tests still pass on the branch: `cd /home/retro/work/helix && go test ./api/pkg/server/... -run 'Fork|Pause|Transcript' -count=1` is green (re-confirms the baseline before manual validation)

## Phase 2 — Backend smoke test script

- [ ] Create `validate_fork.sh` in this task directory (`/home/retro/work/helix-specs/design/tasks/002082_what-i-want-to-do-is/`) that:
  - [ ] Logs in to the inner Helix and grabs a bearer token
  - [ ] Creates (or reuses, via env var) a `zed_external` session and posts one user message
  - [ ] Waits for the first interaction to complete
  - [ ] Calls `POST /sessions/{id}/fork` with a different runtime
  - [ ] Asserts 200 + `new_session_id`
  - [ ] Asserts parent has `metadata.paused == true` and `metadata.paused_reason` matches `forked_to:<child>`
  - [ ] Asserts child has `metadata.parent_session_id == <parent>` and a `fork_seed` interaction with non-empty `response_message`
  - [ ] Posts a message to the paused parent and asserts 409 with `paused` in the body
  - [ ] Exits non-zero on any assertion failure (prints which one)
- [ ] Run `validate_fork.sh` end-to-end and capture the output

## Phase 3 — Manual UI walkthrough (M1–M9 from design.md)

- [ ] Open the inner Helix UI in Chrome via `chrome-devtools` MCP, log in as `test@helix.ml` / `helixtest`, switch to `testorg` / `testproj`
- [ ] Create a fresh spec task with a `zed_external` agent (Zed built-in is fine for the parent)
- [ ] M1: confirm the chat-panel agent dropdown is visible — screenshot to `screenshots/M1-dropdown-visible.png`
- [ ] M2: pick a different agent (e.g., Claude Code) — confirm network shows `POST /sessions/{id}/fork`, chat panel re-mounts on the child — screenshot `screenshots/M2-dropdown-firing.png`
- [ ] M3: send "remember: my favourite colour is octarine" on the child — confirm completion — screenshot `screenshots/M3-child-replied.png`
- [ ] M4: fork the child to yet another agent (e.g., Qwen Code) via the dropdown — screenshot `screenshots/M4-fork-chain-depth-2.png`
- [ ] M5: on the grandchild, ask "what's my favourite colour?" — confirm reply contains "octarine" — screenshot `screenshots/M5-recall-across-forks.png`
- [ ] M6: navigate back to the original parent — confirm `PausedBanner` renders with a "forked to" link that navigates — screenshot `screenshots/M6-paused-banner.png`
- [ ] M7: navigate to the child — confirm `ForkBadge` renders with a link back to the parent — screenshot `screenshots/M7-fork-badge.png`
- [ ] M8: confirm child timeline shows the `fork_seed` divider with caption + expandable disclosure — screenshot `screenshots/M8-fork-seed-divider.png`
- [ ] M9: confirm the paused parent's chat input is disabled with the "fork to continue" placeholder — screenshot `screenshots/M9-input-disabled.png`

## Phase 4 — Record outcome

- [ ] Fill in the "## Validation outcome" section in `design.md` with: which scenarios passed, which failed, links to any follow-up issues opened for failures
- [ ] For each failed scenario, either fix on this branch (small bugs) or open a tracked issue with reproducer steps (larger defects)
- [ ] Update 002081's `tasks.md` Phase 10 checkboxes to `[x]` for any scenarios proven here

## Phase 5 — Ready to merge

- [ ] Confirm all M1–M9 scenarios pass (or have tracked follow-ups) AND `validate_fork.sh` exits green
- [ ] Post a short outcome summary as a comment on the PR linking to this task directory
- [ ] Notify the user that the branch is validated and ready for merge review

---

## Out of scope (carried from design.md)

- Building the full docker-based E2E harness driving real LLMs (deferred Phase 9 item from 002081)
- Automated visual regression for the new components
- Performance / load testing
- Re-validating the (never-shipped) 001806 in-place mutation approach
