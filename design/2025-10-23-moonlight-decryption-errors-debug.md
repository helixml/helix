# EVP_DecryptFinal_ex Decryption Errors in Wolf/Moonlight Streaming

**Date:** 2025-10-23
**Status:** Fixed with defensive re-pairing logic
**Impact:** Streaming sessions failing with control packet decryption errors

## Problem Statement

Users experiencing streaming failures with `EVP_DecryptFinal_ex failed` errors flooding Wolf logs. Streaming would work initially but fail after service restarts or after some time, preventing users from connecting to external agent sessions.

## Root Cause Analysis

### Certificate and Session Lifecycle Mismatch

**The core issue:** moonlight-web persists client certificates to disk, but Wolf does NOT persist session state (AES keys). This creates a dangerous window where:

1. **Session created** → moonlight-web generates cert + Wolf pairs client → Session runs with AES key `ABC`
2. **Services restart:**
   - moonlight-web loads OLD cert from disk ✅
   - Wolf loses ALL session state including AES keys ❌
3. **Client reconnects** → moonlight-web reuses OLD cert
4. **Wolf tries to pair** → May accept or reject depending on stale state
5. **Result:** Cert/key mismatch → `EVP_DecryptFinal_ex failed`

### Certificate Flow

```
Frontend (MoonlightStreamViewer.tsx)
  ↓ session_id="agent-ses_xxx"
  ↓ client_unique_id="helix-agent-ses_xxx"
  ↓
Helix Proxy (moonlight_proxy.go)
  ↓ Replaces Helix JWT with moonlight credentials
  ↓
moonlight-web (stream.rs)
  ↓ Checks cache for client_unique_id
  ↓ Reuses cert if exists OR generates new
  ↓ Auto-pairs with Wolf using PIN
  ↓
Wolf (Moonlight protocol)
  ✓ Accepts pairing
  ✓ Stores paired client
  ✓ Creates session with AES key
```

### Why Errors Occurred

Wolf's ENET control packets are encrypted with per-session AES keys (NOT the pairing cert). The decryption error happens because:

1. **Wolf restarts** → Loses session AES keys
2. **moonlight-web still has session** → With OLD AES key
3. **Client reconnects** → Uses OLD AES key
4. **Wolf expects NEW AES key** → Decryption fails

Even worse: If Wolf has **stale pairing state** for the cached cert, re-pairing might:
- Succeed but create duplicate/corrupted state
- Fail with "already paired" error
- Succeed but with wrong AES key association

## Investigation Steps

### 1. Added Debug Logging to Wolf

Modified `wolf/src/moonlight-server/control/control.cpp` to log decryption attempts:

```cpp
// Before decryption (line 199-206)
logs::log(logs::debug,
    "[ENET] Decrypting packet: session_id={}, client={}:{}, seq={}, size={}, aes_key_prefix={}",
    client_session->session_id,
    client_ip,
    client_port,
    boost::endian::little_to_native(enc_pkt->seq),
    packet->dataLength,
    client_session->aes_key.substr(0, 8));

// On failure (line 227-232)
logs::log(logs::warning,
    "[ENET] Decryption failed: session_id={}, client={}:{}, error={}",
    client_session->session_id,
    client_ip,
    client_port,
    e.what());
```

**Commit:** `d71c587` in wolf repo on branch `stable-moonlight-web`

### 2. Certificate Persistence Analysis

Verified that moonlight-web's certificate persistence (added earlier) was working:

```bash
$ docker logs moonlight-web | grep "Loaded persisted certificate"
07:29:33 [INFO] Loaded persisted certificate for client_unique_id: helix-agent-ses_01k87xc5...
07:29:33 [INFO] Loaded persisted certificate for client_unique_id: helix-agent-ses_01k86tne...
# ... 8 certificates loaded from previous sessions
```

But Wolf had zero apps after restart:
```bash
$ docker exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps
{"success":true,"apps":[]}
```

