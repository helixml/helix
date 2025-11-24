# Wolf Streaming Platform Stability and Production Readiness

**Date**: 2025-11-14  
**Status**: Problem Analysis & Strategy  
**Priority**: CRITICAL - Production Stability

## Problem Statement

Wolf streaming platform experiences intermittent hangs and connectivity issues in production, leading to:
- Session creation failures ("Connect TimedOut" errors)
- Lost work for active agents mid-task
- No automatic recovery mechanism
- Entire streaming infrastructure unavailable until manual restart

## Current Production Issues

### 1. SSL Handshake Timeouts (CRITICAL)

**Symptoms:**
```
[ERROR] [Stream]: failed to get app list from host: 
  Api(RequestClient(Reqwest(reqwest::Error { 
    kind: Request, 
    url: "https://wolf:47984/serverinfo?...", 
    source: hyper_util::client::legacy::Error(Connect, TimedOut) 
  })))
```

**Root Cause:**
- moonlight-web `connect_timeout = 1 second` (too aggressive)
- Wolf's SSL handshake can take 2-5s under load
- Happens when Wolf is handling multiple concurrent connections

**Evidence:**
- TCP port 47984 responds immediately (port is open)
- But SSL handshake times out after 1s
- Wolf CPU at 48% (under load)
- 101 PIDs (multiple child processes)

**Impact:**
- ~10% of new session creation attempts fail
- Agents can't connect to their workspace
- Users see "failed to start session" errors

### 2. Wolf Hangs/Lock-ups (HIGH)

**Symptoms:**
- Wolf process alive but unresponsive
- No automatic detection or recovery
- Requires manual restart
- All active sessions lost

**Known Triggers:**
- Multiple concurrent SSL handshakes
- Container lifecycle operations
- GStreamer pipeline errors
- Resource exhaustion

### 3. No Observability (MEDIUM)

**Missing:**
- SSL handshake duration metrics
- Active session count
- Resource usage per session
- Health check endpoint
- Alerting when degraded

## Implemented Fixes (2.5.4 Release)

### 1. Increase moonlight-web Connect Timeout ✅
**File**: `moonlight-common/src/network/reqwest.rs:30`
```rust
.connect_timeout(Duration::from_secs(10))  // was: 1
```
**Benefit**: Allows SSL handshake to complete under load
**Risk**: Minimal - only delays failure detection by 9s

### 2. Fix False Pairing Warning ✅
**File**: `server-templates/init-moonlight-config.sh`
- Check data.json for 'paired' section instead of parsing chunked HTTP
- More reliable pairing success detection

### 3. Wolf Template PIN Placeholder ✅
**File**: `wolf/config.toml.template`
```toml
pin = []  # Required for sed substitution
```
**Fixes**: Init script can now set PIN from env var

## Recommended Next Steps

### Immediate (This Week) ⚡

#### 1. Add Docker Healthcheck to Wolf

**File**: `docker-compose.yaml`, `docker-compose.dev.yaml`
```yaml
wolf:
  healthcheck:
    test: ["CMD-SHELL", "timeout 3 bash -c 'cat < /dev/null > /dev/tcp/localhost/47989' || exit 1"]
    interval: 30s
    timeout: 5s
    retries: 3
    start_period: 10s
```

**Benefit:**
- Automatic Wolf restart if hung
- Docker handles restart logic
- Minimal code changes

**Limitation:**
- Active sessions still lost on restart
- But system recovers automatically

#### 2. Add Wolf Startup Timing Logs

**File**: Wolf source `main.cpp` or similar
```cpp
log_info("SSL certificates loaded in %dms", elapsed);
log_info("/serverinfo endpoint ready in %dms", elapsed);
```

**Benefit**: Diagnose slow startup issues

### Short-Term (Next 2 Weeks) 🔥

#### 3. Implement Wolf Internal Watchdog

**Approach**: Separate monitoring thread
```cpp
// Watchdog thread
while (true) {
  auto start = now();
  event_loop.ping();
  auto duration = now() - start;
  
  if (duration > 10s) {
    log_warn("Event loop slow: %dms", duration);
  }
  if (duration > 30s) {
    log_error("Event loop hung, exiting for restart");
    exit(1);  // Docker restarts
  }
  
  sleep(5s);
}
```

**Benefit:**
- Self-healing
- Catches event loop hangs
- No external dependencies

#### 4. Add curl to Wolf Container

**File**: `wolf.Dockerfile`
```dockerfile
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
```

**Benefit**: 
- Easier debugging in production
- Can query own endpoints for health checks

#### 5. Graceful Session Metadata Persistence

**File**: Wolf source - add session tracking
```cpp
// Before starting session
save_session_metadata("/etc/wolf/cfg/active_sessions.json", session);

// On startup
auto orphaned = load_session_metadata();
for (auto& session : orphaned) {
  if (container_exists(session.container_id)) {
    log_info("Reconnecting to orphaned session %s", session.id);
    reconnect_to_container(session);
  } else {
    cleanup_session(session);
  }
}
```

