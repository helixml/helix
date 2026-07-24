# Implementation Tasks: Default to 1M-Context Opus 4.8 on Anthropic Subscription

## Decisions to confirm (small)
- [ ] Default vs. opt-in and UI shape (requirements Open Questions 1 & 2):
      change default only, map "Opus" category → 1M, or add an explicit
      "Opus (1M)" category.
- [ ] Sonnet 1M in scope or Opus-only (Open Question 3).

## Primary implementation (Helix)
- [ ] In `api/pkg/server/zed_config_handlers.go` (subscription branch ~685-687),
      change the empty-model default from `"opus"` to `"opus[1m]"`.

## Companion (discoverability, per decision above)
- [ ] Option 2a: map the existing `opus` category to `"opus[1m]"`, OR
- [ ] Option 2b: add `{ id: "opus[1m]", name: "Claude Opus (1M)" }` to
      `listClaudeModels` in `api/pkg/server/claude_subscription_handlers.go` and
      make it the default; retain a 200k "Opus" option.
- [ ] Update the frontend agent-settings model picker
      (`frontend/src/...`) to show/select the 1M Opus option consistently with
      the backend category list.

## Tests
- [ ] Assert the subscription-mode default resolves to `"opus[1m]"`.
- [ ] If a category is added/changed, assert `listClaudeModels` output.
- [ ] Confirm existing subscription-mode tests still pass.

## Verification
- [ ] Connect a Claude subscription; start a subscription-mode spec task with no
      explicit model; confirm Claude Code runs the `[1m]` Opus (~1M context).
- [ ] Confirm Sonnet / Haiku selections still resolve correctly.
- [ ] Confirm a subscription lacking a 1M Opus row still starts (graceful
      fallback to 200k Opus, no error).