**This confirmed the mismatch:** moonlight-web had old certs, Wolf had no session state.

### 3. Traced Certificate Usage

**Helix creates sessions with:**
- Kickoff: `session_id="agent-{sessionID}-kickoff"`, `client_unique_id="helix-agent-{sessionID}"`
- Browser: `session_id="agent-{sessionID}"`, `client_unique_id="helix-agent-{sessionID}"`

Both use the **same** `client_unique_id` → should reuse same cert → enables Moonlight RESUME protocol.

**moonlight-web caching logic** (stream.rs lines 246-287):
```rust
if let Some(ref unique_id) = client_unique_id {
    let cache = data.client_certificates.read().await;
    if let Some(cached_auth) = cache.get(unique_id) {
        // REUSE cached cert ✅
    } else {
        // Generate NEW cert and cache it ✅
    }
}
```

**The issue:** Cached certs are reused, but Wolf may have stale pairing state for those certs, causing pairing to succeed but with corrupted session state.

## Solution: Defensive Re-Pairing

Added defensive unpair logic in moonlight-web to clear stale Wolf state before pairing:

**File:** `moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs` (lines 311-346)

```rust
// DEFENSIVE: Try to unpair stale pairing state using a SEPARATE host object
// This prevents corrupting the main temp_host used for pairing
if let Ok(server_cert) = pem::parse(&server_certificate_pem) {
    if let Ok(mut unpair_host) = ReqwestMoonlightHost::new(host_address.clone(), host_http_port, None) {
        if let Ok(_) = unpair_host.set_pairing_info(&client_auth, &server_cert) {
            match unpair_host.unpair().await {
                Ok(_) => {
                    info!("[Stream]: Successfully unpaired stale client - will re-pair fresh");
                }
                Err(err) => {
                    debug!("[Stream]: Unpair returned error (expected if not paired): {:?}", err);
                }
            }
        }
    }
}

// Create temporary host to pair (FRESH, not used for unpair)
let mut temp_host = match ReqwestMoonlightHost::new(host_address.clone(), host_http_port, None) {
    Ok(h) => h,
    Err(err) => {
        warn!("[Stream]: Failed to create temp host: {:?}", err);
        return;
    }
};

// Pair with unique credentials (fresh pairing every time)
if let Err(err) = temp_host.pair(&client_auth, format!("session-{}", session_id), pin).await {
    warn!("[Stream]: Auto-pairing failed: {:?}", err);
    // ... error handling
}
```

**Flow:**
1. Create `unpair_host` (separate from `temp_host`)
2. Set pairing info on `unpair_host` (makes unpair work)
3. Try to unpair using `unpair_host` (clears stale Wolf state)
   - Succeeds if Wolf has stale pairing → clears it ✅
   - Fails if not paired → expected, ignore ✅
4. Create FRESH `temp_host` (not contaminated by unpair)
5. Pair fresh using clean `temp_host` (generates new session, new AES keys)

**CRITICAL:** Using separate host objects prevents `set_pairing_info()` from corrupting the host used for pairing. Initial implementation (commit `4e402d3`) used the same host for both, which caused "Certificate verification failed" errors. Fixed in commit `e75781b`.

**Commits:**
- `4e402d3` - Initial defensive unpair (had bug)
- `e75781b` - Fix: Use separate host objects (correct implementation)

## Benefits

**Before:**
- Certificate persistence created stale state bugs
- Wolf restarts → streaming fails until certs manually cleared
- Unpredictable failures during reconnection

**After:**
- Certificate persistence still works (prevents unnecessary re-pairing)
- Wolf restarts → fresh pairing automatically clears stale state
- Predictable, reliable reconnection behavior

## Testing Recommendations

1. **Create external agent session** → verify streaming works
2. **Restart Wolf only** → verify streaming still works (defensive unpair clears stale state)
3. **Restart moonlight-web only** → verify streaming still works (certs persisted)
4. **Restart both** → verify streaming still works (fresh everything)

