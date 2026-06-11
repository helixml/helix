# Implementation Tasks: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Phase 1 — Threshold bump (quick mitigation, ships first)

- [ ] Change `defaultAutoWakeStuckThreshold` in `api/pkg/server/auto_wake_stuck_interactions.go` from `60 * time.Second` to `180 * time.Second`.
- [ ] Rewrite the comment block above the constant (lines 190-209) to explain the new value: 180s covers realistic synchronous tool-call durations (`git push`, `npm install`, `gh pr view`, `find /`), with the decoy-interaction fix (Phase 2) as the real protection and this threshold as defence-in-depth.
- [ ] Update the comment at `auto_wake_stuck_interactions.go:396-415` to correct the empirical claim: tool-call **cascades** touch `lastPublish` on every event, but a **single long-running** tool produces no events during execution, so the gate can decay during it. Document this limitation honestly.
- [ ] Update any existing test in `auto_wake_stuck_interactions_test.go` that depends on the literal 60s value. Verify `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` override still works (add a test case if not covered).
- [ ] Run `go build ./api/pkg/server/...` to confirm compile.
- [ ] Commit with conventional format: `fix(api): raise auto-wake stuck threshold to 180s` referencing this spec task.

## Phase 2 — Hold activation lane until session-terminal (root-cause fix)

- [ ] In `api/pkg/org/infrastructure/runtime/helix/spawner.go`, add a `LivenessProbe func(ctx context.Context, sessionID string) bool` field to `SpawnerConfig`. Document that nil means "no extension; behave as today".
- [ ] Modify `pollUntilDone` (or extract a new wrapper around it) so that on `context.DeadlineExceeded`, IF `LivenessProbe != nil` AND `LivenessProbe(parentCtx, sessionID) == true`, it renews its context for another `ActivationTimeout` and continues polling instead of returning. Loop until terminal status or liveness fails.
- [ ] Plumb the parent context through so the deadline extension is genuine — the current `actCtx, cancel := context.WithTimeout(ctx, cfg.ActivationTimeout)` at spawner.go:219 needs to keep `ctx` available for the extension.
- [ ] In `api/pkg/server/helix_org.go` (or wherever `SpawnerConfig` is constructed for the API server), wire a `LivenessProbe` implementation that returns `true` when both:
  - `externalAgentWSManager.getConnection(sessionID).connected == true`, AND
  - the session's most recent `streamingContexts[sessionID].lastPublish` OR `session.Updated` is within the last `ActivationTimeout`.
- [ ] Confirm the change is no-op for all existing test wirings (they construct `SpawnerConfig` without `LivenessProbe`).
- [ ] Add a unit test on `Spawner` covering: (a) liveness probe returns true → spawner does NOT return on deadline; (b) liveness probe returns false → spawner returns as today; (c) no liveness probe wired → spawner returns as today.
- [ ] Run `go build ./api/pkg/org/... ./api/pkg/server/...`.
- [ ] Commit: `fix(org): hold activation lane until session-terminal not actCtx-terminal`.

## Phase 3 — End-to-end verification in inner Helix

- [ ] Register `test@helix.ml` / `helixtest` in the inner Helix at `http://localhost:8080`. Complete onboarding (testorg → testproj).
- [ ] Create a spec task and ask the agent to run a deliberately slow command (e.g. `sleep 120 && echo done`) inside a tool call. With the fix, the session should complete without a "↻ Retried" or "Incomplete interaction" banner appearing on a decoy interaction.
- [ ] Repeat without Phase 2 applied (revert locally, re-run) to confirm the failure reproduces pre-change. Re-apply.
- [ ] Verify the existing wake-up still fires when genuinely needed: simulate a stuck interaction by manually inserting a `state=waiting` row with empty response and aged-out `created` (or wait for one to naturally occur in a sandbox session). Confirm wake-up runs on the new 180s schedule.
- [ ] Check DB for absence of decoy rows after a long session:
  `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, session_id, state, response_message, auto_wake_count FROM interactions WHERE session_id = '<session>' ORDER BY created;"`
- [ ] Document the test results inline in `design/2026-06-11-auto-wake-tool-call-fix.md` in the Helix repo (per CLAUDE.md convention).

## Phase 4 — CI and ship

- [ ] Push branch, open Helix PR titled `fix(api): stop auto-wake cutting short long tool-call sessions`. PR body links to this spec task and summarises both phases.
- [ ] Watch CI via `gh pr checks <num>` or the Drone MCP tools. Fix any failures before requesting review.
- [ ] On merge, post in the relevant ops channel that operators can drop any custom `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600+` overrides — 180s is now the safe default.
