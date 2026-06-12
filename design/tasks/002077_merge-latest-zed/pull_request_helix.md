# chore(zed): bump ZED_COMMIT to 38d4f86809 (002077 merge)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` from `ecdc2ea67d` (PR #60: `claude-agent-acp` `ede_diagnostic` retry) to `38d4f86809` (002077 merge of upstream Zed `zed-industries/zed@992f395c3d`, 256 upstream commits absorbed).

Pairs with the companion `helixml/zed` PR `feature/002077-merge-latest-zed`. **Merge that PR first**, then merge this one — the spec-task system marks the task done when all PRs merge, and bumping CI to a commit that hasn't landed yet would point at a non-existent SHA temporarily.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT=ecdc2ea67d…` → `ZED_COMMIT=38d4f86809…`

## What's in the new ZED_COMMIT

256 upstream commits over 10 days, plus the Helix porting-guide entry. Headline items:

- **Compaction**: built-in `/compact` slash command (`e5052961af`), `auto_compact` agent setting (`9baefe701e`), compaction UX refinements (`e17e272d24`, `0e9e8d0e68`), compaction-cancel race fix (`5c90b0664f`), hide token usage after `/compact` (`0bc6c76fcf`).
- **ACP**: stabilised session usage/deletion (`56b71271c4`), preserve waiting tool call status on updates (`d7ac5e6cf4`, +602 lines), raw agent registry binaries (`8ce658ffa7`), ACP permission dialog keyboard shortcut fix (`d130d03f5d`).
- **Agent / thread**: cumulative token usage accumulation + persistence (`27191913e9`), flush thread content on app quit (`620ceaaaca`), inherit source agent without draft content (`116e4bc184`), keep pending subagent edits when regenerating a prompt (`c78bd36fd8`), thread import UX improvements (`5a318bad74`).
- **Workspace / UI**: typed workspace errors (`215ca2fb0b`) + overflow popup fix (`83aa943705`), update window title when switching workspace (`a32999e00b`), reset pane sizes command (`dde7c1c07f`).
- **Settings / providers**: Anthropic-compatible provider support (`fef979dec4`), sandbox write-path hardening (`89cac4944d`).
- **Cloud refactor**: typed completion errors (`a98485809b`), `crates/language_model/src/model/` deleted (`CloudModel` relocated to `language_model_core` + `language_models/src/provider/cloud.rs`).
- **Editor / language**: multi-byte char fixes (`992f395c3d`, `137e677a05`), GLSL syntax indentation (`16991c9a29`), helix-mode line selection fix (`befa775029`).

## Notes

- This is the first Zed merge since 002029 that required **zero** Helix-side signature-drift repairs. The Helix surface auto-merged cleanly across all critical fixes and load-bearing patches.
- This bump implicitly ships PR #60's `ede_diagnostic` retry loop (which has been on fork main since 2026-06-09 but was already included in the previous `ZED_COMMIT=ecdc2ea67d` bump; no new behaviour on top).

## Test plan

- [x] Companion Zed PR built and E2E-tested green for both `zed-agent` and `claude` personalities.
- [ ] CI sandbox rebuild against new `ZED_COMMIT` (will run on push).
- [ ] Inner-Helix smoke test of the bumped sandbox image after CI green.

Release Notes:

- N/A
