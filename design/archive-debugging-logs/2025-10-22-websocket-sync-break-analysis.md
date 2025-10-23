# WebSocket Sync Breakage Analysis - 2025-10-22

## Expected Code Flow (Working Branch: feature/external-thread-sync-backup)

### 1. Workspace Initialization
```
zed.rs:initialize_workspace()
  └─> Creates AgentPanel
      └─> AgentPanel has: acp_history_store: Entity<agent2::HistoryStore>
  └─> Calls external_websocket_sync::setup_thread_handler(
        project,
        agent_panel.read(cx).acp_history_store().clone(),  // Returns Entity<agent2::HistoryStore>
        fs,
        cx
      )
```

### 2. Thread Handler Setup
```
thread_service.rs:setup_thread_handler(
  project: Entity<Project>,
  acp_history_store: Entity<HistoryStore>,  // Expected type: agent2::HistoryStore
  fs: Arc<dyn Fs>,
  cx: &mut App
)
  └─> Registers thread creation callback
  └─> Spawns handler task waiting for thread creation requests
```

### 3. WebSocket Message Flow
```
WebSocket receives message from Helix
  └─> Creates ThreadCreationRequest
  └─> Sends request via callback channel
  └─> Handler task receives request
  └─> create_new_thread_sync() called
      └─> Uses acp_history_store to create NativeAgentServer
      └─> Creates AcpThread with server connection
      └─> Thread sends/receives messages
```

## Changes Between Branches (Merge from main)

### Working Branch (feature/external-thread-sync-backup)
```rust
// agent_panel.rs
pub struct AgentPanel {
    // ...
    acp_history_store: Entity<agent2::HistoryStore>,  // Separate field for ACP
    history_store: Entity<HistoryStore>,              // Different field for UI
    // ...
}

#[cfg(feature = "external_websocket_sync")]
pub fn acp_history_store(&self) -> &Entity<agent2::HistoryStore> {
    &self.acp_history_store  // Returns the ACP-specific store
}

// thread_service.rs
use agent2::HistoryStore;  // Uses agent2 module
```

### Broken Branch (feature/external-thread-sync)
```rust
// agent_panel.rs
pub struct AgentPanel {
    // ...
    history_store: Entity<agent::HistoryStore>,  // ONLY ONE field, wrong type!
    // acp_history_store field REMOVED!
    // ...
}

#[cfg(feature = "external_websocket_sync")]
pub fn acp_history_store(&self) -> &Entity<agent::HistoryStore> {
    &self.history_store  // Returns wrong type!
}

// thread_service.rs
use agent::HistoryStore;  // Uses agent module (DIFFERENT TYPE)
```

## Root Cause Analysis

### Theory 1: Type Mismatch (MOST LIKELY)
**Hypothesis**: `agent2::HistoryStore` and `agent::HistoryStore` are incompatible types with different APIs.

**Evidence**:
- `agent2::HistoryStore` has `HistoryEntry::AcpThread(DbThreadMetadata)`
- `agent::HistoryStore` has `HistoryEntry::Thread(SerializedThreadMetadata)`
- Different field names and struct layouts

**Impact**: Even though both are called "HistoryStore", they serve different purposes:
- `agent2::HistoryStore` - Manages ACP threads (new architecture)
- `agent::HistoryStore` - Manages text threads (old architecture)

**Result**: WebSocket integration receives wrong store type, cannot create ACP threads properly.

### Theory 2: Missing Field Reference
**Hypothesis**: Compilation fix incorrectly merged two separate fields into one.

**Evidence**:
- Working branch has BOTH `acp_history_store` and `history_store` fields
- Broken branch only has `history_store` field
- The method was changed to return the wrong field

**Impact**: UI code that relies on `agent::HistoryStore` gets the right store, but WebSocket code that needs `agent2::HistoryStore` gets the wrong one.

### Theory 3: Upstream Rename Breaking Integration
**Hypothesis**: Upstream Zed renamed/refactored agent modules, breaking our integration assumptions.

**Evidence**:
- Both `agent` and `agent2` crates exist side-by-side
- Merge introduced changes from upstream that assume different module structure

**Impact**: Our WebSocket code was written for the old architecture but now runs against the new one.

## Fix Strategy

### Option 1: Restore Dual Fields (RECOMMENDED)
Restore the struct to have both fields:
```rust
pub struct AgentPanel {
    // ...
    acp_history_store: Entity<agent2::HistoryStore>,  // For WebSocket sync (ACP)
    history_store: Entity<agent::HistoryStore>,        // For UI (text threads)
    // ...
}
```

### Option 2: Update WebSocket Integration
Update all WebSocket code to use `agent::HistoryStore` and adapt to the new API.
- More risky, requires understanding all API differences
- May break existing functionality

### Option 3: Revert Merge
Revert the merge and stay on the working branch until upstream stabilizes.
- Safest but prevents using new upstream features
- Not a long-term solution

## Testing Plan

1. Implement Option 1 (restore dual fields)
2. Ensure thread_service.rs uses `agent2::HistoryStore`
3. Build Zed with fix
4. Test WebSocket connection establishment
5. Test message "hi" delivery to Zed
6. Test response streaming back from Zed
7. Verify in browser console for `message_completed` events
