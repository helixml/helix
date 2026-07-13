# Configurable auto-compaction threshold for external agents (2026-07-13)

## Problem

Long-lived spec-task threads (real Zed native agent, model calls proxied through
Helix to a remote provider) start failing after days/weeks with `504 Gateway
Time-out` from the ingress. Investigation (design doc
`2026-07-13-sandbox-agent-504-provider-slowness`) showed the 504 is
`model_response_time > proxy_timeout`, and reproduced it end-to-end. Context size
is harmless on a *fast* provider, but on a slower/remote provider a large thread's
prefill eventually crosses the ingress timeout.

Zed's native auto-compaction *is* enabled and runs every turn
(`thread.rs::run_turn_internal -> perform_compaction_if_needed`), gated on the
model window being >= 80k and firing at a **default 90%** of the window. The trap:
at 90% of (e.g.) a 200k window the thread is ~180k tokens, and the compaction call
itself must send that whole ~180k-token thread to the summary model. That call is
exactly as large and slow as the turns already timing out, so **compaction 504s
too** and the thread is stuck. That is why it becomes consistent once a task is old
enough to approach the threshold.

Helix writes no `auto_compact` setting, so every external agent inherits the 90%
default.

## Change

Add an opt-in operator override, `HELIX_AGENT_AUTO_COMPACT_THRESHOLD`, written into
the agent's Zed settings as `agent.auto_compact.threshold`. Firing compaction
*earlier* keeps both the running context and the compaction call small enough to
complete on a slow provider, so the thread never reaches the failure zone.

Value is passed through to Zed, which already parses either form:
- a **percentage** (`"50%"`) measured against the model's context window, or
- a **token count** (`"60000"`).

Unset preserves Zed's default (no behaviour change).

## Why a percentage is the clean default

A percentage **auto-scales per model** - `"50%"` is 100k on a 200k model, 64k on a
128k model - so a single value fits every window and Helix does not need to compute
a per-model number. Zed does the math from the model's window.

## Related gap (not fixed here): Helix does not tell Zed the window

`buildLanguageModels` writes only `{api_url}` per provider - no
`available_models[].max_tokens`. So for a *custom* model Zed only knows the context
window if the operator declared it in `available_models`. Zed needs the window both
for the 80k eligibility gate and for a percentage threshold. Helix already knows
each model's window via `ModelInfoProvider` (`model_info.json` + `dynamic_model_info`,
which reads the provider `/models` `max_model_len`), so a follow-up should populate
`available_models[].max_tokens` from that. Until then, a percentage threshold works
wherever the window is known (as in the affected customer's config); a token-count
threshold works regardless but does not auto-scale.

## Non-goals

- Not changing the global default (opt-in only). Whether to lower the default for
  all external agents is a separate product decision.
- Not the summary-model swap (rejected): a compaction call's cost is dominated by
  its *input* (the whole thread), so a cheaper output model does not help. The
  threshold (smaller input) is the correct lever.

## Testing

- `go build ./pkg/external-agent/` passes.
- The change writes `agent.auto_compact.threshold`; Zed's threshold parsing and the
  compaction trigger are already unit-tested upstream. Live confirmation that a
  lowered threshold makes a compaction call appear in `llm_calls` (with the
  `COMPACTION_PROMPT`) and drops the next turn's `prompt_tokens` is tracked
  separately.
