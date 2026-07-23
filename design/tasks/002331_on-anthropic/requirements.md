# Requirements: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Background

When using Zed with a **direct Anthropic subscription** (Zed's built-in
Anthropic provider, provider id `"anthropic"`, authenticated with an Anthropic
account/subscription), new agent threads default to a version of **Claude
Opus 4.8 that only exposes a 200k-token context window**. The model menu also
lists a second Opus 4.8 entry with a **1M-token context window** (shown with a
`[1m]`-style label). We want the 1M-context variant to be the default.

> Note: this is the **direct Anthropic provider**, not the `zed.dev` / Zed
> Cloud provider. The two use different model-selection code paths.

### What the codebase investigation found

The direct Anthropic provider fetches its model list dynamically from Anthropic's
`/v1/models` endpoint. Both the 200k and the 1M Opus 4.8 entries come from that
response as **two distinct model ids** with different `max_input_tokens` values;
the client shows whatever the API returns (`crates/anthropic/src/anthropic.rs`
`Model::from_listed`, `max_input_tokens: entry.max_input_tokens`). There is no
client-side "1M" synthesis and no hardcoded model enum.

The default model is chosen in
`crates/language_models/src/provider/anthropic.rs`:

- `default_model()` (line ~224) → `pick_preferred_model(&fetched,
  &["claude-sonnet-", "claude-opus-", "claude-"])`.
- `pick_preferred_model()` (line ~333) takes, within the first matching prefix
  group, the model with the **lexicographically-greatest `id`**
  (`max_by(|a, b| a.id.cmp(&b.id))`). **It never looks at the context window.**

So today the default among Opus entries is decided purely by string comparison
of the model id, which currently lands on the 200k variant. The larger context
window of the 1M variant plays no part in the choice. (`default_fast_model()`
and `recommended_models()` use the same `pick_preferred_model` helper.)

Resolution flow that reaches this code: with no explicit user
`agent.default_model` that is available, the registry's environment fallback
uses each authenticated provider's `default_model()`
(`crates/language_model/src/registry.rs:414-422`,
`crates/language_models/src/language_models.rs:152-186`).

## User Stories

### US-1 — Default to the 1M-context Opus
**As** a user on the direct Anthropic subscription,
**I want** new agent threads to default to the 1M-context Opus 4.8,
**so that** I get the larger context window without switching models each time.

**Acceptance Criteria**
- On a fresh profile (no user override of `agent.default_model`), when the
  Anthropic subscription's model list contains both a 200k and a 1M Opus 4.8,
  the default resolves to the **1M** variant.
- Both variants remain listed and selectable in the model menu (this change only
  moves the default; it removes nothing).
- A user who has explicitly set their own `agent.default_model` is not
  overridden.

### US-2 — Context-window-aware, id-agnostic selection
**As** a Helix maintainer,
**I want** the preference to be based on the model's context window rather than
a hardcoded model id,
**so that** the default keeps preferring the larger-context variant even if the
subscription's model ids change.

**Acceptance Criteria**
- The default-selection logic prefers the candidate with the larger
  `max_input_tokens` within a prefix group, without hardcoding a specific
  Opus/1M model id.
- Behavior is deterministic (a defined tie-break when context windows are
  equal).

## Non-Goals
- Changing the `zed.dev` / Zed Cloud provider path.
- Adding, renaming, or removing any model from the menu.
- Changing which model *family* is preferred (Sonnet-before-Opus prefix order
  stays as-is unless Open Question 2 says otherwise).

## Open Questions
1. **Confirm two distinct ids.** We believe the 200k and 1M Opus 4.8 are two
   separate `/v1/models` entries with distinct ids and distinct
   `max_input_tokens` (they must be distinct ids, or they would collide in the
   id-keyed model map and only one would appear in the menu). Please confirm,
   and if convenient share the two ids and their `max_input_tokens` values.
2. **Scope of the context-window preference.** Should "prefer the larger context
   window within a prefix group" apply to *all* groups (`claude-sonnet-`,
   `claude-opus-`, `claude-`, and the fast/haiku path), or only to Opus? Applying
   it everywhere is simpler and consistent, but it would also make a
   larger-context Sonnet/Haiku variant the preferred one in those groups.
3. **Which prefix group actually resolves.** `default_model()` prefers Sonnet
   before Opus. Since you observe Opus 4.8 as the default, your subscription's
   fetched list presumably contains no Sonnet (or only Opus). Please confirm the
   fetched model list so we target the right group.
4. **Fast / recommended defaults.** Should `default_fast_model()` and
   `recommended_models()` adopt the same larger-context preference, or only the
   primary `default_model()`?
