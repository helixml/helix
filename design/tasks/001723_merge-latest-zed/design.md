# Design: Merge Latest Zed Upstream into Helix Fork

## Repository Layout

- **Fork repo**: `/home/retro/work/zed/` — remote at `http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-zed-4`
- **Upstream**: `https://github.com/zed-industries/zed` — add as remote named `upstream` (already added during planning)
- **Porting guide**: `portingguide.md` in repo root — canonical reference for all fork-specific changes
- **E2E tests**: `crates/external_websocket_sync/e2e-test/` — Docker-based, 10-phase test suite
- **Helix repo**: `/home/retro/work/helix/` — for updating `sandbox-versions.txt` after merge

## Current State

| Metric | Value |
|--------|-------|
| Upstream HEAD | `7597666c08c8a2bbb45e9b02954112194e31f6f4` |
| Fork HEAD | `42e50d49ba6341cf4136aa4e7f7a1a3702d759a7` |
| Commits behind | 506 |
| Helix-specific commits ahead | 165 |
| Files changed upstream | 1,375 (140,703 insertions, 116,539 deletions) |
| Last merge task | 001617 (March 2026) |

## Merge Strategy

**Use `git merge upstream/main`** (merge commit, not rebase). This is consistent with the last 3 merges (001554, 001560, 001617) and keeps the Helix commit history intact.

```bash
cd /home/retro/work/zed
git checkout -b feature/001723-merge-latest-zed
git fetch upstream
git merge upstream/main
# Resolve conflicts
# Test
# Push
```

## High-Risk Upstream Changes (506 Commits)

### 1. Agent V2 Feature Flag Removal (`#52792`)
Upstream removed the Agent V2 feature flag entirely, making the new agent architecture the default. This likely changes conditionals and code paths in `agent_panel.rs` and `agent.rs` that Helix's cfg-gated code may depend on.

**Risk**: Medium. Our code is behind `external_websocket_sync` feature gates, not Agent V2 flags.

### 2. Sidebar / MultiWorkspace Rework (`#53096`, `#53032`, `#53267`, `#53364`)
Major restructuring: sidebar rendering now uses `MultiWorkspace`'s project groups, `ProjectGroup` renamed to `Project`, temporary/retained workspace behavior introduced. This affects `workspace.rs` where Helix has the agent-follow focus guard.

**Risk**: High. The `follow()` and `update_follower_items()` guards must be re-applied after merge. The workspace removal fix (`#53366`) may conflict with our focus-stealing prevention.

### 3. Agent Panel Git Worktree/Branch Pickers (`#52979`, `#53324`, `#53334`, `#53351`, `#53372`)
Upstream added git worktree/branch pickers to the agent panel, substantially modifying `agent_panel.rs`. New fields: worktree pickers, error callouts, thread picker resets.

**Risk**: High. `agent_panel.rs` has 4,088 lines changed — our cfg-gated callback setup, `from_existing_thread` calls, onboarding dismissal, and `acp_history_store()` accessor will need careful re-application.

### 4. Worktree Archival on Thread Archive (`#53215`)
New `ArchivedGitWorktree` data model. Thread archiving now manages associated worktrees. Likely adds new fields to thread-related structs.

**Risk**: Medium. May affect `from_existing_thread()` constructor if `ConnectedServerState` gains new fields.

### 5. Language Model Refactor (`#53270`, `#52913`)
`language_models` crate refactored, deps extracted, decoupled from Zed-specific implementation details. New `cloud` module extraction.

**Risk**: Medium. Our `current_model_id()` fallback chain in `thread_view.rs` references `LanguageModelRegistry` — its API may have changed.

### 6. Subagent Permission Requests (`#53428`)
Surfaces subagent permission requests in the sidebar. New UI components in agent_panel.

**Risk**: Low. Additive change unlikely to conflict with Helix's cfg-gated code.

### 7. ACP Connection Changes (`#53396`)
Better handling of terminal auth on remote connections. May change `AgentConnection` trait or `connection.rs`.

**Risk**: High. If `AgentConnection` trait gained new methods, `HeadlessConnection` must implement them or the build will fail.

### 8. Notification Panel Removal (`#50204`)
The notification panel was removed entirely. If any Helix code referenced it, those references will break.

**Risk**: Low. Helix doesn't use the notification panel.

### 9. conversation_view.rs Changes (1,259 lines changed)
Max-width for thread view content (`#52730`), skip restoring external threads without metadata (`#52978`), slash commands restoration (`#53209`), fast mode persistence (`#53356`).

**Risk**: High. `conversation_view.rs` contains `HeadlessConnection`, `from_existing_thread()`, thread registry integration, and the `is_resume` flag — all Helix-critical.

### 10. acp_thread.rs Changes (622 lines changed)
Streaming tools fix for invalid JSON (`#52891`), crash fix on non-ASCII titles (`#53114`), repeated prompts fix (`#53216`).

