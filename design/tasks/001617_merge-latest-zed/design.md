# Design: Merge Latest Zed Upstream into Fork

## Repository Layout

- **Fork repo**: `/home/retro/work/zed-4/` — Helix fork of Zed, remote at `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
- **Upstream**: `https://github.com/zed-industries/zed` — must be added as a git remote named `upstream`
- **Porting guide**: `portingguide.md` in repo root — canonical reference for all fork-specific changes
- **E2E tests**: `crates/external_websocket_sync/e2e-test/` — Docker-based, 10-phase test suite

## Merge Strategy

**Branch name**: `feature/001617-merge-latest-zed` — the internal git server only accepts pushes to branches matching the `feature/<task-id>-*` pattern, as seen across all existing task branches (e.g. `feature/001608-when-zed-first-starts-up`).

**Rebase onto upstream** (preferred) rather than a merge commit, to keep history linear and diffs against upstream readable. If upstream has diverged significantly and rebase produces too many conflicts, a merge commit is acceptable.

```
git checkout -b feature/001617-merge-latest-zed
git remote add upstream https://github.com/zed-industries/zed
git fetch upstream
git rebase upstream/main   # or: git merge upstream/main
```

All Helix-specific commits are grouped at the tip of the fork's history (see `portingguide.md` Commit History table). This makes rebasing tractable — most conflicts occur in the ~18 modified upstream files.

## Sandbox Version Pin

`/home/retro/work/helix-4/sandbox-versions.txt` pins the Zed commit SHA used in the Helix sandbox:

```
ZED_COMMIT=23412509b7b6568a0af54cb0db8b1f2aa7036a24
```

After the merge is complete and tests pass, this file must be updated to the new HEAD SHA of the zed-4 `main` branch, and a PR raised against helix-4. This is what causes the sandbox to actually use the updated Zed build.

## Known Upstream Breaking Change: ACP Consolidation

Upstream Zed has retired the legacy non-ACP native agent and made all agent functionality go through ACP. As part of this, they have removed the `acp` prefix from various types, crates, and file paths (e.g. `acp_thread` may be renamed, `AcpThread` → something else, `crates/agent_ui/src/acp/` directory structure may change).

**What this means for the fork:**

- The fork's Helix WebSocket sync layer still needs ACP — ACP is what backs thread/session storage in the agent. This has not changed. Do not interpret the upstream rename as a reason to remove ACP support.
- The rename is cosmetic from our perspective: upstream consolidated *to* ACP, so the underlying protocol and thread model are the same. Our `thread_service.rs` and `HeadlessConnection` integration points still map to the same concepts.
- Concretely, file paths and type names referenced in the portingguide.md may have changed. When resolving conflicts, track down the new names and update portingguide.md references accordingly. Do not assume a missing `acp_thread` crate means the feature is gone — find where it moved.
- `Critical Fix #3` (`content_only()` on `AssistantMessage` in `crates/acp_thread/`) is particularly at risk: if the crate was renamed, find the equivalent type in its new location and ensure the fix is present there.

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
3. E2E test: `ANTHROPIC_API_KEY=<key> crates/external_websocket_sync/e2e-test/run_docker_e2e.sh`

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
| `crates/agent_ui/src/acp/thread_view.rs` | High | Critical Fix #2 + `HeadlessConnection`, `from_existing_thread` — path may have changed upstream due to ACP consolidation |
| `crates/acp_thread/src/acp_thread.rs` | High | Critical Fix #3 — `content_only()` — crate may be renamed upstream, find new location |
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
