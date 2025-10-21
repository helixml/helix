# Production Zed WebSocket Freeze Investigation

**Date:** 2025-10-21
**Issue:** External agents in production start containers and Zed process, but Zed freezes after database migrations and never connects WebSocket to Helix API

## Environment Comparison

### Working (Dev)
- **Setup:** docker-compose.dev.yaml with bind-mounted zed-build/
- **Network:** helix_default (172.19.0.x)
- **Zed Binary:** /home/luke/pm/zed/target/debug/zed (1.3GB, md5: 256375c356edfe48b10077fd5f5f67bb)
- **API Token:** oh-hallo-insecure-token
- **Result:** ✅ Zed connects via WebSocket, agent panel opens, fully functional

### Broken (Prod)
- **Setup:** docker-compose.yaml with Zed baked into image
- **Network:** helix_default (172.19.0.x, same network)
- **Zed Binary:** /zed-build/zed in image (1.3GB, md5: 256375c356edfe48b10077fd5f5f67bb - **IDENTICAL**)
- **API Token:** vh4FsB2yeCL3YPq (different but valid)
- **Result:** ❌ Zed freezes after DB migrations, never creates workspace/agent panel

## Key Findings

### Binary and Configuration
- ✅ **Exact same Zed binary** (md5 verified: `256375c356edfe48b10077fd5f5f67bb`)
- ✅ **Identical env vars:** ZED_HELIX_URL=api:8080, ZED_HELIX_TLS=false, etc.
- ✅ **Same launch command:** `/zed-build/zed .`
- ✅ **Same settings structure:** Both have `auto_open_panel: true`, `external_sync.enabled: true`
- ✅ **Network connectivity works:** api:8080 is reachable, HTTP/WebSocket endpoints respond correctly

### Behavioral Differences

**Dev Zed logs (4792 lines):**
```
2025-10-20T15:42:47 INFO  [external_websocket_sync] External WebSocket sync module initialization completed
...migrations...
2025-10-20T15:42:48 ERROR [zed] Is a directory (os error 21)
2025-10-20T15:42:49 INFO  [external_websocket_sync] 🔧 [CALLBACK] init_thread_display_callback() called
2025-10-20T15:42:49 INFO  [zed::zed] 🔧 [ZED] Setting up WebSocket integration...
2025-10-20T15:42:49 INFO  [zed::zed] 🔌 [ZED] WebSocket sync ENABLED - starting service
2025-10-20T15:42:49 INFO  [external_websocket_sync::websocket_sync] ✅ [WEBSOCKET] WebSocket connected! Response status: 101
```

**Prod Zed logs (25 lines):**
```
2025-10-20T16:17:05 INFO  [external_websocket_sync] External WebSocket sync module initialization completed
...migrations...
2025-10-20T16:17:06 INFO  [sqlez::migrations] Found 9 foreign key relationships to check
[STOPS HERE - NO FURTHER LOGGING]
```

**Docker logs with debug output (latest test):**
```
🔍 [DEBUG] Checking for open request...
🔍 [DEBUG] Got open request, parsing...
🔍 [DEBUG] Open request parsed, handling request with paths
🔍 [DEBUG] handle_open_request() called with kind=false, open_paths=1
[STOPS HERE]
```

### Code Flow Analysis

From `main.rs` line 910-918:
```rust
if !request.open_paths.is_empty() || !request.diff_paths.is_empty() {
    eprintln!("🔍 [DEBUG] Spawning async task to open paths..."); // NOT APPEARING
    task = Some(cx.spawn(async move |cx| {
        eprintln!("🔍 [DEBUG] Inside open_paths spawn..."); // NOT APPEARING
        let paths_with_position = derive_paths_with_position(...).await;
        // ... open_paths_with_positions() ...
    }));
}
```

The debug log shows we reach line 797 (`handle_open_request()` entry) but **never** print the "Spawning async task" message at line 911.

**Hypothesis:** Something between line 797 and line 910 is silently failing or the condition `!request.open_paths.is_empty()` is somehow false.

## Network Tests from Prod Container

```bash
# DNS resolution
$ getent hosts api
172.19.0.8      api

# TCP connectivity
$ nc -zv api 8080
Connection to api (172.19.0.8) 8080 port [tcp/http-alt] succeeded!

# HTTP connectivity
$ curl http://api:8080/health
HTTP/1.1 200 OK

# WebSocket upgrade
$ curl -H "Upgrade: websocket" http://api:8080/api/v1/external-agents/sync?session_id=...
HTTP/1.1 101 Switching Protocols
```

✅ All network tests pass - not a connectivity issue

## Differences from Dev

1. **Zed binary source:**
   - Dev: Bind-mounted from host `/home/luke/pm/helix/zed-build/`
   - Prod: Baked into Docker image at `/zed-build/zed`

2. **API token values:**
   - Dev: `oh-hallo-insecure-token`
   - Prod: `vh4FsB2yeCL3YPq` (but both work for zed-config endpoint)

3. **Settings external_url:**
   - Dev: `http://localhost:8080/...` (unused - env vars take precedence)
   - Prod: `https://code.helix.ml/...` (unused - env vars take precedence)

4. **Container creation:**
   - Dev: Created by local Wolf
   - Prod: Created by remote Wolf (same version, same code)

## Next Steps

1. ✅ Added extensive debug logging to `handle_open_request()` flow
2. ⏳ Waiting for fresh session creation with latest debug image
3. ⏳ Need to see if "Spawning async task" debug message appears
4. ⏳ If not appearing: investigate why condition at line 910 fails
5. ⏳ If appearing but spawn doesn't execute: executor/runtime issue

## Debug Image Deployed

**Current prod image:** `registry.helixml.tech/helix/zed-agent:0de86af7e`
**Digest:** `sha256:0b9d0b9213131ea67d48e6d7e783d5c00ef29e2246cc8262b0af8c47f7e5a985`
**Status:** Ready for testing with next session creation

## Open Questions

1. Why does `handle_open_request()` not proceed past line 797?
2. Why does dev have `ERROR [zed] Is a directory (os error 21)` but prod doesn't?
3. Why does dev log "Found 10 foreign key relationships" (3rd migration) but prod stops at 9?
4. Is there a silent panic or deadlock in prod that doesn't appear in dev?
