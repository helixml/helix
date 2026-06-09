# Design: Validate Fork-and-Pause Branch Behaviour

> **Sibling docs:** see [requirements.md](requirements.md) and [tasks.md](tasks.md).
> **Predecessor:** task [002081](../002081_kickoff-mid-session/) — implementation. Phases 0–8 landed on `feature/001806-high-leverage-for-us-to`; this task closes the validation gap (its Phase 10 and the deferred Phase 9 docker E2E item).

## What we are validating

The branch's 7 commits add three observable behaviours:

1. **Fork creates a child session and pauses the parent.** Backend handler + helper.
2. **The child's first message reaches the agent with the parent transcript prepended.** Websocket-layer injection.
3. **The UI exposes the dropdown→navigate→banner→badge→divider chain.** Frontend components.

In-process Go tests (29 unit + 7 HTTP integration on commit `562b1f38f`) already prove (1) and parts of (2) against the in-memory store. They do **not** prove:

- The chain works end-to-end on a real process (Postgres-backed store, real websocket, real container provisioning).
- The frontend wires up correctly (the dropdown actually fires the API call, the navigation actually happens, the banner actually renders).
- The new agent on the child can actually use the seeded transcript.

This task fills those gaps.

## Strategy: three layers, lightest first

### Layer 1 — Backend smoke test against the live stack

A short shell script (`validate_fork.sh`) using `curl` + `jq` that:

1. Logs in to the inner Helix as `test@helix.ml` and grabs a bearer token.
2. Creates a fresh `zed_external` session via the existing session-create path (or uses a known existing session id supplied via env var).
3. Posts one user message and waits for the interaction to complete (poll `GET /sessions/{id}` until `interactions[-1].state == "complete"`).
4. Hits `POST /sessions/{id}/fork` with `{ helix_app_id: <other_app> }`.
5. Asserts the JSON response has `new_session_id` and HTTP 200.
6. Re-reads the parent — asserts `metadata.paused == true` and `metadata.paused_reason` matches `forked_to:<child_id>`.
7. Reads the child — asserts `metadata.parent_session_id == <parent>` and one of its interactions has `trigger == "fork_seed"` with non-empty `response_message`.
8. Tries `POST /sessions/{parent_id}/messages` — asserts HTTP 409 and the body contains `paused`.

Why a shell script and not Go: the goal is "shake hands with the running process", not "build an integration-test framework". A 60-line shell script is enough; if it grows we promote it.

The script lives at `/home/retro/work/helix-specs/design/tasks/002082_what-i-want-to-do-is/validate_fork.sh` so it travels with this spec and the next agent can re-run it without code-side artefacts. It can be re-purposed for any future fork-related work.

### Layer 2 — Manual UI walkthrough

The Phase 10 checklist from 002081 (lines 110–117 of its `tasks.md`) is the source of truth for the manual flow. We repeat it here as a numbered scenario list, with one important addition: capture a screenshot for each UI surface so the PR has visual evidence rather than just "I clicked through it".

Walkthrough scenarios:

| # | Action | Pass criteria |
|---|---|---|
| M1 | Open the chat panel on a running spec task with one agent | Agent dropdown is visible in the panel header |
| M2 | Pick a different agent from the dropdown | Network shows `POST /sessions/{id}/fork`; chat panel re-mounts on the returned child id; URL still points at the spec task |
| M3 | Send a message in the child saying "remember: my favourite colour is octarine" | Agent acknowledges; interaction completes |
| M4 | Pick another agent from the dropdown on the child | Fork chain depth 2; child-of-child gets the seeded transcript via injection |
| M5 | Ask "what's my favourite colour?" on the grandchild | Agent replies "octarine" (proves recall across two forks) |
| M6 | Navigate back to the original parent | Paused banner appears; clicking the "forked to" link goes to the child |
| M7 | Navigate to the child | Fork badge appears; clicking it goes back to the parent |
| M8 | Look at the child's timeline | A divider appears at the top with "Forked from … with N interactions of context" and an expandable disclosure that reveals the raw transcript |
| M9 | Try to type in the paused parent's chat input | Input is disabled with the paused-state placeholder |

Screenshots are saved to `screenshots/` in this task directory (paths like `screenshots/M2-fork-dropdown-firing.png`). Use the `helix-desktop` MCP `save_screenshot` tool per the kit instructions in the harness header.

### Layer 3 — Docker E2E (deferred, scoped here)

