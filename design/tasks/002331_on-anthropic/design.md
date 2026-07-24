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

## Testing
- Unit: assert `ClaudeSubscriptionModel` default is `"opus[1m]"` where it is
  seeded; if a category mapping is added, assert `listClaudeModels` returns it.
- Manual: connect a Claude subscription, start a subscription-mode spec task with
  no explicit model, confirm Claude Code runs the `[1m]` Opus (~1M context) and
  that Sonnet/Haiku selections still resolve correctly.
- Manual: a subscription without a 1M Opus row still starts (falls back to 200k
  Opus, no error).
