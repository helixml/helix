# Upstream Merge Quick Reference Card

**For full analysis, see:** [2025-10-21-upstream-main-merge-analysis.md](./2025-10-21-upstream-main-merge-analysis.md)

## TL;DR

**What:** Merge 605 upstream commits into feature/external-thread-sync
**When:** Ready to proceed (estimated 6 hours)
**How:** Git merge (not rebase)
**Risk:** Medium (well-defined changes, clear migration path)

## Critical Breaking Changes

1. **agent2 → agent** (3 imports)
2. **workspace-hack removed** (2 lines)
3. **Lamport::new() signature changed** (1 call site)
4. **ReplicaId is now newtype** (affects clock usage)

## 5 Files to Fix

```bash
crates/external_websocket_sync/Cargo.toml              # 2 deletions
crates/external_websocket_sync/src/types.rs            # 2 replacements
crates/external_websocket_sync/src/thread_service.rs   # 1 replacement
crates/external_websocket_sync/src/external_websocket_sync.rs  # 1 fix + import
crates/agent_ui/src/agent_panel.rs                     # Merge conflicts
```

## Exact Fixes Required

### 1. Cargo.toml (delete 2 lines)

```diff
-agent2.workspace = true
-workspace-hack.workspace = true
```

### 2. types.rs (replace agent2 → agent)

```diff
-history: gpui::Entity<agent2::HistoryStore>,
+history: gpui::Entity<agent::HistoryStore>,

-Rc::new(agent2::NativeAgentServer::new(fs, history))
+Rc::new(agent::NativeAgentServer::new(fs, history))
```

### 3. thread_service.rs (replace import)

```diff
-use agent2::HistoryStore;
+use agent::HistoryStore;
```

### 4. external_websocket_sync.rs (fix Lamport call)

```diff
+use clock::ReplicaId;

-MessageId(clock::Lamport::new(1))
+MessageId(clock::Lamport::new(ReplicaId::new(1)))
```

### 5. agent_panel.rs (merge conflicts)

**Strategy:** Accept upstream changes, keep our feature-gated blocks

```rust
// Accept upstream:
use agent::{...};  // Not agent2::

// Keep ours:
#[cfg(feature = "external_websocket_sync")]
use external_websocket_sync_dep as external_websocket_sync;

#[cfg(feature = "external_websocket_sync")]
use tokio::sync::mpsc;

// Keep our callback registration code around line 730+
```

## Merge Commands

```bash
# 1. Backup
cd /home/luke/pm/zed
git branch feature/external-thread-sync-backup feature/external-thread-sync

# 2. Merge
git checkout feature/external-thread-sync
git merge upstream/main

# 3. Resolve conflicts (use fixes above)

# 4. Verify
cargo check -p external_websocket_sync --features external_websocket_sync
cargo build -p zed --features external_websocket_sync
./stack build-zed

# 5. Test
# - External agent thread creation
# - Bidirectional sync (Helix ↔ Zed)
# - message_completed events

# 6. Commit (use template from full analysis doc)
git add -A
git commit -m "Merge upstream/main into feature/external-thread-sync..."
```

## Rollback Plan

```bash
# If merge fails:
git merge --abort

# If compilation fails after merge:
git reset --hard feature/external-thread-sync-backup

# If integration tests fail:
# Debug, fix, or reset and try again
```

## Success Criteria

- ✅ No compiler errors
- ✅ No new warnings
- ✅ External agents create successfully
- ✅ WebSocket sync works bidirectionally
- ✅ No agent2 references remain
- ✅ No workspace-hack references remain

## Key Upstream Commits

| What | Commit | PR |
|------|--------|-----|
| agent2 → agent | 3f1319162a | #40495 |
| Remove workspace-hack | ef5b8c6fed | #40216 |
| ReplicaId newtype | 43a9368dff | #40600 |
| ACP 0.4.3 → 0.5.0 | 85c2aa7325 | #40701 |

## Timeline

- **Day 1:** Merge + resolve conflicts (4 hours)
- **Day 2:** Test + fix issues (2 hours)
- **Total:** ~6 hours over 1-2 days

## Questions Before Starting?

1. Is feature/external-thread-sync up to date with latest local work? ✅
2. Have we committed all changes? ✅
3. Do we have a backup? (Will create)
4. Do we have time to test thoroughly? (Plan for it)

---

**Ready to proceed?** Follow the steps in the full analysis document.
