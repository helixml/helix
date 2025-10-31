# Wolf-Moonlight-Web Pairing and Certificate Architecture

**Date**: 2025-10-29
**Status**: Investigation - Production pairing failures
**Environment**: Helix Code streaming platform

## Overview

This document explains the complete certificate generation and pairing process between Wolf (Moonlight streaming server) and moonlight-web (browser-based Moonlight client). We'll document how it SHOULD work, verify working behavior in dev, then compare against prod to identify the exact failure point.

## Architecture Components

### Wolf (Streaming Server)
- **Location**: Docker container `wolf`
- **Network**: Internal Docker network `helix_default` (172.19.0.0/16)
- **IP Address**: 172.19.0.50 (static in dev)
- **Ports**:
  - 47989 (HTTP - pairing, serverinfo)
  - 47984 (HTTPS - streaming, control)
- **Certificate Storage**: `/etc/wolf/cfg/cert.pem` and `/etc/wolf/cfg/key.pem`
- **Config File**: `/etc/wolf/cfg/config.toml`
- **Auto-pairing**: Reads `MOONLIGHT_INTERNAL_PAIRING_PIN` env var to auto-accept pairing

### Moonlight-Web (Browser Client)
- **Location**: Docker container `moonlight-web`
- **Network**: Same Docker network as Wolf
- **Ports**: 8080 (internal), 8081 (external)
- **Data Storage**: `/app/server/data.json` (persistent pairing data)
- **Config Storage**: `/app/server/config.json` (credentials, TURN servers)
- **Init Script**: `/app/server/init-moonlight-config.sh` (initializes config from templates)

## Certificate Generation Flow

### Step 1: Wolf Certificate Generation (First Startup)

**When**: Wolf container first starts without existing certificates
**Where**: Wolf's internal certificate generation (C++ code)
**Output**:
- `/etc/wolf/cfg/cert.pem` (server certificate)
- `/etc/wolf/cfg/key.pem` (server private key)

**Certificate Details**:
- **Type**: Self-signed X.509 certificate
- **Format**: PEM-encoded
- **Subject**: Moonlight Game Streaming Host
- **Usage**: TLS server authentication for Moonlight HTTPS protocol

**Important**: This certificate is Wolf's SERVER certificate. Moonlight clients must trust this certificate to establish HTTPS connections to Wolf.

### Step 2: Moonlight Pairing Protocol

The Moonlight protocol uses a **mutual authentication** pairing process:

1. **Client initiates pairing**: moonlight-web calls Wolf's HTTP pairing endpoint
2. **PIN exchange**: Wolf generates/accepts a 4-digit PIN
3. **Client certificate generation**: moonlight-web generates its own CLIENT certificate
4. **Server certificate exchange**: Wolf sends its SERVER certificate to moonlight-web
5. **Certificate storage**: Both sides store each other's certificates for mutual TLS

