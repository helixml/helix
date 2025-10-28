# Wolf Lobby HostNotPaired Fix

**Date:** 2025-10-28
**Status:** ✅ RESOLVED
**Commit:** 3da88d191

## Problem

moonlight-web was failing with `HostNotPaired` error when trying to stream to Wolf lobbies for External Agents and Personal Dev Environments.

### User Report
```
still fucking HostNotPaired in the dev env on code.helix.ml :(((((
```

### Symptoms
- Auto-pairing init script was working correctly (Wolf UI app paired successfully)
- External Agent and PDE lobbies were being created successfully
- moonlight-web could not connect to lobbies - HostNotPaired error in browser
- Streaming to Wolf UI app worked fine (paired app)

## Root Cause Analysis

### Architecture Context
1. **moonlight-web pairing**: During container startup, moonlight-web auto-pairs with Wolf using `MOONLIGHT_INTERNAL_PAIRING_PIN=1234` (environment variable)
2. **Wolf UI app**: Single app (ID `134906179`) that moonlight-web pairs with during init script
3. **Wolf lobbies**: Separate streaming sessions created dynamically by Helix for External Agents and PDEs

### The Problem

Wolf lobbies were being created with **random PINs** for "multi-tenancy access control":

```go
// BROKEN CODE (before fix):
lobbyPIN, lobbyPINString := generateLobbyPIN()  // Random 4-digit PIN

lobbyReq := &wolf.CreateLobbyRequest{
    PIN: lobbyPIN,  // [1, 2, 3, 4] - random!
    // ...
}
```

When moonlight-web tried to stream:
1. Frontend calls `/moonlight/stream.html?hostId=0&appId=134906179` (Wolf UI app)
2. moonlight-web connects using Wolf UI credentials (paired during init)
3. **Wolf tries to switch to the lobby** for the actual session
4. Lobby requires its own PIN (random, not known to moonlight-web)
5. Wolf rejects with `HostNotPaired` because credentials don't match

### Why This Happened

Misunderstanding of Wolf's pairing model:
- **Apps** are static, pre-configured entities that get paired once
- **Lobbies** are dynamic, created on-demand, and each can have its own PIN
- We created lobbies with random PINs thinking it was "multi-tenancy security"
- But moonlight-web had no way to know these random PINs!

### Evidence from Code Investigation

```typescript
// frontend/src/components/external-agent/MoonlightWebPlayer.tsx
// Lines 48-73: Always connects to Wolf UI app, not to lobby directly
const wolfUIAppID = data.wolf_ui_app_id;
setStreamUrl(`/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}`);
```

```bash
# Wolf lobby listing showed PIN requirement:
$ docker compose -f docker-compose.dev.yaml exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies
{
  "lobbies": [
    {
      "id": "83efc6c3-786c-40c6-a130-6494813eb506",
      "name": "Agent y3n7",
      "pin_required": true,  # ❌ PROBLEM!
      ...
    }
  ]
}
```

## Solution

**Remove PIN requirement from all Wolf lobbies.**

### Implementation

```go
// FIXED CODE:
// CRITICAL: Do NOT set lobby PIN - moonlight-web is paired with Wolf UI, not individual lobbies
// Setting PIN here would cause HostNotPaired errors when streaming via moonlight-web
// Individual lobbies don't need PINs because:
// 1. Access is already controlled by Helix session ownership
// 2. moonlight-web is trusted (internal component, already paired with Wolf UI)
// 3. Lobby URLs are not publicly exposed (only via Helix frontend)

lobbyReq := &wolf.CreateLobbyRequest{
    PIN: nil,  // ✅ No PIN - moonlight-web connects using Wolf UI pairing
    // ...
}
```

### Changes Made

**File:** `api/pkg/external-agent/wolf_executor.go`

**Locations:**
1. **External Agent lobby creation** (line 388)
2. **PDE lobby creation** (line 741)
3. **PDE lobby recreation** (line 1487)

