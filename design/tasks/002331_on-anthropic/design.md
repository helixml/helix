# Design: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Summary

In Anthropic subscription mode, Helix tells Claude Code which model to use by
writing a tier alias (default `"opus"`) into
`/etc/claude-code/managed-settings.json`. Claude Code's ACP layer resolves
`"opus"` to the **bare 200k** Opus. Resolving to the **1M** Opus instead only
requires writing the context-hinted alias **`"opus[1m]"`**. The change lives in
**Helix** (primary repo); Zed is not modified.

## Verified control flow

```
Helix agent settings (three-category picker: opus/sonnet/haiku)
        │  GET /api/v1/claude-subscriptions/models   (claude_subscription_handlers.go)
        ▼
assistant.ClaudeSubscriptionModel  ──(empty ⇒ "opus")──►  CodeAgentConfig.Model
        │   api/pkg/server/zed_config_handlers.go:685-687
        ▼
settings-sync-daemon → /etc/claude-code/managed-settings.json {"model": "<v>"}
        │   api/cmd/settings-sync-daemon/main.go  writeClaudeManagedSettings()
        ▼
Claude Code ACP  resolveModelPreference(models, "<v>")   (@agentclientprotocol/claude-agent-acp)
        │   models = Claude Code's LIVE Anthropic model list (direct, no proxy)
        ▼
   "opus"     → bare Opus row      (200k)
   "opus[1m]" → "[1m]" Opus row    (1M)   ← canonicalizeModelId unifies -1m / [1m]
```

Key evidence in the ACP source (`dist/acp-agent.js`):
- *"Claude Code CLI persists display strings like `opus[1m]` … but the SDK model
  list uses IDs like `claude-opus-4-6-1m`."*
- `resolveModelPreference` first tries exact/canonical/resolved matches, then a
  hint-aware substring tier that **won't let a bare row absorb a 1M-hinted
  preference**, then tokenized matching for aliases like `"opus[1m]"`.
- If no 1M row exists, tokenized matching falls back to the best same-family
  row (the 200k Opus) — i.e. graceful degradation, no error.

## Decision

**Primary (minimal): change the subscription-mode default alias to `"opus[1m]"`.**

In `api/pkg/server/zed_config_handlers.go` (subscription branch, ~685-687):

```go
model = assistant.ClaudeSubscriptionModel
if model == "" {
    model = "opus[1m]"   // was "opus"; 1M-context Opus by default
}
```

Rationale:
- Directly and minimally produces the requested behavior.
- Uses the alias the ACP layer is explicitly built to understand
  (`resolveModelPreference` tokenized matching); no dependency on a specific
  dated model id.
- Degrades gracefully when a subscription lacks the 1M row.
- No change to Zed or to the ACP package.

**Recommended companion (discoverability — pending Open Questions 1/2):** surface
the 1M Opus in the picker so users see and can change it, one of:
- (2a) map the existing `opus` category to `"opus[1m]"` (simplest — the "Opus"
  the user picks is the 1M one), or
- (2b) add a fourth category `{ id: "opus[1m]", name: "Claude Opus (1M)" }` in
  `listClaudeModels` (`claude_subscription_handlers.go`) and make it the default,
  keeping a 200k "Opus" option; update the frontend agent-settings picker
  accordingly.

Recommendation: do the default change (primary) plus (2a) unless we want an
explicit 200k option retained, in which case (2b).

## What was ruled out
- **Zed `pick_preferred_model`** (`crates/language_models/src/provider/anthropic.rs`):
  governs the *native* Zed agent / Anthropic-API-key provider, a different
  runtime than subscription mode.
- **Helix inference proxy `/v1/models`**: in API-key mode it already reports
  `claude-opus-4-8` at 1M, but subscription mode bypasses the proxy, so it has no
  effect here.
- **Pinning a dated id** (e.g. `claude-opus-4-8-1m`): brittle across model
  updates; the `"opus[1m]"` alias is the durable choice.

## Implementation Notes (as built)

Chose **approach 2b** (explicit 1M option, default to it, keep 200k). Opus-only.

The tier list/default is defined in **three** places that must stay in sync — the
frontend keeps its own hardcoded copy and sends the chosen value explicitly, so a
backend-only change would not affect UI-created apps:

1. `frontend/src/components/agent/CodingAgentForm.tsx` —
   `CLAUDE_SUBSCRIPTION_MODELS` gained an `opus[1m]` entry ("Claude Opus (1M
   context, recommended)") and a retained `opus` ("200k"); `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL`
   is now `'opus[1m]'`. Consumed by `AppSettings.tsx` (`app.claude_subscription_model
   || DEFAULT_CLAUDE_SUBSCRIPTION_MODEL`), so the picker shows and defaults to it.
2. `api/pkg/server/claude_subscription_handlers.go` — `listClaudeModels` returns
   the `opus[1m]` and `opus` categories alongside `sonnet`/`haiku`.
3. `api/pkg/server/zed_config_handlers.go` — subscription-branch empty default
   changed from `"opus"` to `"opus[1m]"`; `types.go` `ClaudeSubscriptionModel`
   doc comment updated to match.

Tests: `zed_config_handlers_test.go` default cases updated to `"opus[1m]"` and
pass; `go build` of `pkg/server`/`pkg/types` and frontend `tsc --noEmit` both clean.

Gotchas:
- The value travels as a raw JSON string through `managed-settings.json`; the
  `"[1m]"` bracket form is exactly what the ACP `resolveModelPreference` expects
  (it canonicalizes `opus[1m]` / `opus-1m` / `claude-opus-*-1m`).
- The `override` path (explicit `ClaudeSubscriptionModel`) is unchanged — a user
  who explicitly picks 200k "Opus" still gets 200k.

## Testing
- Unit: assert `ClaudeSubscriptionModel` default is `"opus[1m]"` where it is
  seeded; if a category mapping is added, assert `listClaudeModels` returns it.
- Manual: connect a Claude subscription, start a subscription-mode spec task with
  no explicit model, confirm Claude Code runs the `[1m]` Opus (~1M context) and
  that Sonnet/Haiku selections still resolve correctly.
- Manual: a subscription without a 1M Opus row still starts (falls back to 200k
  Opus, no error).