**Result**:
- Wolf stores client certificate in `config.toml` (paired clients list)
- Moonlight-web stores both:
  - Client certificate (for client authentication)
  - Server certificate (to trust Wolf's TLS)

## Pairing Data Structures

### Wolf config.toml (After Pairing)

```toml
uuid = "1bcc8cea-c0f1-428e-bc02-2a2c15dca0e0"

[[paired_clients]]
client_cert = """-----BEGIN CERTIFICATE-----
MIIDNzCCAh+gAwIBAgIBADANBgkqhkiG9w0BAQsFADBFMQswCQYDVQQGEwJBVTET...
-----END CERTIFICATE-----"""
app_id = 0

[[paired_clients]]
client_cert = """-----BEGIN CERTIFICATE-----
MIIDNzCCAh+gAwIBAgIBADANBgkqhkiG9w0BAQsFADBFMQswCQYDVQQGEwJBVTET...
-----END CERTIFICATE-----"""
app_id = 0
```

### Moonlight-Web data.json (After Pairing)

```json
{
  "hosts": [{
    "address": "wolf",
    "http_port": 47989,
    "https_port": 47984,
    "paired": {
      "client_private_key": "-----BEGIN PRIVATE KEY-----\r\nMIIEvQIBADA...",
      "client_certificate": "-----BEGIN CERTIFICATE-----\r\nMIIDNzCCAh+gAwIBAgIBADA...",
      "server_certificate": "-----BEGIN CERTIFICATE-----\r\nMIIDUzCCAjugAwIBAgIUPvhxISgw..."
    }
  }]
}
```

**Critical Fields**:
- `client_private_key`: Moonlight-web's private key for client cert
- `client_certificate`: Moonlight-web's cert (sent to Wolf during pairing)
- `server_certificate`: Wolf's cert.pem (received from Wolf during pairing)

## Auto-Pairing Flow (Helix Implementation)

### Environment Setup

**Wolf** sets:
```bash
MOONLIGHT_INTERNAL_PAIRING_PIN=1234  # Example PIN
```

**Moonlight-Web** sets:
```bash
MOONLIGHT_CREDENTIALS=helix          # API auth credentials
MOONLIGHT_INTERNAL_PAIRING_PIN=1234  # Same PIN (not actually read by moonlight-web)
```

### Auto-Pairing Sequence

**Location**: `/app/server/init-moonlight-config.sh` (moonlight-web init script)

```bash
# 1. Wait for Wolf to be ready (port 47989 responding)
# 2. Wait additional 5s for HTTPS endpoint initialization
# 3. Call moonlight-web's internal pairing API

POST http://localhost:8080/api/pair
Authorization: Bearer $MOONLIGHT_CREDENTIALS
Content-Type: application/json
Body: {"host_id": 0}
```

**What happens inside moonlight-web**:
1. `/api/pair` endpoint receives request
2. Checks if host 0 (Wolf) is already paired
3. If not paired:
   - Generates new client certificate/key pair
   - Calls Wolf's HTTP pairing endpoint (port 47989)
   - Sends PIN (from `MOONLIGHT_INTERNAL_PAIRING_PIN` env var)
   - Receives Wolf's server certificate
   - Stores all certificates in `data.json`

**What happens inside Wolf**:
1. HTTP pairing endpoint receives request
2. Reads `MOONLIGHT_INTERNAL_PAIRING_PIN` env var
3. Auto-accepts pairing if PIN matches
4. Adds client certificate to `config.toml`
5. Returns server certificate to client

## Dev Environment (Working State)

### Configuration

**docker-compose.dev.yaml**:
- Wolf: `image: wolf:helix-fixed` (local build)
- Moonlight-web: `image: helix-moonlight-web:helix-fixed` (local build)
- Wolf IP: 172.19.0.50 (static)
- Moonlight-web: Dynamic IP on same network

**Environment Variables**:
```yaml
wolf:
  environment:
    - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}
    - WOLF_INTERNAL_IP=172.19.0.50

moonlight-web:
  environment:
    - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}
    - MOONLIGHT_CREDENTIALS=${MOONLIGHT_CREDENTIALS:-helix}
```

**File Mounts**:
- Wolf: `./wolf/:/etc/wolf/cfg/` (bind-mount for config persistence)
- Wolf init script: `./wolf/init-wolf-config.sh:/etc/cont-init.d/05-init-wolf-config.sh:ro`
- Moonlight-web: `./moonlight-web-config:/app/server:rw` (config persistence)

**Stack Lifecycle**:
```bash
./stack stop   # Clears pairing data: rm moonlight-web-config/data.json
./stack start  # Fresh pairing on every startup
```

### Dev Pairing Sequence

1. **Wolf starts**: Generates certificates if missing, reads existing config.toml
2. **Moonlight-web starts**: Init script runs
3. **Init script**:
   - Checks if `data.json` exists (empty on first start)
   - Calls `/api/pair` to trigger pairing
4. **Pairing succeeds**: Certificates exchanged, both services trust each other
5. **Streaming works**: Browser connects, HTTPS handshake succeeds

**Key behavior**: Dev clears `data.json` on every restart, forcing fresh pairing. This ensures certificates stay in sync.

## Production Environment (Failing State)

### Configuration

**docker-compose.yaml**:
- Wolf: `image: ghcr.io/games-on-whales/wolf:wolf-ui` (registry image)
- Moonlight-web: `image: registry.helixml.tech/helix/moonlight-web:2.5.0-rc8`
- Network: Same Docker network setup
- Persistent volumes for both Wolf and moonlight-web config

**Environment Variables**:
```yaml
wolf:
  environment:
    - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}

moonlight-web:
  environment:
    - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}
    - MOONLIGHT_CREDENTIALS=${MOONLIGHT_CREDENTIALS:-}
    - TURN_PUBLIC_IP=${TURN_PUBLIC_IP:-}
```

**Key Difference**: Production does NOT clear `data.json` on restart. Old pairing data persists across container recreations.

### Observed Failures

#### Symptom 1: Auto-Pairing 404 Error

**Evidence**:
```bash
ssh root@code.helix.ml "docker exec helixml-moonlight-web-1 cat /tmp/pair-response.log"
# Output: HTTP/1.1 404 Not Found
```

**Investigation**:
- Binary inspection shows `pair_host` function IS compiled into rc8 image
- Direct API test with correct credentials: `HTTP/1.1 404 Not Found`
- Same API test with wrong credentials: `HTTP/1.1 401 Unauthorized`

**Conclusion**: The `/api/pair` endpoint exists but is somehow not being registered in the route table at runtime.

#### Symptom 2: TLS/HTTPS Errors (Original Issue)

**From previous investigation**:
```
HTTPS connection errors between moonlight-web and Wolf
Certificate verification failures
Unable to establish streaming sessions
```

**Hypothesis**: If pairing never succeeded (404 on `/api/pair`), then moonlight-web never received Wolf's server certificate. Without the server certificate, HTTPS connections to Wolf fail certificate verification.

## Comparison Matrix: Dev vs Prod

| Aspect | Dev (Working) | Prod (Failing) | Notes |
|--------|---------------|----------------|-------|
| Wolf Image | `wolf:helix-fixed` (local) | `ghcr.io/games-on-whales/wolf:wolf-ui` | Same upstream code |
| Moonlight-Web Image | `helix-moonlight-web:helix-fixed` | `registry.helixml.tech/helix/moonlight-web:2.5.0-rc8` | Different builds |
| Wolf Init Script | Bind-mounted from host | **MISSING** - not in image | **CRITICAL** |
| Pairing Data Persistence | Cleared on `./stack stop` | Persists across restarts | Different lifecycle |
| `/api/pair` Endpoint | Works (verified in dev) | Returns 404 | **ROOT CAUSE** |
| Server Certificate in data.json | Populated after pairing | May be stale from old pairing | |
| MOONLIGHT_CREDENTIALS | `helix` (default) | `6SZ3jzirPCzFup4z` (random) | Generated by install script |

## Root Cause Analysis

### Primary Issue: `/api/pair` Endpoint Not Registered

**Evidence**:
1. Binary contains `pair_host` function (verified via strings)
2. Source code shows `pair_host` in service registration (line 479 of mod.rs)
3. Runtime returns 404 when calling `/api/pair`
4. Other endpoints (`/api/hosts`, `/api/authenticate`) work correctly

**Hypothesis**: The moonlight-web binary running in production was built from a different commit or with different features than expected.

**Test**: Compare image SHAs and build times:
- Local rc8: `e64ebcef4c01` (30 hours ago, 454MB)
- Prod rc8: `e64ebcef4c01` (30 hours ago, 454MB)

**Result**: Same SHA, so it's the same image! The endpoint SHOULD be there.

### Secondary Issue: Stale Pairing Data

**Theory**: Production has old `data.json` from previous pairing attempt. This contains:
- Old client certificate (may not match what Wolf has)
- Old server certificate (may not match Wolf's current cert.pem)

**Evidence**:
```bash
ssh root@code.helix.ml "docker exec helixml-moonlight-web-1 cat /app/server/data.json"
# Shows paired=true with certificates
```

**Impact**: Even if pairing endpoint worked, moonlight-web thinks it's already paired and skips re-pairing.

## Investigation Plan

### Phase 1: Verify Endpoint Availability
- [x] Confirm `/api/pair` exists in rc8 binary (strings check)
- [x] Test endpoint with correct credentials (gets 404)
- [x] Test other endpoints (work correctly)
- [ ] Check if endpoint is conditionally compiled (feature flags)
- [ ] Verify actix-web service registration at runtime

### Phase 2: Certificate Synchronization
- [ ] Compare Wolf's `cert.pem` with moonlight-web's `data.json.server_certificate`
- [ ] Compare moonlight-web's `client_certificate` with Wolf's `config.toml.paired_clients`
- [ ] Check certificate expiration dates
- [ ] Verify certificate formats (PEM encoding, line endings)

### Phase 3: Fresh Pairing Test
- [ ] Stop production containers
- [ ] Clear `data.json` (force fresh pairing like dev)
- [ ] Restart containers and observe pairing sequence
- [ ] Capture full HTTP request/response from pairing attempt

### Phase 4: Image Build Verification
- [ ] Verify rc8 was built from correct branch (feature/kickoff)
- [ ] Check if Cargo.toml has feature flags affecting `/api/pair`
- [ ] Compare local vs registry image contents
- [ ] Rebuild from scratch without cache if needed

## Next Steps

1. **Immediate**: Check actix-web service registration at runtime
   - Add logging to see which endpoints are registered
   - Verify `pair_host` is actually added to route table

2. **Quick Test**: Clear production `data.json` and retry pairing
   - This tests if stale data is blocking pairing
   - Eliminates certificate sync as a variable

3. **Deep Dive**: Binary analysis of rc8 image
   - Extract web-server binary from registry image
   - Compare with locally built binary
   - Check for differences in compiled routes

4. **Fallback**: Manual pairing via Wolf's HTTP endpoint
   - Bypass moonlight-web's `/api/pair` endpoint
   - Call Wolf directly to establish pairing
   - Manually populate `data.json` with correct certificates

## Investigation Findings

### Dev vs Prod Pairing State Comparison

**Dev Environment** (Working):
- `data.json`: Contains complete pairing data with client/server certificates
- Wolf `cert.pem`: Matches server_certificate in data.json exactly
- Pairing: Fresh pairing on every `./stack start` (data.json cleared)
- `/api/pair` endpoint: Accessible and working

**Production Environment** (Failing):
- **Current `data.json`**: EMPTY `{"hosts": []}` (only 17 bytes)
- **Backup `data.json.backup`** (from Oct 29 16:00): Shows successful previous pairing
- Wolf `cert.pem`: MATCHES the server_certificate in backup (identical)
- **Key Finding**: Production WAS successfully paired, but data.json was cleared/lost
- `/api/pair` endpoint: Returns HTTP 404 (despite endpoint existing in binary)

### Certificate Synchronization Analysis

**Verification Results**:
1. ✅ Dev Wolf cert matches dev moonlight-web server_certificate
2. ✅ Prod Wolf cert matches prod backup server_certificate
3. ❌ Prod current data.json is empty (no pairing data)
4. ✅ rc8 binary contains `pair_host` function (verified via strings inspection)
5. ❌ `/api/pair` runtime registration appears to be failing in production

### Timeline of Production Pairing

```
Oct 29 16:00  - Successful pairing completed (backup shows certificates)
Oct 29 18:00  - data.json.backup created (preserving working pairing data)
Oct 29 20:14  - data.json becomes empty (17 bytes, cleared/recreated)
Oct 29 20:21  - Container restart, auto-pairing attempts but gets 404
Current       - No pairing data, HTTPS connections fail TLS verification
```

### Root Cause: Empty data.json

The TLS/HTTPS errors are a **symptom** of missing pairing data:
1. Moonlight-web has empty `data.json` (no server_certificate)
2. Without Wolf's server certificate, moonlight-web can't verify Wolf's TLS
3. Auto-pairing would fix this, BUT `/api/pair` returns 404
4. Therefore pairing cannot be re-established automatically

## ROOT CAUSE IDENTIFIED (UPDATED 2025-10-29 21:33)

### Config Not Available to `/api` Scope in Production

**Discovery Date**: 2025-10-29 (systematic endpoint testing)

**Root Cause**: `Data<Config>` is NOT being inherited by the `/api` scope in production, despite being available at the parent App level.

**Evidence**:
```bash
# Endpoints requiring Config OUTSIDE /api scope - WORK in both dev and prod
GET  /config.js          → 200 OK (requires config: Data<Config>)

# Endpoints requiring Config INSIDE /api scope - FAIL in prod, WORK in dev
POST /api/pair           → 404 Not Found (prod), 304 Not Modified (dev)
PUT  /api/host           → 404 Not Found (both dev and prod - different issue?)
GET  /api/host/stream    → Not tested (WebSocket endpoint)

# Endpoints requiring only RuntimeApiData - WORK in both dev and prod
GET  /api/hosts          → 200 OK
GET  /api/sessions       → 200 OK
```

**Code Structure**:
```rust
// main.rs line 77 - Config added to parent App
App::new()
    .app_data(config.clone())  // Config available here
    .service(api_service(data.clone(), config.credentials.to_string()))
    .service(web_config_js_service())  // config_js works (gets Config from parent)
    .service(web_service())

// api/mod.rs line 465 - /api scope does NOT add Config
pub fn api_service(data: Data<RuntimeApiData>, credentials: String) -> impl HttpServiceFactory {
    web::scope("/api")
        .wrap(middleware::from_fn(auth_middleware))
        .app_data(ApiCredentials(credentials))
        .app_data(data)
        // NO .app_data(config) here!
        .service(services![pair_host, ...])
}
```

**Why Dev Works But Prod Doesn't**: **ENVIRONMENTAL ISSUE CONFIRMED** (2025-10-29 21:45)

After extensive debugging including:
- Applied fix from feat/multi-webrtc (commit 93ff21d) adding Config to api_service scope
- Built multiple rc versions (rc12, rc13, rc13-dev-test) with fix applied
- Verified fix is in source code and freshly compiled
- Deployed EXACT SAME IMAGE (016bcbb649bb) that works in dev to production - still returns 404

**Conclusion**: The `/api/pair` endpoint 404 issue is NOT related to the code or Docker image. It's an environmental difference between dev and production that prevents Config-requiring endpoints in the `/api` scope from working, despite:
- Same source code with fix
- Same binary (dev image tested in prod environment)
- Same actix-web version (4.11.0)
- Working `/config.js` endpoint (proves Config IS available at App level in both environments)
- Working non-Config endpoints in `/api` scope (proves `/api` scope itself works)

**Exhaustive Investigation Performed** (2025-10-29 22:00):

All environmental factors checked:
1. ✅ Config.json content - tested with exact dev config in prod, still fails
2. ✅ Docker versions - prod: 28.5.1, dev: 28.4.0 (minor difference, unlikely cause)
3. ✅ Working directory - both /app
4. ✅ Environment variables - same except MOONLIGHT_CREDENTIALS (doesn't affect this)
5. ✅ Binary comparison - same image (016bcbb649bb) works in dev, fails in prod
6. ✅ Container startup command - both use init-moonlight-config.sh
7. ✅ Certificate key sizes - all 2048-bit RSA (correct)
8. ✅ Config encoding - no hidden characters or encoding issues
9. ✅ Credentials length - not the issue (old prod worked with 16-char creds)

**What We Know For Certain**:
- The `/api/pair` endpoint code EXISTS in the binary (verified via strings)
- The endpoint IS registered in source code (line 479 of mod.rs)
- Config IS available at App level (proven by `/config.js` working)
- Other `/api` endpoints work fine (`/api/hosts`, `/api/sessions`)
- Fix from feat/multi-webrtc was applied (Config added to api_service scope)
- **EXACT SAME IMAGE** works in dev, fails in prod

**Remaining Mystery**: What environmental factor causes Actix-web to fail to register Config-requiring endpoints within the `/api` scope in production but not dev?

**Recommendation**:
1. Try deploying to old prod environment (dev.code.helix.ml) to see if it works there
2. Or use backup workaround: Restore data.json.backup which has working pairing certificates

### Previous Theory (DISCARDED): Missing Config Dependency

**Discovery Date**: 2025-10-29 (commit history analysis)

**Theory**: Actix-web dependency injection failure

The `/api/pair` endpoint **IS registered** in feature/kickoff, but returns 404 because:

1. **Handler requires Config dependency**:
   ```rust
   async fn pair_host(
       data: Data<RuntimeApiData>,
       config: Data<Config>,  // <--- Requires this!
       Json(request): Json<PostPairRequest>,
   ) -> HttpResponse
   ```

2. **api_service doesn't provide Config** (feature/kickoff):
   ```rust
   pub fn api_service(data: Data<RuntimeApiData>, credentials: String) -> impl HttpServiceFactory {
       web::scope("/api")
           .app_data(data)
           // Config is MISSING - dependency injection fails!
   ```

3. **Actix-web returns 404 on dependency injection failure** (not 500)
   - Route IS registered
   - When request arrives, Actix tries to extract `Data<Config>`
   - Can't find Config in app_data
   - Returns 404 instead of calling handler

**The Fix Already Exists**: Commit **93ff21d** on `feat/multi-webrtc` branch (Oct 15 2025):
```rust
pub fn api_service(data: Data<RuntimeApiData>, credentials: String, config: Data<Config>) -> impl HttpServiceFactory {
    web::scope("/api")
        .app_data(data)
        .app_data(config)  // <--- This fixes it!
```

**Git History**:
- Commit 93ff21d: "Fix route registration - use individual .service() calls"
- Message: "Issue: Second services![] macro wasn't registering routes (404 errors)"
- Branch: `feat/multi-webrtc` only (NOT in feature/kickoff)
- Changes: Added Config parameter and `.app_data(config)` to scope

**Why This Wasn't Caught Earlier**:
- feature/kickoff doesn't have the fix
- rc8 was built from feature/kickoff
- Binary contains `pair_host` function (passes strings inspection)
- Route IS registered (no syntax errors)
- But runtime dependency injection fails silently with 404

## `/api/pair` Endpoint History

**Question**: Did we add `/api/pair` or has it always been in moonlight-web?

**Answer**: The endpoint has existed since the initial moonlight-web implementation (commit 7be9862 "moved moonlight web into a directory"). It's part of the original Moonlight pairing protocol implementation, not something we added later.

**Pairing-related commits** (chronological):
1. `7364a3e` - "added pairing" (initial implementation)
2. `da0308a` - "more work on pair"
3. `fe6acd5` - "cleaned up pairing"
4. `67afd8a` - "fixed pairing issues"
5. `603ca85` - "fixed pairing issues"
6. `5cb8108` - "moved fns for pairing"
7. `63171c2` - "added unpair"
8. `6c6456d` - "Add MOONLIGHT_INTERNAL_PAIRING_PIN support for auto-pairing" ← **We added this**
9. `93ff21d` - "Fix route registration - use individual .service() calls" ← **We added this fix**

**What we added**:
- Auto-pairing PIN support (commit 6c6456d)
- Route registration fix (commit 93ff21d) - **THIS IS THE MISSING FIX**

**What was upstream**:
- Core `/api/pair` endpoint and pairing protocol
- Manual pairing flow via browser UI

## SOLUTION: Update Production to Latest rc8

**Immediate Fix**: Pull latest rc8 and recreate container in production:

```bash
ssh root@code.helix.ml "docker compose -f docker-compose.yaml pull moonlight-web"
ssh root@code.helix.ml "docker compose -f docker-compose.yaml down moonlight-web"
ssh root@code.helix.ml "docker compose -f docker-compose.yaml up -d moonlight-web"
```

This will:
1. Pull the newer rc8 image (Oct 28 13:58) from registry
2. Replace the old cached rc8 (30 hours ago)
3. Recreate container with newer image
4. Test if `/api/pair` works with the updated image

**If this doesn't fix it**, then the problem is in the registry rc8 itself and we need to:
- Build and push rc9 or rc10 from current feature/kickoff HEAD
- Or cherry-pick commit 93ff21d if it's actually needed

## Alternative Solution (IF rc8 update doesn't work): Cherry-Pick Route Fix

Cherry-pick commit 93ff21d from feat/multi-webrtc:
```bash
cd /home/luke/pm/moonlight-web-stream
git checkout feature/kickoff
git cherry-pick 93ff21d
# Rebuild and tag as rc10
```

This adds explicit Config to api_service scope (though it shouldn't be necessary since Config is in parent App)

## Recommended Next Steps

### Option A: Restore from Backup (Immediate Workaround)
**Quickest path to working state** - Restore the known-good pairing data:
```bash
ssh root@code.helix.ml "cp /opt/HelixML/moonlight-web-config/data.json.backup /opt/HelixML/moonlight-web-config/data.json"
ssh root@code.helix.ml "docker restart helixml-moonlight-web-1"
```

**Pros**: Immediate fix, certificates already synced
**Cons**: Doesn't solve the `/api/pair` 404 issue for future re-pairing

### Option B: Debug /api/pair 404 Issue
**Systematic investigation** of why endpoint returns 404:

1. **Test endpoint timing** - Add delay before calling `/api/pair`:
   ```bash
   # Modify init script to wait 10 seconds after web-server starts
   # Then test if /api/pair becomes available
   ```

2. **Verify actix-web route registration** - Add debug logging:
   ```rust
   // In api/mod.rs api_service function
   info!("[API]: Registering pair_host endpoint at /api/pair");
   ```

3. **Test endpoint directly** - From inside running container with curl:
   ```bash
   docker exec helixml-moonlight-web-1 bash -c 'sleep 30 && curl -v ...'
   ```

4. **Compare rc8 with local build** - Verify registry image matches source:
   ```bash
   # Extract and compare binaries
   docker run --rm --entrypoint sh registry.helixml.tech/helix/moonlight-web:2.5.0-rc8 -c "sha256sum /app/web-server"
   ```

### Option C: Manual Pairing via Wolf API (Alternative)
**Bypass moonlight-web's `/api/pair`** entirely:

1. Call Wolf's pairing endpoint directly (HTTP port 47989)
2. Generate client certificate manually
3. Populate `data.json` with pairing data
4. Restart moonlight-web to load certificates

**Pros**: Works around broken `/api/pair` endpoint
**Cons**: Complex, requires manual certificate generation

## Success Criteria

Production is working when:
1. `/api/pair` endpoint responds (not 404)
2. Auto-pairing completes successfully during container startup
3. Wolf's `config.toml` contains moonlight-web's client certificate
4. Moonlight-web's `data.json` contains Wolf's server certificate
5. Browser can establish HTTPS streaming session with Wolf
6. No TLS/certificate verification errors in logs
