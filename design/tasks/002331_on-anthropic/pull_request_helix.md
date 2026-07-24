# Default Anthropic subscription mode to the 1M-context Opus

## Summary

In Anthropic subscription mode, Claude Code talks directly to Anthropic and picks
its model from the tier alias Helix writes into `managed-settings.json`. The
alias `"opus"` resolves to the **200k-context** Opus; the context-hinted alias
`"opus[1m]"` resolves to the **1M-context** Opus (Claude Code's
`resolveModelPreference` canonicalizes `opus[1m]` / `opus-1m` / `claude-opus-*-1m`
to the 1M row). This changes the default from `"opus"` to `"opus[1m]"` so new
subscription-mode agents get the 1M context, while keeping the 200k Opus
selectable.

## Changes

- Backend default: `api/pkg/server/zed_config_handlers.go` — subscription-branch
  empty-model default is now `"opus[1m]"` (was `"opus"`).
- Model picker API: `api/pkg/server/claude_subscription_handlers.go` —
  `listClaudeModels` now returns "Claude Opus (1M context)" (`opus[1m]`) and
  "Claude Opus (200k context)" (`opus`) alongside Sonnet/Haiku.
- Frontend: `frontend/src/components/agent/CodingAgentForm.tsx` — adds the
  `opus[1m]` option (labelled recommended) and sets
  `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL = 'opus[1m]'`.
- Docs/comment: `api/pkg/types/types.go` — `ClaudeSubscriptionModel` doc updated
  to reflect the new default.
- Tests: `api/pkg/server/zed_config_handlers_test.go` — default cases updated to
  `"opus[1m]"`.

## Notes

- Users who explicitly select the 200k "Opus" still get 200k (override path
  unchanged).
- If a subscription's Claude Code list has no 1M Opus row, `resolveModelPreference`
  falls back to the best same-family (200k) row — no error.
