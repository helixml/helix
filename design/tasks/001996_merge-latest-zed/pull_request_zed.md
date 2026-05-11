# Merge upstream Zed (1da60a8518..8bdd78e023, 127 commits) into Helix fork

## Summary

Brings the Helix fork up to date with upstream Zed `8bdd78e023` (2026-05-10), 127 commits over 3 days since 001980's `1da60a8518`. All 11 Critical Fixes preserved, all PRs #51–#53 carried through, full E2E suite (14 phases × 2 agents) green.

## Changes

- **Merge commit `bf544922aa`**: `git merge upstream/main`. One conflict in `crates/acp_thread/src/acp_thread.rs` `run_turn()`:
  - Upstream PR #55562 (`0a52f80824`) reordered `running_turn.take()` to come BEFORE the dropped-tx early return, so the panel exits the Generating state when `send_task` is cancelled before `tx.send`.
  - Helix had its own dropped-tx branch with `Stopped(Cancelled)` emission guarded by `stopped_emitted_for_task` (Critical Fixes #6/#8/#9).
  - Resolution folds both: single same-turn `take()` first (upstream's reorder), then dropped-tx branch with the Helix duplicate-guarded `Stopped(Cancelled)` emission. Strict superset of both sides.
- **Build fix `1828cea13c`**: upstream added `BaseView::Terminal { terminal_id }` variant. Helix UI state query in `agent_panel.rs` was non-exhaustive — added a `Terminal` arm reporting `("terminal", None, 0, None)`.
- **Phase 13 race fix `a7ad11ec00`**: discovered by E2E that `message_completed` was racing `turn_cancelled` to the wire (GPUI flushes queued events at the end of the entity update, so the synchronously-emitted `Stopped(Cancelled)` triggered `MessageCompleted` before `cancel()` returned). Reordered the cancellation handler in `thread_service.rs`: probe `thread.status()` first, send `turn_cancelled{cancelled}` BEFORE invoking `cancel()` (so Helix marks Interrupted before MessageCompleted clobbers it), and send `noop` if no turn was running. The probe also tightens the previous logic which sent `cancelled` even when the thread had no running turn.
- **Porting guide updated** (`a767007e53`): new `## Merge 001996` section with conflict trail, `BaseView::Terminal` build-fix lesson, Phase 13 race deep-dive, and 3 new commit-history entries.

## Validation

- `./stack build-zed dev`: green (warm cache 46s, 0 errors)
- E2E `zed-agent`: ALL 14 PHASES PASSED
- E2E `claude`: ALL 14 PHASES PASSED (after one Claude Code npm-install bootstrap flake on a previous attempt — unrelated to this merge)
- All 11 Critical Fixes verified present
- Full silent-drift sweep clean (`ActiveView`/`set_active_view`/`draft_threads`/`selected_agent_type`/`Stopped[^(]`/`smol::Timer` all clean; `--allow-multiple-instances`, `--headless`, `debug-embed`, `external_websocket_sync::get_thread` (Critical Fix #11) all present)

Release Notes:

- N/A
