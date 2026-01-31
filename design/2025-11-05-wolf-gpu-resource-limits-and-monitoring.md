# Wolf GPU Resource Limits and Monitoring Failures

**Date:** 2025-11-05
**Status:** Critical Discovery - Implemented Fixes
**Context:** Resource exhaustion investigation during startup script testing

## Critical Discovery

### Timeline of Resource Exhaustion

**12:28:18** - Last successful nvidia-smi query (2 NVENC sessions)
**12:28-12:30** - NVML stops working inside Wolf container
**12:28-12:35** - 24-minute gap with NO nvidia-smi logs
**12:35:00** - Attempt to create 7th lobby triggers catastrophic failure:
- `GL_OUT_OF_MEMORY` - Failed to allocate memory for buffer object
- `CUDA_ERROR_NOT_PERMITTED` - Operation not permitted
- `Failed to create EGLImage from DMA-BUF`
- Container crashes with exit code 1
- Moonlight-web returns error -102 (ML_ERROR_UNEXPECTED_EARLY_TERMINATION)

**12:52:15** - nvidia-smi works again after Wolf restart

### Resource State at Failure (12:35:00)

From Helix monitoring logs:
```
📊 Wolf Resource Monitoring
- active_lobbies: 6
- active_clients: 0 (no streaming connections)
- wolf_process_rss_mb: 1461
- wolf_gstreamer_buffer_mb: 434
- wolf_total_memory_mb: 1472

🎮 GPU Metrics (ALL ZEROS - MONITORING WAS BROKEN!)
- gpu_name: "" ← Should be "NVIDIA RTX 2000 Ada Generation"
- encoder_sessions: 0 ← Actually had sessions!
- gpu_utilization_pct: 0
- memory_used_mb: 0
- memory_total_mb: 0
- temperature_c: 0

🎬 GStreamer Pipelines
- producer_pipelines: 12 (6 lobbies × 2)
- consumer_pipelines: 0
- total_pipelines: 12
```

**Key Insight:** GPU metrics were all zeros because nvidia-smi had already failed 7 minutes earlier!

From host nvidia-smi at 12:41:
```
GPU Memory: 6720 MiB / 16380 MiB (41% used)
6 Sway processes (38 MB each)
6 Zed processes (228-354 MB each)
3 Ghostty processes (104 MB each)
1 Wolf process (1399 MB)
```

## Root Cause Analysis

### The Resource Limit Cascade

1. **NVML breaks first** (~5-6 lobbies, around 12:28-12:30)
   - nvidia-smi fails with "Failed to initialize NVML: Unknown Error"
   - GPU monitoring goes dark (all metrics return 0)
   - Wolf caches the failed state for 2 seconds, but failures persist

2. **We lose visibility** (12:28-12:35)
   - Helix monitoring shows zeros for all GPU metrics
   - No warning that we're approaching limits
   - System appears to have plenty of capacity

3. **GPU exhaustion** (7th lobby at 12:35:00)
   - OpenGL can't allocate buffer memory
   - CUDA refuses to create new context
   - Wayland compositor fails to initialize
   - Container crashes immediately (exit code 1)

### Why NVML Fails Before GPU Memory Exhaustion

NVML (NVIDIA Management Library) has its own resource limits separate from GPU memory:
- **NVML handles** - limited number of concurrent NVML connections
- **CUDA contexts** - each lobby creates CUDA context for encoding
- **Driver resources** - shared state in nvidia driver gets exhausted

**The actual safe limit appears to be ~5 lobbies**, not 6-7. NVML breaks first as an early warning, but we weren't detecting it.

## Bugs Fixed

### 1. Cleanup Not Working for Deleted Sessions

**Problem:** When you delete a project, sessions are hard-deleted from database, but:
- Wolf lobbies keep running
- Containers stay active
- GPU resources stay consumed
- Cleanup loop fails with "session not found" but logs "terminated successfully" (false!)

**Logs showing the bug:**
```
12:32:43 - Stopping Zed agent via Wolf session_id=ses_01k99zs5qnv8a958bthja4va6p
12:32:43 - Failed to stop idle Zed agent error="session not found in database"
12:32:43 - External agent terminated successfully ← FALSE! Container still running!
```

**Fixes implemented:**

1. **Added lobby credentials to activity tracking** (`api/pkg/types/simple_spec_task.go`):
```go
type ExternalAgentActivity struct {
    // ... existing fields ...
    WolfLobbyID     string `json:"wolf_lobby_id" gorm:"size:255"` // NEW
    WolfLobbyPIN    string `json:"wolf_lobby_pin" gorm:"size:4"` // NEW
}
```

2. **Store credentials when creating session** (`api/pkg/external-agent/wolf_executor.go:792`):
```go
err = w.store.UpsertExternalAgentActivity(ctx, &types.ExternalAgentActivity{
    // ... other fields ...
    WolfLobbyID:     response.WolfLobbyID,   // Store for cleanup
    WolfLobbyPIN:    response.WolfLobbyPIN,  // Store for cleanup
})
```

3. **Cleanup uses activity record as fallback** (`api/pkg/external-agent/wolf_executor.go:1818`):
```go
// If session not found in database, use lobby ID/PIN from activity record
if stopError != nil && strings.Contains(stopError.Error(), "not found in database") && activity.WolfLobbyID != "" {
    log.Info().Msg("Session deleted from database, stopping lobby using activity record credentials")
    // Stop lobby directly using Wolf API with stored credentials
    stopReq := &wolf.StopLobbyRequest{
        LobbyID: activity.WolfLobbyID,
        PIN:     lobbyPIN,
    }
    err := w.wolfClient.StopLobby(ctx, stopReq)
    // ...
}
```

