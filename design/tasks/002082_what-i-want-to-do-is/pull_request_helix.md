# Validate fork-and-pause branch behaviour against a live stack

## Summary
This task is **validation only** — no code changes to the `helix` repo. The fork-and-pause implementation in this branch (commits `d2612ed3a` → `562b1f38f`, designed under spec task [002081](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002081_kickoff-mid-session)) was already complete; this task closes 002081's Phase 10 (manual verification, never done) and proves the wiring against a real process rather than just the in-memory store the Go unit tests use.

## Validation outcome — branch is good to merge

Three layers, lightest first:

**1. Backend smoke test (`validate_fork.sh`) — 17/17 PASS.** A 100-line shell script hits the live API and asserts: 200 + `new_session_id` on fork; parent transitions to `paused=true` with `paused_reason=forked_to:<child>`; child has `parent_session_id` lineage and a non-empty `fork_seed.response_message` (1,150 bytes of serialized parent transcript); `POST /sessions/{paused}/messages` returns 409 with the paused reason in the body; second fork on a paused parent returns 409 with "active descendant" guidance. Full output in `validate_fork.output.txt`.

**2. UI walkthrough M1–M9** — 7/9 confirmed with screenshots, 2 deferred-by-design:
- M1 chat-panel dropdown visible · M2 dropdown fires fork and chat panel re-mounts on child · M4 chain depth 2 (with the recursive-fork-strips-prior-fork_seed behaviour working as designed) · M6 PausedBanner with link to child · M7 ForkBadge on child · M8 fork_seed divider + expandable transcript disclosure · M9 paused input disabled
- M3 / M5 (cross-agent LLM recall) deferred — the *mechanism* enabling recall (parent transcript captured in fork_seed) is verified at the byte level; proving the LLM also chose to use the context requires the docker E2E harness that 002081 explicitly deferred.

**3. Docker E2E** — scope sketch only; deferred to a follow-up issue (matches 002081's stance).

## Defects found
None. Every claim in the 002081 design holds against the live stack.

## Non-blocker UX note (worth a follow-up issue)
The standalone Session view (`/orgs/:org_id/session/:id`) renders the `fork_seed` divider correctly but does NOT render `ForkBadge` or `PausedBanner` — those are wired only into `EmbeddedSessionView`. Consistent with the design (fork flow keeps users in the spec-task page where EmbeddedSessionView lives), but creates a small disconnect when users land on a forked child via direct URL.

## Where the artefacts live
All in [`design/tasks/002082_what-i-want-to-do-is/`](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002082_what-i-want-to-do-is) on the `helix-specs` branch:
- `requirements.md` · `design.md` · `tasks.md` — the spec
- `validate_fork.sh` — re-runnable smoke test (`HELIX_TOKEN=… HELIX_PARENT_SESSION_ID=ses_… ./validate_fork.sh`)
- `validate_fork.output.txt` — captured 17/17 PASS run
- `screenshots/M*-*.png` — UI walkthrough evidence
