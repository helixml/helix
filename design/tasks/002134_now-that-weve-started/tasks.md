# Implementation Tasks: Configurable Model Selection for Claude Subscription Mode

- [ ] In `crates/language_models_cloud/src/language_models_cloud.rs`, update the cloud default model fallback to prefer the highest-versioned `claude-opus-` model from the returned model list when `ListModelsResponse.default_model` is absent or resolves to a Sonnet variant
- [ ] In `crates/agent_ui/src/agent_configuration.rs`, add a model picker inside the `is_zed_provider` block (around line 221) that shows the three tier options (Opus, Sonnet, Haiku) filtered from the live cloud model list
- [ ] Filter logic: for each tier prefix (`claude-opus-`, `claude-sonnet-`, `claude-haiku-`), pick the lexicographically greatest model ID from the cloud model list — reuse the same strategy as `pick_preferred_model()` in `anthropic.rs`
- [ ] Wire the picker's selection to `agent.default_model` via the same persistence path used by API key mode
- [ ] Verify: opening the agent panel in subscription mode shows the new picker, defaulting to Opus on first launch
- [ ] Verify: saved preference survives restart and is not overridden by the cloud API default
- [ ] Verify: API key mode users see no change to their existing full model picker
