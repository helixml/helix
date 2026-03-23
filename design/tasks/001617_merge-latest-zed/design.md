# Design: Merge Latest Zed Upstream into Fork

## Repository Layout

- **Fork repo**: `/home/retro/work/zed-4/` — Helix fork of Zed, remote at `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
- **Upstream**: `https://github.com/zed-industries/zed` — must be added as a git remote named `upstream`
- **Porting guide**: `portingguide.md` in repo root — canonical reference for all fork-specific changes
- **E2E tests**: `crates/external_websocket_sync/e2e-test/` — Docker-based, 10-phase test suite

## Merge Strategy

**Rebase onto upstream** (preferred) rather than a merge commit, to keep history linear and diffs against upstream readable. If upstream has diverged significantly and rebase produces too many conflicts, a merge commit is acceptable.

```
git remote add upstream https://github.com/zed-industries/zed
git fetch upstream
git rebase upstream/main   # or: git merge upstream/main
```

All Helix-specific commits are grouped at the tip of the fork's history (see `portingguide.md` Commit History table). This makes rebasing tractable — most conflicts occur in the ~18 modified upstream files.

## Conflict Resolution Approach

The portingguide.md `Rebase Checklist` (items 1–18) is the authoritative guide. Key principles:

1. **Feature-gated blocks always win** — any `#[cfg(feature = "external_websocket_sync")]` block is Helix code and must be kept.
2. **Structural upstream changes need investigation** — if upstream renames fields in `ConnectedServerState` or changes the `AgentConnection` trait, the fork's `from_existing_thread()` and `HeadlessConnection` must be updated to match.
3. **Critical fixes are high-risk** — the four fixes in portingguide.md are small changes to upstream files and are the most likely to be lost. Check each one explicitly after rebasing.
4. **The `external_websocket_sync` crate itself rarely conflicts** — it's a new crate with no upstream counterpart.

## Test Validation Sequence

Run in this order after the merge, stopping on first failure:

1. `cargo check --package zed --features external_websocket_sync` — fast compile check
2. `cargo test -p external_websocket_sync` — unit + protocol tests (no Docker, fast)
3. E2E test build: `docker build -t zed-ws-e2e -f crates/external_websocket_sync/e2e-test/Dockerfile .`
4. E2E test run: `docker run --rm -e ANTHROPIC_API_KEY=<key> -e TEST_TIMEOUT=120 zed-ws-e2e`

The E2E test requires `ANTHROPIC_API_KEY` (real Claude calls). It validates all 10 phases including multi-thread sync, follow-up to non-visible threads, and UI state queries.

## Porting Guide Updates Required

The portingguide.md has a known stale item: the E2E test description says "Four-phase test" but the suite now has 10 phases (latest commit `68042ac` added Phase 10). This must be corrected regardless of what else changes during the merge.

During conflict resolution, document in portingguide.md:
- Any upstream file that was newly modified by the merge (add to "Modified Upstream Files" table)
- Any structural change to a monitored upstream type/function (update the relevant section)
- Decisions made when resolving ambiguous conflicts

## Key Files to Watch

| File | Risk | Reason |
|------|------|--------|
| `crates/agent/src/agent.rs` | High | Critical Fix #1 — entity lifetime |
| `crates/agent_ui/src/acp/thread_view.rs` | High | Critical Fix #2 + `HeadlessConnection`, `from_existing_thread` |
| `crates/acp_thread/src/acp_thread.rs` | High | Critical Fix #3 — `content_only()` |
| `crates/agent_ui/src/agent_panel.rs` | High | All four callback setups |
| `crates/external_websocket_sync/src/thread_service.rs` | Medium | Critical Fix #4 |
| `crates/feature_flags/src/flags.rs` | Low | `enabled_for_all()` override |
| `crates/extensions_ui/src/extensions_ui.rs` | Low | Agent keyword/upsell removal |
| `crates/agent_settings/src/agent_settings.rs` | Low | `show_onboarding`, `auto_open_panel` |
| `Cargo.toml` (workspace root) | Low | `external_websocket_sync` workspace member |

## Notes for Future Merges

- The fork's design deliberately minimises surface area via feature gates. If upstream refactors a gated file, update the cfg block to match the new structure.
- If upstream changes `ConnectedServerState` (adds/removes/renames fields), `from_existing_thread()` in `thread_view.rs` must be updated to match — this is the most structurally fragile integration point.
- The mock Helix server in `e2e-test/helix-ws-test-server/` is standalone Go code; upstream Zed changes don't affect it.
