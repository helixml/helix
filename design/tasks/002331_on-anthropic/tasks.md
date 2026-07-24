# Implementation Tasks: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Decisions (finalized during implementation)
- **Approach 2b**: add an explicit "Opus (1M)" option (`opus[1m]`), make it the
  default, and keep the 200k "Opus" option selectable. Discoverable + reversible.
- **Opus-only** for now (no Sonnet 1M) — matches the user's request.
- Three places define the tier list/default and must stay in sync: frontend
  `CodingAgentForm.tsx`, backend `listClaudeModels`, backend subscription-branch
  default in `zed_config_handlers.go`.

## Implementation (Helix)
- [x] Frontend: in `frontend/src/components/agent/CodingAgentForm.tsx`, add an
      `opus[1m]` entry to `CLAUDE_SUBSCRIPTION_MODELS` and set
      `DEFAULT_CLAUDE_SUBSCRIPTION_MODEL = 'opus[1m]'` (consumed by AppSettings).
- [x] Backend: add the `opus[1m]` category to `listClaudeModels`
      (`api/pkg/server/claude_subscription_handlers.go`).
- [x] Backend: change the subscription-branch empty-default in
      `api/pkg/server/zed_config_handlers.go` from `"opus"` to `"opus[1m]"`
      (also updated the `ClaudeSubscriptionModel` doc comment in `types.go`).

## Tests / build
- [x] `go build ./pkg/server/... ./pkg/types/...` OK; frontend `tsc --noEmit` OK.
- [x] Updated + passing subscription-mode default tests
      (`zed_config_handlers_test.go`).

## Wrap-up
- [x] Update design.md implementation notes; PR description files.

## Verification (manual, later)
- [ ] Connect a Claude subscription; new subscription-mode task defaults to the
      `[1m]` Opus (~1M context).
- [ ] Sonnet / Haiku still resolve; 200k "Opus" still selectable.
- [ ] Subscription lacking a 1M Opus row still starts (graceful fallback).