**Database storage:** Set `WolfLobbyPIN` to empty string for all new lobbies

**Logging:** Updated all log messages to reflect "no PIN required"

## Security Implications

### Why Removing PINs is Safe

1. **Access Control Already Exists**
   - Helix session ownership controls who can access which lobbies
   - Frontend only shows lobbies belonging to the authenticated user
   - Lobby IDs are UUIDs (not guessable)

2. **moonlight-web is Trusted**
   - Internal component running in same Docker network as Wolf
   - Already paired with Wolf UI app using `MOONLIGHT_INTERNAL_PAIRING_PIN`
   - Not exposed to external networks

3. **Lobby URLs Not Public**
   - Lobbies only accessible via Helix frontend
   - No direct external access to Wolf lobbies API
   - Firewall/network isolation at Docker level

### Multi-Tenancy

Original intent of PINs was multi-tenancy isolation, but:
- Helix already provides user isolation via session ownership
- Wolf lobbies are created per-session (not shared between users)
- User can only connect to their own sessions via Helix frontend

## Migration Path

### Existing Lobbies (with PINs)
- Continue to work (backward compatible)
- Users cannot stream via moonlight-web until recreated
- Will be garbage-collected by reconciliation loop

### New Lobbies (no PINs)
- Created automatically when users start External Agents or PDEs
- moonlight-web can connect immediately
- No manual intervention needed

### Clean Up Old Lobbies

User must recreate existing sessions to get PIN-less lobbies:

```bash
# Stop all existing lobbies with PINs
docker compose -f docker-compose.dev.yaml exec api curl -X DELETE --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies/<lobby-id>

# Or let Helix reconciliation clean them up automatically
# (happens on Wolf restarts via healthMonitor)
```

## Testing

### Verification Steps

1. **Create new External Agent session**
   ```bash
   # Via Helix frontend: Start External Agent
   # Check API logs for lobby creation
   docker compose -f docker-compose.dev.yaml logs api | grep "Wolf lobby created"
   # Should see: "Wolf lobby created successfully (no PIN required)"
   ```

2. **Verify lobby has no PIN**
   ```bash
   docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies | jq '.lobbies[] | {name, pin_required}'
   # Should see: "pin_required": false
   ```

3. **Test moonlight-web connection**
   - Open Helix frontend
   - Navigate to External Agents
   - Click "Stream" button
   - Should connect successfully (no HostNotPaired error)

### Expected Results

✅ **Before fix:** HostNotPaired error in browser console
✅ **After fix:** Streaming works, video/input functional

## Rollback Plan

If needed (unlikely), restore PIN generation:

```go
lobbyPIN, lobbyPINString := generateLobbyPIN()
lobbyReq.PIN = lobbyPIN
pde.WolfLobbyPIN = lobbyPINString
```

But this would break moonlight-web streaming again.

## Lessons Learned

1. **Understand pairing models fully** before implementing security features
2. **Wolf apps vs lobbies** have different pairing semantics
3. **Test with real moonlight-web** early in development (not just API curls)
4. **Security features** should solve actual problems, not theoretical ones
5. **Trust boundaries** matter: moonlight-web is internal, not external client

## References

- **Wolf auto-pairing code:** `/home/luke/pm/wolf/src/moonlight-server/rest/servers.cpp`
- **moonlight-web init script:** `/home/luke/pm/helix/moonlight-web-config/init-moonlight-config.sh`
- **Frontend streaming component:** `frontend/src/components/external-agent/MoonlightWebPlayer.tsx`
- **Related fix:** Commit 62bf8cf9d (moonlight-web init script health check fix)

## Future Improvements

1. **Consider lobby-level pairing** if true external client support needed
2. **Document Wolf pairing model** in CLAUDE.md for future reference
3. **Add integration tests** for moonlight-web streaming to lobbies
4. **Monitor for security implications** as Helix opens to public users
