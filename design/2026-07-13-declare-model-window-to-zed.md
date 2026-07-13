# Declare each model's context window to Zed (2026-07-13)

## Problem

Long-lived external-agent threads (real Zed native agent, model calls proxied
through Helix) start failing with ingress `504`s after days/weeks as the thread
grows. Zed has built-in auto-compaction that would bound the context, but it is
**gated on the model's context window** (`>= 80_000`) and fires **relative to it**
(default 90%). For a custom OpenAI-compatible model (e.g. GLM), Zed only knows the
window if it is declared in `language_models.<provider>.available_models[].max_tokens`.

**Helix never declares it.** `buildLanguageModels` writes only `{api_url}` per
provider (confirmed: a live agent's settings had `available_models: None`). So for
a custom model Zed has no window, and its compaction can't size itself.

## Change

Forward the window Helix already knows. `ModelInfoProvider` resolves each model's
`ContextLength` (static `model_info.json` + dynamic provider `/models`), so we look
it up for the selected `(provider, model)` and write
`available_models: [{ name, max_tokens }]` into the model's provider block.

`GenerateZedMCPConfig` gains a `model.ModelInfoProvider` parameter (nil on the
runner-side path that has no model-manager handle). Best-effort: if the window
can't be resolved, behaviour is unchanged (no `available_models` written).

## Why this and not a Helix-side threshold

We deliberately do **not** set `auto_compact.threshold`. Zed's compaction is a
sensible upstream default; Helix's job is to integrate correctly by telling Zed the
window, then let Zed size compaction. No magic number, and it auto-fits every model
(the window is per-model). If a provider is later found so slow that even Zed's 90%
default is too late (the compaction call itself times out), that is revisited as a
separate, evidence-backed decision - not pre-empted with an override.

## Caveat

Resolution is only as good as `ModelInfoProvider`. If a custom model name doesn't
match `model_info.json` and no dynamic entry carries a `ContextLength`, no window is
written. A follow-up could ensure the provider `/models` `max_model_len` is captured
into dynamic model info so arbitrary custom models resolve.

## Testing

- `go build ./pkg/external-agent/ ./pkg/server/` and `go test ./pkg/external-agent/`
  pass.
- End-to-end confirmation that, with the window declared, a long thread triggers a
  Zed compaction call (a `COMPACTION_PROMPT` entry in `llm_calls`, next turn's
  `prompt_tokens` drops) is tracked separately.
