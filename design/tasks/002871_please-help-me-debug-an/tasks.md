# Implementation Tasks: Fix OpenCode Turns Ending Silently After Reasoning on Qwen Models

## Phase 0 — Settle the root cause (blocking; needs prod access)

- [ ] Run the `llm_calls` query from `design.md` §5 against the controlplane hosting meta.helix.ml for session `ses_01m07rcre65gy6qrd9s4ykxhpa`
- [ ] Record the `finish_reason`, `completion_tokens`, `max_tokens`, and `reasoning_effort` (asked vs sent) for turns 4 and 5 in `design.md`
- [ ] Confirm the exact model id and whether an explicit reasoning effort was selected (Open Questions 2 and 3)
- [ ] Decide from that evidence whether candidate A (truncation) or B (clean narrate-then-stop) is load-bearing, and note the decision in `design.md`

## Phase 1 — Make the failure visible (do regardless of Phase 0 outcome)

- [ ] Inspect `finish_reason` on the `/v1/chat/completions` completion path and log WARN on `length` with session id, model, `completion_tokens`, and request `max_tokens`
- [ ] Mark the interaction as truncated so the condition is visible in the UI, reusing the existing interaction-error/banner mechanism
- [ ] Log distinctly when a turn ends with reasoning content but no assistant text and no tool call
- [ ] Add unit tests for both signals (truncated completion, reasoning-only turn)
- [ ] Confirm no auto-retry is introduced (see Open Question 5)

## Phase 2 — Fix the opencode limit emission

- [ ] Add `omitempty` to both fields of `openCodeModelLimit` in `api/cmd/settings-sync-daemon/opencode.go`
- [ ] Split the `MaxTokens > 0 || MaxOutputTokens > 0` guard so `context` and `output` are each emitted only when known
- [ ] Add a regression test for the mixed case (`MaxTokens > 0`, `MaxOutputTokens == 0`) asserting `limit.output` is absent
- [ ] Extend `TestOpenCodeConfigOmitsUnknownLimits` coverage to the reverse mixed case (`MaxTokens == 0`, `MaxOutputTokens > 0`)
- [ ] State plainly in the commit message that this is a correctness fix and is not expected to resolve the stall on its own

## Phase 3 — Give self-hosted models a real output limit

- [ ] Choose between deriving `MaxOutputTokens` from the advertised context window in `applyAdvertisedModelLimits` or adding it to the curated model table (`design.md` §6.3)
- [ ] Implement the chosen option and verify a self-hosted Qwen model receives a non-zero `limit.output`
- [ ] Verify the effective cap fits a full `xhigh` Qwen reasoning pass plus an answer; if it does not, lower the default effort for this model on the opencode runtime
- [ ] Add tests covering a model absent from `model_info.json`

## Phase 4 — Verify end to end

- [ ] Rebuild the desktop image (`./stack build-ubuntu`) — the settings-sync-daemon does not hot reload
- [ ] Start a **new** session with the opencode runtime and a self-hosted Qwen model
- [ ] Dump the rendered `OPENCODE_CONFIG_CONTENT` from the container and confirm no `"output":0` and no `"context":0`
- [ ] Reproduce the original prompt from turn 4 and confirm the turn either completes or reports truncation explicitly
- [ ] Confirm the `llm_calls` row now carries a diagnosable `finish_reason` and that a `length` result produces the WARN log and the UI signal
- [ ] Run `cd api && go build ./...` and the affected package tests; check CI after pushing

## Phase 5 — Follow-ups (record, do not necessarily fix here)

- [ ] File a note that `injectAgentToolNudge` is applied only on `/v1/chat/completions`, not `/v1/responses` or `/v1/messages` — a latent gap for runtimes on those endpoints
- [ ] Grep the config builders for other `A > 0 || B > 0`-then-write-both guards
- [ ] Confirm whether Zed or opencode emits the `<thinking>` wrapper, since a renderer that always closes the tag masks truncation (Open Question 4)