**Risk**: Medium. Our `content_only()` method and `cancel()` drop fix must be preserved. Stopped tuple variant pattern may have changed.

## Conflict Prediction

Based on the 30+ items in the porting guide rebase checklist and the upstream diff stats, expect conflicts in these files (ordered by risk):

| File | Upstream Lines Changed | Helix Customizations | Risk |
|------|----------------------|---------------------|------|
| `agent_panel.rs` | 4,088 | 6 cfg-gated blocks | Critical |
| `conversation_view.rs` | 1,259 | HeadlessConnection, from_existing_thread, registry | Critical |
| `zed.rs` | 1,144 | WebSocket init (cfg-gated) | High |
| `acp_thread.rs` | 622 | content_only, cancel drop, Stopped tuple | High |
| `agent.rs` | 236 | load_session entity lifetime, multi-project | Medium |
| `Cargo.toml` (root) | 64 | workspace member + dep | Low |
| `workspace.rs` | Unknown (many commits) | Agent follow focus guard | High |
| `connection.rs` | Unknown | AgentConnection trait + HeadlessConnection | High |

## Post-Merge Validation

1. `cargo check --package zed --features external_websocket_sync`
2. `cargo test -p external_websocket_sync`
3. `cargo test -p acp_thread test_second_send`
4. Docker E2E test (all 10 phases, both agents)
5. Update `ZED_COMMIT` in `/home/retro/work/helix/sandbox-versions.txt`
6. Update `portingguide.md` with all new findings

## Learnings from Prior Merges

From task 001617 (most recent):
- **ACP Consolidation** renamed files and types — watch for further renames
- `from_existing_thread()` is the most structurally fragile integration point
- `ConnectedServerState` fields change between upstream versions — verify all required fields
- Feature propagation chain: `zed/Cargo.toml` → `agent_ui/Cargo.toml` → `title_bar/Cargo.toml` must all carry the feature flag

From task 001560:
- Streaming content accumulation race condition — the fix in `thread_service.rs` Stopped handler must be preserved
- EntryUpdated fires BEFORE buffer appends new chunk — any upstream changes to this event flow need careful verification

From task 001554:
- Upstream file modifications can go undocumented — catalog every conflict resolution in portingguide.md
- E2E test phases have grown over time (4 → 7 → 8 → 10) — if adding new scenarios, increment accordingly

## Implementation Notes (from this merge)

### Conflict Resolution Summary
- 25 files conflicted, all resolved across 3 commits after the merge commit
- `agent_panel.rs` was the most complex (5 conflicts), followed by `title_bar.rs` and `workspace.rs`
- Many files auto-merged cleanly (conversation_view.rs, acp_thread.rs, agent.rs, connection.rs)

### Key Discoveries
1. **Two ConnectedServerState types**: `AcpServerView` (in thread_view.rs) still has `login` and `history` fields; `ConversationView` (in conversation_view.rs) does not. Initially removed them from AcpServerView by mistake — had to restore.
2. **assistant_text_thread crate fully removed** by upstream #52757. The `History` enum, `TextThreadHistory`, `TextThreadHistoryEvent`, `SlashCommandWorkingSet`, `TextThreadStore::new()` — all dead code after the removal.
3. **AgentV2FeatureFlag fully removed** by upstream #52792. Code that used `cx.has_flag::<AgentV2FeatureFlag>()` must be simplified to remove the check.
4. **selected_agent_type → selected_agent** rename: Helix cfg-gated block in agent_panel.rs set `this.selected_agent_type = ...`, which silently compiles as a new local variable (Rust shadowing). Must use current field name.
5. **ContextServerStatus::AuthRequired** added: Match arms handling `Stopped | Error(_)` must also include `AuthRequired`.

### Gotchas
- **No Rust toolchain** in the merge environment — cannot run `cargo check/test`. All build verification requires CI or a different machine.
- **Origin is internal git server**, not GitHub — `gh` CLI doesn't work. PR creation must happen through the internal UI.
- **Upstream removed the notification panel entirely** — no impact on Helix code (we don't use it).
- **`should_render_onboarding` → `should_render_new_user_onboarding`** rename — the Helix cfg-gated onboarding dismissal calls this method and must track the current name.

### New Rebase Checklist Items Added (35-39)
Added to portingguide.md to watch for in future merges:
- 35: Check for removed upstream crates/imports
- 36: Check for removed upstream feature flags
- 37: Check agent_panel.rs field renames
- 38: Check context_server_registry.rs match arms for new variants
- 39: Check agent_panel.rs onboarding method renames

### Post-Merge Workflow
The merge is complete. Remaining steps:
1. CI must verify compilation and tests (pushed to `feature/001723-merge-latest-zed`)
2. PR created via internal git UI (description at `pull_request_zed.md`)
3. After PR merge: update `ZED_COMMIT` in helix repo's `sandbox-versions.txt` to new fork HEAD SHA
