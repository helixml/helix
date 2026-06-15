# chore(zed): bump ZED_COMMIT to 5ed995947e (002100 merge)

## Summary

Bumps `ZED_COMMIT` in `sandbox-versions.txt` from `f82e1c6760` (002077 merge of upstream Zed `zed-industries/zed@992f395c3d`) to `5ed995947e` (002100 merge of upstream Zed `zed-industries/zed@a31d3505da`, 25 upstream commits absorbed over 3 days — the smallest catch-up window in this series).

Pairs with the companion `helixml/zed` PR `feature/002100-merge-latest-zed`. **Merge that PR first**, then merge this one — the spec-task system marks the task done when all PRs merge, and bumping CI to a commit that hasn't landed yet would point at a non-existent SHA temporarily.

## Changes

- `sandbox-versions.txt`: `ZED_COMMIT=f82e1c6760…` → `ZED_COMMIT=5ed995947e…`

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

- This is the **second consecutive** Zed merge that required **zero** Helix-side signature-drift repairs (002077 was the first). The Helix surface auto-merged cleanly across all critical fixes and load-bearing patches; the only conflict was a trivial both-sides-added-a-field on `RemoteSettingsContent` (Helix `suggest_dev_container` vs upstream `dev_container_use_buildkit`).
- Fork main was quiet for the full 3-day window — zero Helix-only commits since 002077 merged.

## Test plan

- [x] Companion Zed PR built (`./stack build-zed dev`) and E2E-tested green for both `zed-agent` and `claude` personalities.
- [ ] CI sandbox rebuild against new `ZED_COMMIT` (will run on push).
- [ ] Inner-Helix smoke test of the bumped sandbox image after CI green.

Release Notes:

- N/A
