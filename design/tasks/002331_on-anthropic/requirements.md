# Requirements: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Background

In **Anthropic subscription mode**, Helix runs the Claude Code ACP agent
(`@agentclientprotocol/claude-agent-acp`), which talks **directly to Anthropic**
using the user's Claude Max/Pro OAuth credentials (the token only works through
Claude Code). This path **does not go through Helix's inference proxy or its
model routing** — it is Claude Code ↔ Anthropic.

The only model selection Helix exposes for this mode is a **three-category
picker** returned by `GET /api/v1/claude-subscriptions/models`
(`api/pkg/server/claude_subscription_handlers.go`):

```
opus   → "Claude Opus"   (Most capable)
sonnet → "Claude Sonnet" (Balanced)
haiku  → "Claude Haiku"  (Fastest)
```

These are Claude Code **tier aliases**, not concrete model ids.

### How the selection reaches Claude Code (verified end-to-end)

1. The user's choice is stored as `assistant.ClaudeSubscriptionModel`; if empty
   it defaults to **`"opus"`**
   (`api/pkg/server/zed_config_handlers.go:685-687`).
2. It flows into `CodeAgentConfig.Model` and the settings-sync-daemon writes
   `/etc/claude-code/managed-settings.json` as `{"model": "<value>"}`
   (`api/cmd/settings-sync-daemon/main.go` `writeClaudeManagedSettings`).
3. The ACP package resolves it via `resolveModelPreference(models, preference)`
   against Claude Code's **live** model list from Anthropic.

The ACP list contains **two distinct Opus rows**: a bare Opus (200k) and a
`[1m]` Opus (1M). Per the ACP source, the CLI persists display strings like
`"opus[1m]"` while the SDK ids look like `"claude-opus-4-6-1m"`, and
`resolveModelPreference` unifies the `-1m`/`[1m]` spellings. **`"opus"` (no
context hint) resolves to the bare 200k Opus; `"opus[1m]"` resolves to the 1M
row.** That is why the current default lands on 200k.

> Note on the earlier investigation: Zed's native Anthropic provider
> (`pick_preferred_model`) and the Helix inference-proxy `/v1/models` (which, in
> API-key mode, already reports `claude-opus-4-8` at 1M) are **different code
> paths** and are **not** what governs subscription mode. They are out of scope.

## User Stories

### US-1 — Default to the 1M-context Opus in subscription mode
**As** a user on the Anthropic (Claude Max/Pro) subscription,
**I want** the Opus selection (and the unset default) to use the 1M-context
Opus,
**so that** I get the larger context window without hand-editing model settings.

**Acceptance Criteria**
- With no explicit model chosen, a subscription-mode agent session runs against
  the **1M-context Opus** (Claude Code shows the `[1m]` Opus / ~1M context), not
  the 200k one.
- If the user's subscription/Claude Code list does **not** expose a 1M Opus row,
  the selection degrades gracefully to the best available Opus (no error).
- Sonnet and Haiku selections are unaffected.

### US-2 — Selectable in Helix agent settings (optional but recommended)
**As** a user,
**I want** the 1M Opus to be visible/selectable in the Helix model picker,
**so that** the default is discoverable and I can switch back to 200k if I want.

**Acceptance Criteria**
- The `/api/v1/claude-subscriptions/models` category list and the frontend
  agent-settings picker offer the 1M Opus option (exact UX per Open Question 2).

## Non-Goals
- Zed's native Anthropic provider (`crates/language_models/src/provider/anthropic.rs`)
  and the `zed.dev` Cloud provider — different runtimes, not subscription mode.
- The Helix inference proxy / API-key model routing.
- Changing Sonnet/Haiku behavior.

## Resolved by investigation (previously open)
- The 200k vs 1M Opus are two distinct rows in Claude Code's live model list;
  the `[1m]` row is what the user sees in the menu. Confirmed via the ACP
  `resolveModelPreference` source and the user report.
- The exact value that selects the 1M Opus is the context-hinted alias
  **`"opus[1m]"`** (equivalently `"opus-1m"` / a `…-1m` id — canonicalized to the
  same row).

## Open Questions
1. **Default vs. opt-in.** Do we want to (a) simply change the unset default
   from `"opus"` to `"opus[1m]"` so everyone gets 1M Opus by default, or (b) add
   a distinct "Opus (1M)" category and make *that* the default while keeping a
   200k "Opus" option? (a) is a one-line change; (b) is more discoverable/
   reversible in the UI.
2. **Category-list / UI shape.** If we expose it in the picker, should the three
   categories become four (Opus, Opus 1M, Sonnet, Haiku), or should the existing
   "Opus" category simply map to the 1M variant?
3. **Sonnet 1M too?** Should Sonnet likewise default to / offer its `[1m]`
   variant, or is this Opus-only for now?
4. **Fallback expectation.** Confirm the graceful-degradation behavior in US-1 is
   acceptable for subscriptions where the 1M Opus row isn't available.