4. **Added soft delete support** (`api/pkg/types/types.go:687`):
```go
type Session struct {
    // ... existing fields ...
    DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"` // NEW
}
```

5. **Cleanup can find soft-deleted sessions** (`api/pkg/store/store_sessions.go:104`):
```go
// GetSessionIncludingDeleted retrieves a session including soft-deleted ones
func (s *PostgresStore) GetSessionIncludingDeleted(ctx context.Context, sessionID string) (*types.Session, error) {
    var session types.Session
    err := s.gdb.WithContext(ctx).Unscoped().Where("id = ?", sessionID).First(&session).Error
    // ...
}
```

6. **StopZedAgent uses GetSessionIncludingDeleted** (`api/pkg/external-agent/wolf_executor.go:878`):
```go
dbSession, err := w.store.GetSessionIncludingDeleted(ctx, sessionID)
```

### 2. GPU Monitoring Goes Dark Without Detection

**Problem:** When NVML fails, GPU stats return all zeros, but we don't detect this and log warnings.

**Fix needed:** Add loud logging when GPU stats are unavailable or suspiciously zero.

### 3. No Hard Limit on Concurrent Lobbies

**Problem:** System keeps creating lobbies until GPU crashes, no proactive limit checking.

**Fix needed:** Enforce 5 lobby hard limit in Helix API before creating new sessions.

## Implementation Plan

### High Priority (Do Now)

1. ✅ **Fix cleanup for deleted sessions** - DONE (belt and braces approach)
   - Activity table stores lobby credentials
   - Soft delete support for sessions
   - Cleanup uses both session DB and activity fallback

2. **Log loudly when nvidia-smi fails** - Add to Helix monitoring loop:
```go
if gpu_stats.gpu_name == "" || gpu_stats.memory_total_mb == 0 {
    log.Error().Msg("⚠️ GPU MONITORING FAILED - nvidia-smi not returning data! Resource limits may be exceeded!")
}
```

3. **Enforce 5 lobby hard limit** - Before creating new lobby:
```go
activeLobbies, err := w.wolfClient.ListLobbies(ctx)
if len(activeLobbies) >= 5 {
    return nil, fmt.Errorf("GPU resource limit reached (5/5 lobbies active). Please close an unused session and try again.")
}
```

4. **Return clear error to frontend** - Show error with link to sessions list

### Medium Priority

5. **Verify wolf-ui session cleanup** - Ensure streaming sessions don't leak
6. **Add lobby age tracking** - Log which lobbies are oldest for manual cleanup
7. **Dashboard warning UI** - Show warning banner when at 4/5 lobbies

### Low Priority

8. **NVML health checks** - Periodically verify nvidia-smi works, restart Wolf if stuck
9. **Graceful degradation** - Fall back to container count if nvidia-smi unavailable
10. **Auto-cleanup oldest idle lobby** - Only as last resort, with clear warning

## Resource Limits Discovered

| Resource | Safe Limit | Failure Point | Detection |
|----------|------------|---------------|-----------|
| Concurrent Lobbies | **5** | 6-7 | NVML fails first |
| NVML Handles | Unknown | ~5-6 lobbies | nvidia-smi errors |
| CUDA Contexts | Unknown | ~6-7 lobbies | CUDA_ERROR_NOT_PERMITTED |
| GPU Memory | 16380 MB total | ~6720 MB at 6 lobbies | nvidia-smi (when working) |
| GStreamer Pipelines | 12 (6 lobbies × 2) | Unknown | Wolf tracks directly |

## Monitoring Improvements Needed

1. **Detect when monitoring itself is broken** - nvidia-smi failures
2. **Log monitoring health** - "nvidia-smi working: true/false"
3. **Alert on suspiciously zero metrics** - gpu_name empty = monitoring dead
4. **Fallback metrics** - Use Docker stats, process counts when nvidia-smi fails

## Lessons Learned

1. **Monitoring can fail before the system** - nvidia-smi died 7 min before GPU crash
2. **Zero metrics mean broken monitoring** - not "system is idle"
3. **Resource limits are complex** - NVML/CUDA/GL have separate limits
4. **Fail fast is critical** - Should error at 5 lobbies, not wait for crash at 7
5. **Observability needs observability** - Monitor the monitors!

## Questions for Future Investigation

1. Why does NVML fail at 5-6 lobbies specifically?
2. Can we increase NVML handle limit via nvidia driver settings?
3. Do wolf-ui streaming sessions (consumers) count toward CUDA context limit?
4. Would software encoding bypass these limits? (Trade performance for capacity)
5. Can we pool CUDA contexts across lobbies?

## References

- Wolf container startup failure: `docker events` at 12:35:01-12:35:02
- Wolf logs: GL_OUT_OF_MEMORY, CUDA_ERROR_NOT_PERMITTED at 12:35:00
- Helix monitoring: All zero GPU stats at 12:35:00-12:35:43
- nvidia-smi gap: 12:28:18 → 12:52:15 (24 minutes)
- Error code: -102 (ML_ERROR_UNEXPECTED_EARLY_TERMINATION)
