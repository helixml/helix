# Design: Merge Latest Zed Upstream

## Repository Layout

- **Helix fork of Zed**: `/home/retro/work/zed/` (origin: helix internal git)
- **Upstream**: `zed-industries/zed` on GitHub (added as `upstream` remote)
- **Porting guide**: `/home/retro/work/zed/portingguide.md` (canonical reference — update during merge)
- **Helix platform repo**: `/home/retro/work/helix/` (contains `sandbox-versions.txt` with `ZED_COMMIT=`)

## Current State

| | Commit | Date |
|---|---|---|
| Fork HEAD | `1e07aea47a` (PR #41: auto-approve ACP permissions) | Apr 15, 2026 |
| Last upstream merge | `1f96cb8812` merging upstream `8b822f9e10` | Mar 23, 2026 |
| Upstream HEAD | `d4163c7031` (#54519: command palette fix) | Apr 22, 2026 |
| Helix-specific commits since last merge | 89 (PRs #29–#41) | |
| Upstream commits to merge | **~920** | 30 days |

## Merge Strategy

Use `git merge upstream/main` (not rebase), consistent with all previous merges. This preserves Helix commit history and makes conflict resolution traceable.

```bash
cd /home/retro/work/zed
git remote add upstream https://github.com/zed-industries/zed.git
git fetch upstream
git checkout -b feature/001885-merge-latest-zed
git merge upstream/main
# Resolve conflicts
# Verify build + tests
# PR to fork main
```

## Critical Upstream Breaking Changes (March 23 – April 22, 2026)

### 1. `AgentConnection::prompt` — Signature Change (PR #53850) — **CRITICAL**

`user_message_id` changed from `Option<UserMessageId>` to required `UserMessageId`.

**Impact on fork**: `HeadlessConnection` in `conversation_view.rs` implements this trait. Its `prompt` method signature must be updated. The `send()` call site in `acp_thread.rs` now unconditionally generates a `UserMessageId`.

**Action**: Update `HeadlessConnection::prompt` signature. Since headless connections never actually call prompt (it's a no-op), just accept the new required parameter.

### 2. Session Ref-Counting in `NativeAgent` (PR #53999) — **CRITICAL**

`load_session()` in `agent.rs` was completely rewritten with:
- `ref_count` field on `Session` struct
- New `PendingSession` struct with `task: Shared<Task<...>>`
- `pending_sessions: HashMap<acp::SessionId, PendingSession>` on `NativeAgent`
- `register_session()` now takes `ref_count` parameter
- Extracted `close_session()` method

**Impact on fork**: Critical Fix #1 (entity lifetime in `load_session`) must be re-applied within the new ref-counting structure. The fix principle is the same — keep a strong `Entity<NativeAgent>` reference alive during the async task — but the surrounding code has changed significantly.

**Action**: After merge conflict resolution, verify that `load_session()` still clones the entity before the async task. The shared-task deduplication may actually help (multiple callers share one task), but the entity lifetime issue remains.

### 3. Worktree Picker Moved Out of Agent Panel (PR #54183) — **CRITICAL**

Massive 30+ file refactor:
- `thread_worktree_picker.rs` deleted from `agent_ui`
- `agent_panel.rs` rewritten (+802/-2451 lines)
- Worktree creation moved to `git_ui` crate
- Keymaps changed (removed `agent::ToggleWorktreeSelector`)

**Impact on fork**: `agent_panel.rs` is the most conflict-prone file in the fork. The Helix cfg-gated blocks (thread display callback, UI state query, onboarding dismissal, split-brain detection, auto-follow) must be re-applied within the new structure. The worktree picker itself is irrelevant to Helix, but the surrounding code reorganization will cause extensive conflicts.

**Action**: Expect 5-10+ conflicts in `agent_panel.rs`. Resolve by accepting upstream structure changes and re-inserting Helix cfg-gated blocks in the correct new locations.

### 4. Draft/Background Threads Unified (PR #53737) — **HIGH**

- `DraftId`/`DraftIdCounter` removed entirely
- `draft_threads` + `background_threads` merged into `retained_threads: HashMap<ThreadId, Entity<ConversationView>>`
- `create_draft()` → `create_thread()`, `activate_draft()` → `activate_retained_thread()`
- `ActiveView::AgentThread` gains `thread_id: ThreadId` field

**Impact on fork**: Helix code that references `ActiveView::AgentThread` must include the new `thread_id` field in pattern matches. The `from_existing_thread()` integration may need to work with `retained_threads` instead of the old draft system.

**Action**: Update all `ActiveView::AgentThread` match arms. Check if `from_existing_thread()` needs to register in `retained_threads`.

### 5. Thread Generation Direction — Top-Down (PR #52440) — **MODERATE**

`ListAlignment::Bottom` → `ListAlignment::Top` with new `follow_tail` feature.

**Impact on fork**: Mostly transparent to Helix integration. The WebSocket sync layer doesn't depend on scroll direction. But `conversation_view.rs` modifications may conflict around the list initialization code.

**Action**: Accept upstream change. Verify no Helix-specific code depends on bottom-up scroll behavior.

### 6. AcpThread Slash Commands Persistence (PR #53209) — **MODERATE**

`available_commands` persisted on `AcpThread`, `SessionCapabilities` restored from thread state.

**Impact on fork**: Minor. Helix headless threads don't use slash commands. But the struct changes may cause merge conflicts in `acp_thread.rs`.

**Action**: Accept upstream change. Verify `content_only()` method (Critical Fix #3) still applies cleanly.

## High-Risk File Conflict Map

Based on previous merges and upstream changes, ordered by expected conflict severity:

| File | Risk | Upstream PRs | Helix Changes |
|------|------|-------------|---------------|
| `agent_panel.rs` | **CRITICAL** | #53737, #54183, #52979, #54387, #54005, #53384, #53352, #53334, #53324 (9+ PRs) | Thread display callback, UI state query, onboarding dismissal, split-brain detection, auto-follow, `acp_history_store()` |
| `conversation_view.rs` | **HIGH** | #52440, #54439, #52545 | `HeadlessConnection`, `from_existing_thread()`, THREAD_REGISTRY, history refresh, `is_resume` flag, thread unregistration |
| `agent.rs` | **HIGH** | #53999 (session ref-counting) | Critical Fix #1 (entity lifetime), `wait_for_tools_ready` |
| `connection.rs` | **HIGH** | #53850 (prompt signature) | `wait_for_tools_ready()` trait method |
| `acp_thread.rs` | **MODERATE** | #52724, #52196, #53209 | `content_only()`, `cancel()` drop fix, `stopped_emitted` guard, `Stopped(StopReason)` tuple |
| `workspace.rs` | **MODERATE** | #54439 | Agent follow focus guard |
| `Cargo.toml` files | **MODERATE** | Multiple | Feature flag propagation chain |
| `title_bar/` | **LOW** | #54183 (worktree picker move) | Connection status indicator |

## Key Patterns from Previous Merges

### Two `ConnectedServerState` Types
`AcpServerView` (in `thread_view.rs`) has `login` and `history` fields. `ConversationView` does NOT. Easy to mix up during conflict resolution.

### Feature Propagation Chain
Must be maintained: `zed/Cargo.toml` → `agent_ui/Cargo.toml` → `title_bar/Cargo.toml`. All three must propagate `external_websocket_sync`.

### from_existing_thread Fragility
The `from_existing_thread()` constructor must match `ConnectedServerState`'s current fields exactly. Every upstream change to this struct requires updating the constructor. Check fields: `connection`, `auth_state`, `active_id`, `threads`, `conversation`, `history`, `_connection_entry_subscription`.

### Auto-Merged Files Still Need Validation
Previous merge (001723): 25 conflicted files, but auto-merged files like `conversation_view.rs` and `acp_thread.rs` still needed manual verification via the rebase checklist.

### Smaller Merges = Less Risk
This merge covers 30 days / 920 commits. The 001723 merge (506 commits) took 3 commits to resolve after initial merge. This will likely be more complex. Consider whether the merge can be staged (merge to an intermediate upstream commit first, then to HEAD) — but this adds complexity and previous specs have not used this approach.

## Post-Merge Validation

### 1. Compile Check (fast)
```bash
cargo check --package zed --features external_websocket_sync
```

### 2. Grep Verification of Critical Fixes
```bash
# Fix 1: entity lifetime
grep -n "load_session" crates/agent/src/agent.rs | grep "clone()"
# Fix 3: content_only
grep -n "content_only" crates/acp_thread/src/acp_thread.rs
# Fix 8: drop not await
grep -n "drop(turn.send_task)" crates/acp_thread/src/acp_thread.rs
# Fix 9: stopped guard
grep -n "stopped_emitted_for_task" crates/acp_thread/src/acp_thread.rs
```

### 3. Unit Tests
```bash
cargo test -p external_websocket_sync
cargo test -p acp_thread test_second_send
```

### 4. E2E Test (requires fresh binary + Docker + ANTHROPIC_API_KEY)
```bash
cd crates/external_websocket_sync/e2e-test
# Build fresh Zed binary first
cargo build --features external_websocket_sync -p zed --release
cp target/release/zed zed-binary
./run_docker_e2e.sh                              # zed-agent only
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh  # both agents
```

### 5. Full Rebase Checklist
Walk through all 33+ items in `portingguide.md` rebase checklist section.

## Documentation Updates Required

After merge, update `portingguide.md` with:
1. New upstream API changes discovered (prompt signature, session ref-counting, etc.)
2. Any new `ConnectedServerState` fields
3. New file renames or module restructuring
4. Updated rebase checklist items for patterns discovered during this merge
5. Updated commit history table with new Helix commits
6. Any new `AgentConnection` trait methods that `HeadlessConnection` must implement
