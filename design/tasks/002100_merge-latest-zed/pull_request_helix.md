# chore(zed): bump ZED_COMMIT to 7e0a439153 (002100 merge, two rounds)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` from `f82e1c6760` (002077 merge of upstream Zed `zed-industries/zed@992f395c3d`) to `7e0a439153` (002100 merge of upstream Zed `zed-industries/zed@e45e42af6e`, **120 upstream commits absorbed over 6 days** across two rounds: round 1 covered 25 commits to `a31d3505da`, round 2 / extension covered the next 95 commits to `e45e42af6e`).

Pairs with the companion `helixml/zed` PR `feature/002100-merge-latest-zed`. **Merge that PR first**, then merge this one — the spec-task system marks the task done when all PRs merge, and bumping CI to a commit that hasn't landed yet would point at a non-existent SHA temporarily.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT=f82e1c6760…` → `ZED_COMMIT=7e0a439153…`

## What's in the new ZED_COMMIT

25 upstream commits over 3 days, plus the Helix porting-guide entry. Headline items:

- **Editor**: bracket-splitting in line comments (`cccc7b2d44`), columnar-selection alignment on multi-byte rows (carried in from 002077's HEAD `992f395c3d` is now the merge fence — this window's commits are post-fence).
- **GPUI macOS**: warn-on-NoopTextSystem fallback (`759bacff37`), traffic-light hitbox after repositioning (`138139f830`), revert "title bar clicks delayed on macOS 27" (`fca2ccd403`).
- **File finder**: list-item design adjustment (`cb7721602b`), align split buffer headers with scrollbar (`96285fc140`).
- **Agent / agent_ui**: shell-hang fix on syntax errors with terminal tool usage (`c578f4d12b`), truncate long model names in config selector (`c7987fabf7`), match project name in archive-view search (`df9c9f055e`), remove dead `Rules Library` menu link (`1e017d04b9`).
- **Extensions UI**: hide agent-server/grammar/slash-command/docs chips from extension filter (`f39cf25c0b`).
- **Component preview**: fix example rendering no text/icons (`f9d57b54dc`).
- **Client**: link to appropriate docs for Preview/Nightly channels (`3e54ce8bce`).
- **Recent projects**: close remote-projects window on button press (`e044074eb2`).
- **Markdown preview**: improve heading spacing (`29b90dc4e8`).
- **REPL**: show kernel error on notebook cell instead of hanging (`26a355b11d`).
- **Audio**: fix phantom presence in channels (`e5b6041e9d`).
- **Call**: log LiveKit connection-info refresh outcomes in retry loop (`abbe85a332`).
- **CI**: fix caching of release jobs (`1d217ee39d`); patch `async-process` to allow reusing their reaper (`d4cc8d2409`).
- **Multi-buffer**: don't eagerly clone `BufferSnapshot` in `range_to_buffer_ranges` (`b6c7496aea`).
- **Prettier**: read project `.editorconfig` files when formatting from within Zed (`8589cbbf6b`).
- **Dev containers**: support classic Docker builder via `dev_container_use_buildkit` setting (`26fc42721a`), expand bare `$VAR` in addition to `${VAR}` in Dockerfiles (`832ab56db8`).
- **Git**: don't run `git stash list` on every file save (`a31d3505da`).

## Notes

- This is the **third consecutive** Zed merge that required **zero** Helix-side signature-drift repairs (002077 → 002100 round 1 → 002100 round 2/extension). The Helix surface auto-merged cleanly across all critical fixes and load-bearing patches in both rounds.
- Round 1 had one trivial conflict (`RemoteSettingsContent` both-sides-added-a-field); round 2 had one trivial conflict (`grep_tool.rs` `snippet`/`text` variable refactor, kept Helix's `truncate_long_lines` 001410 patch).
- Round 2 was a much denser window: 95 upstream commits in 3 days vs round 1's 25 commits in 3 days. Major upstream clusters absorbed: agent sandboxing (Linux + Windows WSL + network allowlist + in-process proxy), in-thread search bar (`+962` new file), provider-side compaction in language model clients, max_output_tokens accounting in compaction threshold, grep-tool clickable results, default_open_behavior on Finder, the v1.9.0 version bump (`4cab63fb59`).
- Between round 1 and round 2, fork main absorbed PR #63 (claude-agent-acp wedge recovery, 6 commits) and PR #64 (`agent_ready` re-emit on reopening already-loaded thread, 1 commit). These ship as part of this bump.

## Test plan

- [x] Companion Zed PR built (`./stack build-zed dev`) and E2E-tested green for both `zed-agent` and `claude` personalities.
- [ ] CI sandbox rebuild against new `ZED_COMMIT` (will run on push).
- [ ] Inner-Helix smoke test of the bumped sandbox image after CI green.

Release Notes:

- N/A
