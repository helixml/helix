# Requirements: Default to 1M-Context Opus 4.8 on Zed Subscription

## Background

When using Zed with an Anthropic subscription (the `zed.dev` / Zed Cloud
provider), new agent threads default to a version of **Claude Opus 4.8 that
only exposes a 200k-token context window**. The model menu also lists a second
Opus 4.8 entry with a **1M-token context window** (shown with a `[1m]`-style
label). We want the 1M-context variant to be the default instead of the 200k
one.

### What the codebase investigation found (important)

The `zed.dev` subscription path is **entirely server-driven**. The client does
not hardcode the model list; it fetches `GET /models` and the server returns a
`ListModelsResponse` containing the model list plus a `default_model`,
`default_fast_model`, and `recommended_models` (all referenced by model id).
See `crates/language_models_cloud/src/language_models_cloud.rs:698-772` and the
wire contract in `crates/cloud_llm_client/src/cloud_llm_client.rs:288-333`.

The client's only in-repo "default" is a **settings seed** in
`assets/settings/default.json:1046-1053`:

```json
"default_model": {
  "provider": "zed.dev",
  "model": "claude-sonnet-4",
  "enable_thinking": false
}
```

Resolution order (`crates/language_model/src/registry.rs:414-422`,
`crates/language_models/src/language_models.rs:152-186`):
1. Use the configured `agent.default_model` from settings **if that model id is
   available** in the provider's list.
2. Otherwise use the environment fallback: for `zed.dev`, the cloud provider's
   server-supplied `default_model`, else the first `recommended_models` entry.

Because the seeded `claude-sonnet-4` is not offered on our Anthropic
subscription, the seed is ignored and the effective default comes from the
**server's** `default_model` — currently the 200k Opus 4.8. This means the
default is chosen by our LLM backend's `/models` response, not by Zed client
code, and the two natural fix locations are (a) the client settings seed and
(b) the server's `default_model` designation.

## User Stories

### US-1 — Default to the 1M-context Opus
**As** a Helix/Zed user on the Anthropic subscription,
**I want** new agent threads to default to the 1M-context Opus 4.8,
**so that** I get the larger context window without manually switching models
every time.

**Acceptance Criteria**
- On a fresh profile (no user override of `agent.default_model`), a new agent
  thread is created against the 1M-context Opus 4.8 model, not the 200k one.
- The 200k Opus 4.8 remains selectable in the menu (this change only moves the
  default, it does not remove any model).
- A user who has explicitly set their own `agent.default_model` is not
  overridden by this change.

### US-2 — Deterministic, documented default source
**As** a Helix maintainer,
**I want** a single, clearly-documented place that controls the subscription
default model,
**so that** future changes to the default don't require rediscovering the
server-vs-client resolution logic.

**Acceptance Criteria**
- The mechanism chosen (client settings seed vs. server `default_model`) is
  documented in `design.md`, including the exact file/field changed.
- The chosen 1M model id is verified to exist in the live `/models` response so
  the default actually takes effect rather than silently falling through to the
  old fallback.

## Non-Goals
- Changing the direct bring-your-own-API-key Anthropic provider
  (`crates/language_models/src/provider/anthropic.rs`); it is a separate path.
- Adding, renaming, or removing any model from the menu.
- Introducing a client-side "prefer 1M over 200k" heuristic that inspects
  context windows.

## Open Questions
1. **Separate model vs. "max mode"?** The user describes a distinct menu entry,
   which implies the 1M variant is a **separate `LanguageModel` id** (its `[1m]`
   label coming from the server `display_name`). Please confirm it is a separate
   id and not the same Opus 4.8 exposed via Zed's "Burn/Max Mode"
   (`max_token_count_in_max_mode`). If it is max-mode, the fix is different (see
   design.md) and this spec needs adjustment.
2. **Exact model id + provider.** What is the precise `id` (and confirm
   `provider` is `zed.dev`) of the 1M Opus 4.8 entry in our subscription's
   `/models` response? The client seed must reference an id that actually exists
   in that list, or it is ignored.
3. **Fix location preference.** Given the server-driven design, do we want the
   fix in the **Zed client** (`assets/settings/default.json` seed — in this repo)
   or in the **Helix LLM backend** (`ListModelsResponse.default_model` — a
   different repo, arguably the more robust/global fix)? This spec assumes the
   in-repo client seed as primary, with the server option noted as an
   alternative. Confirm which you want.
4. **Fast/subagent defaults.** Should `default_fast_model` and any subagent /
   inline-assistant default also move to the 1M variant, or only the primary
   `default_model`?
