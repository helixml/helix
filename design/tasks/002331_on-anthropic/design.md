# Design: Default to 1M-Context Opus 4.8 on Zed Subscription

## Summary

Make new agent threads on the `zed.dev` (Anthropic subscription) provider
default to the 1M-context Opus 4.8 model instead of the 200k one. The
subscription model list and its default are server-driven, so there are two
viable levers. The **primary, in-repo change** is to update the seeded
`default_model` in `assets/settings/default.json` to point at the 1M variant.

## How model selection works today (verified)

```
GET /models  ──►  ListModelsResponse { models, default_model,
                    default_fast_model, recommended_models }   (server-supplied)
        │
        ▼
CloudModelProvider.update_models()   language_models_cloud.rs:744-772
  - resolves default_model / default_fast_model / recommended_models by id
        │
        ▼
LanguageModelRegistry.default_model()   registry.rs:414-422
  1. settings `agent.default_model`  (if that id is available)
  2. else environment fallback:
       zed.dev cloud provider.default_model(), else recommended_models[0]
       (language_models.rs:152-186)
```

Key source locations:
- Settings seed: `assets/settings/default.json:1046-1053` (currently
  `provider: "zed.dev"`, `model: "claude-sonnet-4"`).
- Settings field type: `crates/agent_settings/src/agent_settings.rs:214`
  (`default_model: Option<LanguageModelSelection>`); provider enum renders as
  `"zed.dev"` (`crates/settings_content/src/language_model.rs:27,465`).
- Applied to registry: `crates/agent_ui/src/agent_ui.rs:876-912`
  (`update_active_language_model_from_settings` → `select_default_model`).
- Cloud provider default getters: `language_models_cloud.rs:795-808`.
- Wire contract: `crates/cloud_llm_client/src/cloud_llm_client.rs:288-333`.

**Why the user sees 200k today:** the seeded `claude-sonnet-4` is not offered on
our subscription, so it is dropped and the environment fallback uses the
server's `default_model` — the 200k Opus 4.8.

## Decision: change the client settings seed (primary)

Update `assets/settings/default.json` `default_model` to select the 1M Opus 4.8
entry by its id:

```json
"default_model": {
  "provider": "zed.dev",
  "model": "<1M-opus-4-8 model id>",   // e.g. the id whose display_name is "…[1m]"
  "enable_thinking": false
}
```

Rationale:
- It is the only default that lives in this repo and is under our direct control
  in the Zed client.
- It respects user overrides: a user who set their own `agent.default_model` is
  unaffected (US-1 AC).
- It requires no client code changes — pure config — if the 1M variant is a
  separate model id (per Open Question 1).

### Critical dependency
`select_default_model` only "sticks" if the seeded model id is present in the
live `/models` list. Therefore the exact id **must be verified against the
running subscription's `/models` response** before implementation, or the seed
will be silently ignored and the old fallback (200k) will persist. This is the
main risk and the reason for the verification task.

## Alternative: server-side `default_model` (note, likely out of this repo)

Because selection ultimately honors `ListModelsResponse.default_model`, setting
the Helix LLM backend to return the 1M variant's id as `default_model` (and,
optionally, `default_fast_model`) is the most robust, global fix — it changes
the default for all clients regardless of the client seed. This is **not** a
change in the `zed` repo and is listed here as a decision point (Open Question
3). If chosen, the client seed change may be unnecessary (or kept as a
belt-and-suspenders default).

## Edge case: if the 1M variant is "Max Mode", not a separate id

If Open Question 1 resolves to "same Opus 4.8 model, larger window via
`max_token_count_in_max_mode`" rather than a separate id, then a
`default_model` id swap cannot select it — the larger window is gated behind
Zed's Burn/Max Mode toggle, a per-thread/agent state, not a model id. In that
case this task changes scope to "default new threads to Max/Burn Mode for this
model," which is a different mechanism and would need its own design. Flagged as
a blocker in requirements Open Question 1.

## Testing
- Manual: fresh profile (or clear `agent.default_model`), sign in to the
  Anthropic subscription, open a new agent thread, confirm the selected model is
  the 1M Opus 4.8 and its context indicator shows ~1M tokens.
- Confirm the 200k Opus 4.8 is still present and selectable in the model menu.
- Confirm a user with an explicit `agent.default_model` set is not overridden.
- The existing cloud-provider tests around model resolution
  (`crates/language_models/src/provider/cloud.rs`) should still pass; add/adjust
  only if the chosen mechanism warrants it.
