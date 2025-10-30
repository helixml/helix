# WebSocket Sync Fix - Complete Summary

## Problem Statement
After merging upstream `main` into `feature/external-thread-sync`, WebSocket sync was completely broken:
- Zed didn't connect to API server via WebSocket
- Messages weren't sent to Zed
- No responses streamed back from Zed

## Root Cause Analysis

### What Upstream Did
The upstream merge made a **major architectural change**:
- **Renamed** `agent2` crate → `agent` crate
- **Removed/renamed** old `agent` crate (text threads)
- **Deleted** the entire `agent2` crate from the codebase

### What The Compilation Fix Did Wrong
The compilation fix commit (`3dced7bcdf`) tried to adapt to the new architecture but made **critical mistakes**:

1. **Removed the separate `acp_history_store` field**
   - **Before (working)**: `acp_history_store: Entity<agent2::HistoryStore>`
   - **After (broken)**: Field deleted, only `history_store: Entity<agent::HistoryStore>` remained

2. **Changed method to return wrong field**
   - **Before**: `acp_history_store()` → returns `&self.acp_history_store`
   - **After**: `acp_history_store()` → returns `&self.history_store`

3. **Type confusion**
   - The compilation fix assumed old `agent::HistoryStore` = new `agent::HistoryStore`
   - But new `agent::HistoryStore` is actually old `agent2::HistoryStore`!

## The Fix

### Key Insight
The new `agent` crate (after merge) is functionally equivalent to the old `agent2` crate. They have:
- Identical `DbThreadMetadata` struct
- Same `NativeAgentServer` implementation
- Same `HistoryStore` for ACP thread management

### Changes Made

#### 1. Restored the Separate Field
```rust
// agent_panel.rs - AgentPanel struct
acp_history_store: Entity<agent::HistoryStore>,  // Added back (using new agent:: which = old agent2::)
```

#### 2. Fixed Method to Return Correct Field
```rust
#[cfg(feature = "external_websocket_sync")]
pub fn acp_history_store(&self) -> &Entity<agent::HistoryStore> {
    &self.acp_history_store  // Fixed to return correct field
}
```

#### 3. Created Store in Initialization
```rust
// In AgentPanel::new()
let acp_history_store = cx.new(|cx| agent::HistoryStore::new(text_thread_store.clone(), cx));
let acp_history = cx.new(|cx| AcpThreadHistory::new(acp_history_store.clone(), window, cx));
```

#### 4. Updated All References
Changed ~10 instances of `self.history_store` → `self.acp_history_store` throughout `agent_panel.rs`

#### 5. Fixed WebSocket Integration Imports
```rust
// thread_service.rs
use agent::HistoryStore;  // Now refers to ACP history store (old agent2::)

// types.rs
impl ExternalAgent {
    pub fn server(&self, ..., history: Entity<agent::HistoryStore>) -> ... {
        Self::NativeAgent => Rc::new(agent::NativeAgentServer::new(fs, history)),
    }
}
```

## Files Modified

1. **crates/agent_ui/src/agent_panel.rs**
   - Restored `acp_history_store` field
   - Fixed `acp_history_store()` method
   - Updated store initialization
   - Changed all `history_store` refs to `acp_history_store`

2. **crates/external_websocket_sync/src/thread_service.rs**
   - Import from `agent::` instead of `agent2::`

3. **crates/external_websocket_sync/src/types.rs**
   - Updated `ExternalAgent::server()` signature

## Testing Plan

### 1. Create External Agent Session
```bash
# Frontend running at localhost:8080
# API key: hl-M-hfmHiCZQsdnX2iz7xBnjNbmNyXzrtLsr8ZrTJXBXc=
# App ID: app_01k63mw4p0ezkgpt1hsp3reag4
```

1. Navigate to External Agents section
2. Click "Start External Agent Session"
3. Send a message "hi" to trigger thread creation

### 2. Verify WebSocket Sync
Expected behavior:
- ✅ Zed connects via WebSocket to API server
- ✅ Message "hi" appears in Zed
- ✅ AI response streams back to Helix
- ✅ `message_completed` WebSocket event fires

### 3. Check Logs
```bash
# API logs should show WebSocket connection
docker compose -f docker-compose.dev.yaml logs --tail 50 api | grep -i websocket

# Browser console should show message_completed events
# Check Network tab → WS connection → Messages
```

## Build Status

✅ **Zed built successfully** (1.3GB binary)
```bash
./stack build-zed  # Completed in ~26 seconds
```

✅ **Sway image rebuilt** with fixed Zed
```bash
./stack build-sway  # Image: helix-sway:latest
```

✅ **Ready to test** - New external agent sessions will use the fixed code

## Why This Works

The key realization was that **upstream consolidated two codebases**:
- Old `agent` (text threads) → removed/renamed
- Old `agent2` (ACP threads) → renamed to `agent`

The compilation fix tried to adapt but **lost the distinction** between the ACP history store and the UI history store. By restoring the separate `acp_history_store` field (now using the new `agent::` namespace), we maintain the separation while using the correct types from the consolidated codebase.

## Next Steps

1. **Test**: Create external agent session and verify message flow
2. **If working**: Commit the fixes to `feature/external-thread-sync`
3. **If issues**: Check logs and debug based on specific error