The full `e2e-test/` harness that drives a real Claude through fork A→B→C and LLM-asserts cross-agent recall is **deferred** (matches 002081's stance). What we DO write in this task is a one-page note (in `design.md` follow-ups, below) describing what such a test would look like, so the next person doesn't have to re-derive the scope.

The minimum bar we *can* hit without that harness is the manual M3+M5 colour-recall scenario above, which proves the cross-agent recall claim with a human in the loop instead of an LLM judge.

## What "validated" means at the end of this task

The branch is validated for merge when:

- `validate_fork.sh` runs green against `http://localhost:8080` on the dev stack.
- All M1–M9 scenarios above pass with screenshots captured.
- Any failure produces either a fix on this branch or a tracked follow-up issue.
- A short outcome summary is appended either to this task's `design.md` (under "## Validation outcome") or to the PR description.

## Key decisions

- **Live stack, not staging.** The inner Helix on `localhost:8080` is what's running in the harness; no need to deploy to verify Go-side behaviour.
- **`curl` + `jq` over Go test framework.** Smaller surface; the goal is to confirm wiring, not exhaustively cover edge cases (Go tests do that).
- **Human-judged recall over LLM-judged.** A reviewer eyeballing "octarine" appearing in the child's response is sufficient evidence for v1; the full LLM-as-judge loop adds infra for marginal value.
- **Screenshots in the spec task, not the PR description.** Spec tasks travel with the project's design history; PR descriptions are archived once merged.

## Codebase patterns / gotchas to mind during validation

- The chat-panel `ForkAgentControl` (`frontend/src/components/session/ForkAgentControl.tsx:1`) is *new* in this branch — distinct from the existing settings-sidebar `AgentDropdown` used elsewhere. When walking the manual flow, make sure the dropdown being clicked is the one in the chat panel header, not a settings page.
- The fork endpoint validates that source and target runtimes differ — picking the same agent will 400 with "already using". When choosing the M2 dropdown change, pick a runtime visibly different from the current one.
- The `fork_seed` interaction is the UI-visible record; the actual injection to the agent happens via `maybePrependTranscript` on the websocket layer. If M5 fails (no recall), check Helix API logs for the `transcript_len > 0` line on the first child message before assuming a UI bug.
- Pause enforcement was intentionally NOT wired into `pickupWaitingInteraction` (commit `be5d51313` rationale). If during M9 you somehow see an in-flight interaction on the paused parent continue to complete, that is **expected** — pause is "no new input", not "kill the agent".
- The 002081 design notes Postgres JSONB serialization should round-trip the new fields with no migration. If the validation script sees `metadata.paused` missing on the response, that's a bug worth chasing (the field has `omitempty` so missing == false, which is correct).

## Implementation pivot (discovered during execution)

The original plan had the smoke script create everything from scratch (org → project → app → session → message → fork). In practice this requires re-implementing the entire onboarding pipeline against the API, which is far more code than the validation needs.

**Pivoted approach:** the chrome-devtools UI walkthrough creates the orgs/projects/sessions naturally as a side effect of testing the UX. After that pass, the smoke script `validate_fork.sh` only needs the JWT token + an already-created `zed_external` session ID as env inputs, and validates just the fork endpoint contract (200 + new_session_id, parent paused, child has fork_seed, 409 on send-to-paused). This keeps the script under 100 lines and avoids duplicating onboarding logic.

Discovered details:
- The inner Helix at `localhost:8080` starts empty. First step is always `POST /api/v1/auth/register` with `{email, password, password_confirm, full_name}` — the response returns a JWT in `token`. The first registered user is auto-admin.
- The JWT is the bearer token for all `authRouter` endpoints (including `/sessions/{id}/fork`).
- The 002081 design and tasks docs explicitly state "register `test@helix.ml` / `helixtest`" — using those credentials for idempotency between agent runs.

## Validation outcome

(Filled in at the end of the task — once `validate_fork.sh` and the M1–M9 walkthrough are complete, append a short report here noting any failures or follow-up issues.)

## Future work (out of scope for v1)

- Full docker-based E2E harness driving real LLMs through a fork chain with LLM-judged recall assertions. Scope sketch: extend `zed-repo/crates/external_websocket_sync/e2e-test/` with a new `phase_13_fork_recall.go` and `phase_14_fork_chain_depth_2.go`; reuse existing Anthropic + Zed test fixtures; gate on `RUN_EXPENSIVE_E2E=1` to keep CI cheap.
- Automated visual regression for the new components (PausedBanner / ForkBadge / ForkSeedDivider) — useful long-term, overkill for v1 validation.
- Validate fork-from-paused rejection in the UI (covered by HTTP test; the UI surface for forking from paused is moot because the dropdown is disabled on paused sessions — but confirm that's actually how it renders).
