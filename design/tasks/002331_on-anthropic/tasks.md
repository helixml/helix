# Implementation Tasks: Default to 1M-Context Opus 4.8 on Zed Subscription

## Verify first (blocks implementation)
- [ ] Confirm with the team whether the 1M Opus 4.8 is a **separate model id**
      or the same model exposed via Burn/Max Mode (`max_token_count_in_max_mode`)
      — resolves requirements Open Question 1.
- [ ] Capture the live `/models` response from the Anthropic subscription and
      record the exact `id`, `provider`, and `display_name` of the 1M Opus 4.8
      entry (and its current `default_model` id) — resolves Open Question 2.
- [ ] Decide fix location: client settings seed (this repo) vs. server
      `default_model` (Helix LLM backend) — resolves Open Question 3.

## Primary implementation (client settings seed)
- [ ] Update `assets/settings/default.json` `default_model` (lines ~1046-1053)
      to `{ "provider": "zed.dev", "model": "<verified 1M opus id>" }`.
- [ ] Decide and set `enable_thinking` appropriately for the 1M variant.
- [ ] (Optional, per Open Question 4) Update `default_fast_model` / subagent /
      inline-assistant defaults if they should also use the 1M variant.
- [ ] Update any settings docs that reference the default model
      (e.g. `docs/src/ai/agent-settings.md`) if they cite the old default.

## Verification
- [ ] Build Zed and, on a fresh profile, sign in to the Anthropic subscription;
      confirm a new agent thread defaults to the 1M Opus 4.8 with a ~1M context.
- [ ] Confirm the 200k Opus 4.8 is still listed and selectable.
- [ ] Confirm an existing user override of `agent.default_model` is not changed.
- [ ] Run cloud provider model-resolution tests
      (`crates/language_models/src/provider/cloud.rs`) and fix/extend if needed.

## If Max Mode (contingency)
- [ ] If the 1M variant is Max Mode (not a separate id), stop and re-scope: draft
      a follow-up design for defaulting new threads to Burn/Max Mode for this
      model instead of a `default_model` id swap.
