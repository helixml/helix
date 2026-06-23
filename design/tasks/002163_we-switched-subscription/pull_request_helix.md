# Use tier-level shorthand for subscription model default

## Summary

The subscription mode was defaulting to `claude-opus-4-6` (two versions behind). Instead of bumping to `claude-opus-4-8` (which breaks again on the next release), this changes the default to `"opus"` — a tier-level shorthand that Claude Code's `resolveModelPreference()` resolves to the latest Opus automatically.

## Changes

- **Backend default**: `"opus"` instead of `"claude-opus-4-6"` in `zed_config_handlers.go`
- **Model list endpoint**: returns `"opus"`, `"sonnet"`, `"haiku"` (no version pinning)
- **Frontend dropdown**: labels updated to "Claude Opus", "Claude Sonnet", "Claude Haiku"
- **Normalizer fix**: added missing `claude-opus-4-7` and `claude-opus-4-8` entries in `normalizeModelIDForZed` (API-key path bug)
- **Onboarding**: bumped API-key path default to `claude-opus-4-8`
- **RECOMMENDED_CODING_MODELS**: added `claude-opus-4-8` as first entry

## QA notes

- Verify in a real subscription container that writing `"model": "opus"` to `/etc/claude-code/managed-settings.json` resolves to the latest Opus
- Existing agents with `claude-opus-4-6` saved should continue to work unchanged
- The OpenAPI spec (`docs.go`) needs regeneration via `./stack update_openapi`