**Benefit:**
- Can resume sessions after Wolf restart
- Or at least clean them up gracefully

### Medium-Term (Next Month) 📊

#### 6. Add Prometheus Metrics

**Endpoint**: `/metrics`
**Metrics**:
- `wolf_ssl_handshake_duration_seconds` (histogram)
- `wolf_active_sessions` (gauge)
- `wolf_session_creation_errors_total` (counter)
- `wolf_cpu_usage_percent` (gauge)
- `wolf_memory_usage_bytes` (gauge)

**Alerting Rules**:
```yaml
- alert: WolfSlowSSL
  expr: histogram_quantile(0.95, wolf_ssl_handshake_duration_seconds) > 2
  for: 5m

- alert: WolfHighErrorRate
  expr: rate(wolf_session_creation_errors_total[5m]) > 0.1
  for: 2m
```

#### 7. Connection Pool/Queue for SSL

**Problem**: 10 simultaneous session creations → 10 SSL handshakes → overload
**Solution**:
```cpp
// Limit concurrent SSL handshakes
SemaphoreGuard ssl_slot = ssl_semaphore.acquire();  // max 3 concurrent
perform_ssl_handshake();
```

#### 8. Separate Health Check Port (HTTP only)

**Add**: Port 47985 with simple HTTP health endpoint
```cpp
// No SSL overhead
GET /health → 200 OK {"status": "healthy", "sessions": 3}
```

**Benefit**: Reliable healthcheck without SSL complexity

### Long-Term Architecture (2+ Months) 🏗️

#### 9. Multi-Instance Wolf with Load Balancing

**Architecture**:
```
             Load Balancer
                  |
        +---------+---------+
        |         |         |
      Wolf-1   Wolf-2   Wolf-3
        |         |         |
    Sessions  Sessions  Sessions
    1,2,3     4,5,6     7,8,9
```

**Benefits**:
- Failure of Wolf-1 doesn't affect sessions 4-9
- Can do rolling restarts
- Horizontal scaling

**Challenges**:
- Need session affinity/sticky routing
- Shared state management
- Container orchestration complexity

#### 10. Session State Checkpointing

**Approach**:
- Periodically save Zed state to persistent storage
- Save Sway window positions, open files
- On reconnect, restore from checkpoint

**Implementation**:
```bash
# Every 60 seconds per session
zed-backup-state /workspace/.helix/session-checkpoint.json

# On reconnect
zed-restore-state /workspace/.helix/session-checkpoint.json
```

#### 11. Session Migration Between Wolf Instances

**When Wolf-1 becomes unhealthy:**
1. Mark Wolf-1 as draining (no new sessions)
2. Checkpoint active sessions
3. Start equivalent containers on Wolf-2
4. Restore from checkpoints
5. Update routing to Wolf-2
6. Restart Wolf-1

**Complexity**: High, but enables zero-downtime deployments

## Recommended Implementation Priority

### Phase 1: Quick Wins (This Week)
1. ✅ Increase connect_timeout to 10s (DONE)
2. ✅ Fix false pairing warning (DONE)
3. ⏳ Add Docker healthcheck to Wolf
4. ⏳ Add curl to Wolf container

### Phase 2: Reliability (Next 2 Weeks)
5. Internal Wolf watchdog thread
6. Graceful session metadata persistence
7. Add startup timing logs

### Phase 3: Observability (Next Month)
8. Prometheus metrics
9. Connection pool for SSL
10. Dedicated health check port

### Phase 4: Architecture (2+ Months)
11. Multi-instance support
12. Session state checkpointing
13. Session migration

## Success Metrics

**Target SLA**: 99.9% uptime

**Measurements**:
- SSL handshake p95 < 500ms, p99 < 2s
- Session creation success rate > 99%
- Mean time to recovery (MTTR) < 60s
- Zero data loss from Wolf crashes

**Current** (before fixes):
- SSL handshake p95 ~ 3s (estimated from timeouts)
- Session creation success rate ~ 90%
- MTTR ~ manual (could be hours)

## Production Monitoring (Now)

```bash
# Check if Wolf is responding
watch -n 10 'docker exec helixml-wolf-1 timeout 3 bash -c "cat < /dev/null > /dev/tcp/localhost/47984" && echo "✓ OK" || echo "✗ HUNG"'

# Monitor error rate
docker compose logs -f moonlight-web | grep ERROR

# Check resource usage
docker stats wolf-1
```

## Next Actions

1. **Merge and deploy timeout fix** (2.5.4 release)
2. **Add healthcheck to docker-compose** (PR this week)
3. **Monitor production** for reduced error rate
4. **Plan watchdog implementation** (design next week)

