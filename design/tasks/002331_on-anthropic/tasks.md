# Implementation Tasks: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Verify first
- [ ] Confirm the 200k and 1M Opus 4.8 are two distinct `/v1/models` entries with
      distinct ids and distinct `max_input_tokens` (resolves requirements Open
      Question 1); capture the fetched model list for the subscription (Open
      Question 3).
- [ ] Decide the scope of the larger-context preference: all prefix groups vs.
      Opus only (Open Question 2), and whether `default_fast_model` /
      `recommended_models` are included (Open Question 4).

## Primary implementation
- [ ] Update `pick_preferred_model` in
      `crates/language_models/src/provider/anthropic.rs` (~line 333) to select by
      largest `max_input_tokens` within a prefix group, tie-breaking by model id.
- [ ] If Open Question 2 says "Opus only", instead special-case the Opus group
      rather than changing the shared comparator.

## Tests
- [ ] Add a unit test for `pick_preferred_model`: two Opus 4.8 entries differing
      only by `max_input_tokens` (200_000 vs 1_000_000) → the 1M entry is chosen;
      equal windows fall back to the id tie-break deterministically.
- [ ] Ensure existing Anthropic provider tests still pass.

## Verification
- [ ] Build Zed; on a fresh profile, authenticate the Anthropic subscription,
      open a new agent thread, confirm the default is the 1M Opus 4.8 (~1M
      context).
- [ ] Confirm both the 200k and 1M variants remain listed/selectable.
- [ ] Confirm an existing user override of `agent.default_model` is not changed.
