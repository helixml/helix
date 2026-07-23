# Design: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Summary

Make Zed's **direct Anthropic provider** default to the 1M-context Opus 4.8
instead of the 200k one. The provider already fetches both entries from
Anthropic's `/v1/models`; the problem is that the default-selection helper picks
by lexicographic model id and ignores the context window. The fix is to make the
preference **context-window-aware** so the larger-window variant wins — without
hardcoding any model id.

## How the default is chosen today (verified)

`crates/language_models/src/provider/anthropic.rs`:

```rust
fn default_model(&self, cx: &App) -> Option<Arc<dyn LanguageModel>> {
    let fetched = self.state.read(cx).fetched_models.clone();
    pick_preferred_model(&fetched, &["claude-sonnet-", "claude-opus-", "claude-"])
        .map(|model| self.create_language_model(model))
}

fn pick_preferred_model(models, preferred_prefixes) -> Option<Model> {
    for prefix in preferred_prefixes {
        let candidate = models.iter()
            .filter(|m| m.id.starts_with(prefix))
            .max_by(|a, b| a.id.cmp(&b.id));   // <-- id-only comparison
        if let Some(model) = candidate { return Some(model.clone()); }
    }
    None
}
```

- `default_model()` — line ~224; `default_fast_model()` — ~233;
  `recommended_models()` — ~239; all call `pick_preferred_model`.
- `pick_preferred_model` — line ~333.
- Model list source: `provided_models()` (~246) merges Anthropic `/v1/models`
  results (`self.state...fetched_models`) with user `available_models`; context
  window comes from `max_input_tokens`
  (`crates/anthropic/src/anthropic.rs` `Model::from_listed`,
  `provider/anthropic.rs::max_token_count` ~536).

Both Opus 4.8 entries live in `fetched_models` with distinct ids and distinct
`max_input_tokens`; `max_by(id)` currently lands on the 200k id.

## Decision: make `pick_preferred_model` prefer the larger context window

Change the comparator so that, within a prefix group, the candidate with the
largest `max_input_tokens` wins, with the model id as a deterministic tie-break:

```rust
let candidate = models.iter()
    .filter(|m| m.id.starts_with(prefix))
    .max_by(|a, b| a.max_input_tokens
        .cmp(&b.max_input_tokens)
        .then_with(|| a.id.cmp(&b.id)));
```

Rationale:
- **Directly fixes the reported behavior**: among the two Opus 4.8 entries, the
  1M variant now wins.
- **Id-agnostic / future-proof** (US-2): no hardcoded `claude-opus-4-8-…` id; it
  keeps preferring the larger window even if ids change.
- **Minimal, localized** change in one helper; the prefix ordering
  (Sonnet → Opus → any Claude) is unchanged.
- Deterministic on ties via the id comparison.

### Scope note (Open Question 2)
Because `default_model()`, `default_fast_model()`, and `recommended_models()`
share `pick_preferred_model`, the larger-context preference applies to all of
them. If we want it to affect Opus only, we would instead special-case the Opus
group rather than change the shared comparator — pending the answer to Open
Question 2. Default recommendation: apply it to the shared helper (simplest,
consistent).

## Alternative considered: settings seed

Setting `assets/settings/default.json` `agent.default_model` to
`{ "provider": "anthropic", "model": "<1M opus id>" }` would also work, but:
- requires knowing and pinning the exact 1M model id (brittle if ids change),
- the current seed is `provider: "zed.dev"`, so this would also repoint the
  global seed away from the Cloud provider,
- it is user-overridable and does not express the real intent ("prefer the
  larger context window").

The comparator change is preferred; the seed is documented only as a fallback.

## Testing
- Unit test for `pick_preferred_model`: given two Opus 4.8 entries differing only
  in `max_input_tokens` (200_000 vs 1_000_000), assert the 1M entry is chosen;
  assert the id tie-break when windows are equal.
- Manual: fresh profile (or cleared `agent.default_model`), authenticate the
  Anthropic subscription, open a new agent thread, confirm the default is the 1M
  Opus 4.8 and the context indicator shows ~1M.
- Confirm both variants remain selectable and that an explicit user
  `agent.default_model` is not overridden.