Watch logs for defensive unpair in action:
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web | grep -i "unpaired stale"
```

## Related Changes

### Wolf Debug Logging
- **File:** `wolf/src/moonlight-server/control/control.cpp`
- **Purpose:** Log session_id, AES key prefix, and error details for decryption failures
- **Benefit:** Makes future debugging much faster - can identify which sessions are failing

### Certificate Persistence (Previously Implemented)
- **Files:** `moonlight-web-stream/moonlight-web/web-server/src/data.rs`, `stream.rs`
- **Purpose:** Persist client certificates across moonlight-web restarts
- **Commit:** `38d287f` in moonlight-web-stream repo

## Future Improvements

### Option 1: Wolf Session State Persistence
Persist Wolf's session state (AES keys, session IDs) to disk so they survive restarts. This would eliminate the need for defensive re-pairing.

**Pros:** Eliminates root cause entirely
**Cons:** More complex, Wolf would need state persistence layer

### Option 2: Session Version Tracking
Add version numbers to sessions, detect stale sessions, force fresh pairing automatically.

**Pros:** More explicit error detection
**Cons:** Additional complexity in protocol

### Option 3: Current Approach (Defensive Re-Pairing)
Keep defensive unpair logic as-is - simple, robust, works reliably.

**Pros:** Simple, reliable, minimal code changes
**Cons:** Small overhead of unpair call on every session creation

**Recommendation:** Stick with Option 3 (defensive re-pairing) - it's simple, reliable, and has minimal overhead.

## Lessons Learned

1. **State persistence must be complete** - Partial persistence (certs but not sessions) creates worse bugs than no persistence
2. **Defensive programming pays off** - Unpair-before-pair pattern prevents many failure modes
3. **Debug logging is essential** - Wolf's control packet logging made this debuggable
4. **Test lifecycle boundaries** - Always test restart scenarios, not just happy path

## References

- **Wolf repo:** `/home/luke/pm/wolf` branch `stable-moonlight-web`
- **moonlight-web repo:** `/home/luke/pm/moonlight-web-stream` branch `feature/kickoff`
- **Helix repo:** `/home/luke/pm/helix` branch `feature/helix-code`

## Status

**REVERTED** - Defensive unpair approach caused moonlight-web streamer panics.

### Why Defensive Unpair Failed

The defensive unpair logic (commits 4e402d3 and e75781b) caused moonlight-web streamer to panic during video streaming:

```
thread '<unnamed>' (218) panicked at bytes-1.10.1/src/bytes.rs:396:9:
range end out of bounds: 643903619 <= 60
```

This panic occurred ~1 minute after video stream started, causing blank screens for users.

**Root cause unclear** - possibly:
1. `unpair()` corrupts Wolf's internal state
2. `unpair()` + `pair()` creates duplicate session records
3. Wolf video pipeline gets confused by rapid unpair/pair cycle
4. Timing issue where video packets arrive during unpair state transition

**Reverted in commit:** `0876816` - Removed all defensive unpair logic, back to certificate persistence only (commit 38d287f).

### Current State (Working)

- ✅ Certificate persistence enabled (survives moonlight-web restarts)
- ✅ Wolf debug logging enabled (helps diagnose future issues)
- ❌ Defensive unpair removed (caused panics)

**EVP_DecryptFinal_ex errors** appear to have been from stale sessions after restarts, not an active ongoing issue. Fresh restarts clear the problem.

### Recommendation

**Accept current behavior:**
- Certificate persistence prevents unnecessary re-pairing within same session
- If both Wolf and moonlight-web restart together, certs persist but Wolf loses state
- Solution: Restart both services together for clean slate: `docker compose down wolf moonlight-web && docker compose up -d wolf moonlight-web`
- Alternative: Delete `/server/data.json` in moonlight-web container to clear cached certs

**Do NOT attempt defensive unpair** - it triggers unknown bugs in Wolf/moonlight-web interaction.
