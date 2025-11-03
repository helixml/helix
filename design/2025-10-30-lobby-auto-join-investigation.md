# Wolf Lobbies Auto-Join Investigation

**Date:** 2025-10-30
**Status:** Investigation in progress
**Context:** Lobby auto-joining not working when accessing via moonlight-web

## Problem Statement

When a user accesses a specific session through moonlight-web in the browser, the expected behavior is automatic joining of the corresponding lobby. Currently, users must manually:
1. Navigate the Wolf UI lobby browser
2. Find their lobby by name
3. Enter the PIN

**Expected behavior:** Auto-join the lobby directly based on session context.

## Current Implementation Analysis

### URL Construction (Frontend)

**File:** `frontend/src/components/external-agent/MoonlightWebPlayer.tsx`

```typescript
// Lobbies mode URL construction
setStreamUrl(`/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}`);
```

**Current URL parameters:**
- `hostId=0`: Wolf server index in moonlight-web config (always 0)
- `appId=${wolfUIAppID}`: Wolf UI app ID (lobby browser interface)

**Missing parameters:**
- ❌ No lobby ID
- ❌ No lobby PIN
- ❌ No session ID or client ID

### moonlight-web Stream Handler

**File:** `/home/luke/pm/moonlight-web-stream/moonlight-web/web-server/web/stream.ts`

```typescript
// Lines 25-36: URL parameter parsing
const queryParams = new URLSearchParams(location.search)
const hostIdStr = queryParams.get("hostId")
const appIdStr = queryParams.get("appId")
```

**Supported parameters:**
- ✅ `hostId` - Which Wolf server to connect to
- ✅ `appId` - Which app to launch

**NOT supported:**
- ❌ `lobbyId` - Which lobby to auto-join
- ❌ `lobbyPin` - PIN for auto-authentication
- ❌ `sessionId` - Helix session identifier
- ❌ `clientId` - Moonlight client identifier

### Data Flow

**What we have:**

1. **Helix API** creates external agent session with Wolf lobby:
   ```go
   // api/pkg/server/external_agent_handlers.go:195-199
   response.WolfLobbyID = lobbyID
   response.WolfLobbyPIN = lobbyPIN
   ```

2. **Helix Session** stores lobby ID and PIN:
   ```go
   session.Config.WolfLobbyID = lobbyID
   session.Config.WolfLobbyPIN = lobbyPIN
   ```

3. **Frontend** has access to lobby ID and PIN:
   ```typescript
   // From API: session?.config?.wolf_lobby_pin
   // Displayed in UI for manual entry
   ```

4. **URL** is constructed with only `hostId` and `appId`:
   ```typescript
   /moonlight/stream.html?hostId=0&appId=${wolfUIAppID}
   ```

5. **moonlight-web** connects to Wolf UI but has no context about which lobby to join

**What's missing:**

- ❌ Lobby ID not passed to moonlight-web
- ❌ Lobby PIN not passed to moonlight-web
- ❌ No auto-join mechanism in moonlight-web
- ❌ No Wolf API for programmatic lobby joining

## Potential Auto-Join Approaches

### Option 1: URL Parameters + moonlight-web Enhancement

**Add to URL:**
```typescript
/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}&lobbyId=${wolfLobbyId}&lobbyPin=${wolfLobbyPin}
```

**Pros:**
- Simple, straightforward approach
- Parameters are already available in frontend
- Could work if moonlight-web supports it

**Cons:**
- Requires moonlight-web code changes
- PIN visible in URL (security concern)
- moonlight-web doesn't currently support this

### Option 2: Wolf API-Based Auto-Join

**Use Wolf's lobby join API directly:**
```bash
POST /api/v1/lobbies/join
{
  "lobby_id": "96a044f8-25cc-4d14-9829-92f895648452",
  "moonlight_session_id": "9415399566440428936",
  "pin": [9, 6, 5, 3]
}
```

**Pros:**
- Wolf already has this API
- Secure (POST body, not URL)
- Could be called from Helix backend

**Cons:**
- Requires `moonlight_session_id` (client identifier)
- Client needs to be already connected to Wolf before join
- Chicken-and-egg: need to connect to get session ID, but need session ID to join

### Option 3: Pre-Authenticated Session Token

**Create a session token that moonlight-web uses:**
1. Helix API creates Wolf lobby join token
2. Token is passed to moonlight-web via URL
3. moonlight-web presents token to Wolf
4. Wolf auto-joins lobby based on token

**Pros:**
- Secure (token-based, not PIN in URL)
- Clean separation of concerns
- Token can be single-use/time-limited

**Cons:**
- Requires Wolf code changes
- Most complex approach
- Need to implement token generation/validation

### Option 4: Client ID Pre-Registration

**Pre-register the client with Wolf before streaming:**
1. Helix API calls Wolf to register client for lobby
2. Wolf associates client cert with lobby + PIN
3. moonlight-web connects with pre-paired cert
4. Wolf auto-joins based on cert recognition

**Pros:**
- No sensitive data in URL
- Uses existing Wolf pairing mechanism
- Secure

**Cons:**
- Complex flow
- Requires understanding Wolf's client pairing
- May not work with multi-device scenarios

## Client ID vs Session ID Confusion

The user mentioned "client_id/session_id variable confusion". Let's clarify:

### Wolf Terminology

**`moonlight_session_id`:** (in Wolf lobby join API)
- This is Wolf's identifier for a connected Moonlight client
- Generated when a client connects to Wolf
- Used in `/api/v1/lobbies/join` POST body
- **NOT the same as Helix session ID**

**`session_id`:** (in Wolf GStreamer pipelines)
- Wolf's internal streaming session identifier
- Used for interpipe switching
- Example: `"9415399566440428936"` (numeric string)

**`lobby_id`:** (in Wolf lobbies API)
- UUID identifying the lobby
- Example: `"96a044f8-25cc-4d14-9829-92f895648452"`

### Helix Terminology

**`sessionID`:** (Helix external agent session)
- Helix's identifier for agent session
- Example: `"ses_01JBFK..." ` (prefixed)
- Stored in database, used in API endpoints
- Mapped to Wolf lobby ID via `session.Config.WolfLobbyID`

**`client_id`:** (moonlight-web pairing)
- Not currently used in auto-join context
- May be relevant for certificate-based approaches

## Current UX (Manual Lobby Join)

**User Flow:**
1. User clicks "Live Stream" on session
2. Browser loads `/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}`
3. moonlight-web connects to Wolf UI (lobby browser)
4. User sees list of available lobbies
5. User clicks on their lobby (identified by name, e.g., "Agent xyz")
6. Wolf prompts for PIN
7. User enters PIN from Helix UI
8. User successfully joins lobby

**Pain points:**
- Steps 4-7 are manual and repetitive
- User must find their lobby in the list
- User must copy/paste or remember PIN
- If multiple users have sessions, list can be confusing

## Desired UX (Auto-Join)

**Ideal Flow:**
1. User clicks "Live Stream" on session
2. Browser loads moonlight-web with session context
3. moonlight-web automatically joins correct lobby with PIN
4. User immediately sees streaming session
5. Done!

**Benefits:**
- Seamless user experience
- No manual navigation or PIN entry
- Works like apps mode (direct connection feel)
- Still gets multi-user benefits of lobbies mode

## Wolf Lobbies API

**Current endpoints (from Wolf source):**

```bash
# List lobbies
GET /api/v1/lobbies
Response: {"lobbies": [{"id": "...", "name": "...", ...}]}

# Join lobby
POST /api/v1/lobbies/join
Body: {
  "lobby_id": "96a044f8-...",
  "moonlight_session_id": "9415399566440428936",
  "pin": [9, 6, 5, 3]
}
```

**Key constraint:** `moonlight_session_id` must be from an ALREADY CONNECTED client.

This means the client must:
1. Connect to Wolf first (via Moonlight protocol handshake)
2. Get assigned a `moonlight_session_id`
3. THEN call `/api/v1/lobbies/join` with that ID

This is a chicken-and-egg problem for URL-based auto-join.

## Possible Solution: Two-Phase Connection

**Phase 1: Connect to Wolf UI**
- URL: `/moonlight/stream.html?hostId=0&appId=${wolfUIAppID}`
- moonlight-web connects to Wolf
- Wolf assigns `moonlight_session_id`

**Phase 2: Auto-Join Lobby**
- moonlight-web detects `lobbyId` and `lobbyPin` URL parameters
- After connection complete, calls Wolf's `/api/v1/lobbies/join` API
- Joins lobby automatically
- User sees lobby stream, not lobby browser

**Implementation:**
1. Frontend adds `lobbyId` and `lobbyPin` to URL
2. moonlight-web enhanced to:
   - Parse these new URL parameters
   - Wait for connection to complete
   - Call Wolf lobby join API
   - Handle success/failure

## Next Steps

1. **Verify moonlight-web capabilities:**
   - Can it call Wolf APIs after connecting?
   - Can it handle lobby join programmatically?
   - Does it expose connection state events?

2. **Check Wolf API authentication:**
   - Does `/api/v1/lobbies/join` require authentication?
   - Can moonlight-web call it from browser context?
   - CORS/security implications?

3. **Identify where the implementation should live:**
   - moonlight-web code changes?
   - Helix backend proxy/helper endpoint?
   - Wolf code changes?

4. **Understand current behavior:**
   - Why does the user think auto-join should work?
   - Was there previous functionality that broke?
   - Is there confusion about what "auto-join" means?

5. **Add debugging:**
   - Log lobby ID and PIN availability
   - Trace URL parameter flow
   - Monitor Wolf API calls
   - Identify exact failure point

## Questions to Answer

1. **Was auto-join ever implemented?**
   - Check git history for removed functionality
   - Search for previous auto-join code

2. **What does "not working" mean exactly?**
   - Is there an error?
   - Does it fall back to manual join?
   - Is there missing data?

3. **Where is the client ID coming from?**
   - moonlight-web session ID after connection?
   - Pre-existing pairing data?
   - Something else?

4. **Why the mention of client_id/session_id confusion?**
   - Are these being mixed up in the code?
   - Is there a bug in identifier mapping?
   - Are logs showing wrong IDs?

## Immediate Debugging Actions

1. **Add logging to URL construction:**
   ```typescript
   console.log('MoonlightWebPlayer: Constructing stream URL', {
     wolfLobbyId,
     wolfLobbyPin: session?.config?.wolf_lobby_pin,
     wolfUIAppID,
     streamUrl
   });
   ```

2. **Check Wolf lobby state:**
   ```bash
   # See if lobbies exist and their IDs
   docker compose -f docker-compose.dev.yaml exec api \
     curl --unix-socket /var/run/wolf/wolf.sock \
     http://localhost/api/v1/lobbies | jq '.'
   ```

3. **Monitor Wolf lobby join attempts:**
   ```bash
   # Watch Wolf logs for join API calls
   docker compose -f docker-compose.dev.yaml logs --tail 50 -f wolf | grep -i "lobby.*join"
   ```

4. **Verify session metadata:**
   ```bash
   # Check if Helix sessions have lobby ID/PIN stored
   # Via API: GET /api/v1/external-agents/{sessionID}
   ```

## Summary - UPDATED WITH FINDINGS

### Auto-Join WAS Implemented But REMOVED Due to Timing Issue

**Timeline:**
- **Oct 27, 2025** (commit `46f27524a`): Auto-join implemented in backend
- **Oct 28, 2025** (commit `97fa15e65`): Auto-join REMOVED - broke input functionality

**What the implementation did:**
1. Added `autoJoinWolfLobby()` function in `external_agent_handlers.go` (110 lines)
2. Called during `createExternalAgent` API handler
3. Found Wolf UI app ID via `wolf.Client.ListApps()`
4. Queried `/api/v1/sessions` to find client's `moonlight_session_id`
5. Called `wolf.Client.JoinLobby()` with lobby ID, session ID, and PIN

**Why it was removed:**
> "Auto-join was called during agent creation, BEFORE user connected. At that point, no Wolf UI session existed yet to join. This broke input functionality."

**The Timing Problem:**
```
Current flow:
1. User creates external agent → createExternalAgent() runs
2. ❌ autoJoinWolfLobby() tries to find session (DOESN'T EXIST YET)
3. User clicks "Live Stream" → moonlight-web connects → session created
4. ❌ Too late - auto-join already failed

Correct flow should be:
1. User creates external agent → lobby created
2. User clicks "Live Stream" → moonlight-web connects → session created
3. ✅ NOW call auto-join (session exists)
```

### The JoinLobby Infrastructure Still Exists

The removal only deleted the `autoJoinWolfLobby()` function. The underlying infrastructure remains:

**Still in codebase:**
- ✅ `wolf.Client.JoinLobby()` method (`api/pkg/wolf/client.go:536`)
- ✅ `JoinLobbyRequest` struct with proper types
- ✅ Wolf `/api/v1/lobbies/join` API integration
- ✅ PIN conversion logic (string → `[]int16`)

**What's needed:**
- Call auto-join at the RIGHT TIME (after user connects)
- Either:
  1. New API endpoint triggered by frontend after connection
  2. Frontend calls directly (requires exposing Wolf client)
  3. WebSocket message trigger after connection event

### Client ID vs Session ID Clarification

The removed code shows the mapping:

```go
// From Wolf /api/v1/sessions response
var sessionsData struct {
    Sessions []struct {
        AppID    string `json:"app_id"`
        ClientID string `json:"client_id"` // NOTE: This is the Moonlight session_id
        ClientIP string `json:"client_ip"`
    } `json:"sessions"`
}
```

**Key insight:** Wolf's `/api/v1/sessions` returns `client_id` which IS the `moonlight_session_id` needed for `/api/v1/lobbies/join`.

The "confusion" was likely:
- Helix session IDs (external agent sessions: `"ses_01JBFK..."`)
- Wolf app/lobby IDs (numeric strings: `"134906179"`)
- moonlight_session_id (numeric strings from Wolf sessions API)

These need to be mapped correctly, and the timing must be right for auto-join to work.

## FINAL SOLUTION: Frontend-Provided Wolf Client ID

### Root Cause Analysis (CONFIRMED)

**Wolf client_id generation:**
```cpp
// From ~/pm/wolf/src/moonlight-server/state/config.hpp:99-101
inline std::size_t get_client_id(const PairedClient &current_client) {
  return std::hash<std::string>{}(current_client.client_cert);
}
```

- Wolf generates `client_id` as a HASH of the client certificate (not sequential, not random - deterministic per cert)
- Multiple connections from same paired client get DIFFERENT client_ids (different certs created per session)
- The `session_id` moonlight-web sends (`agent-${sessionId}`) is NOT exposed in Wolf's `/api/v1/sessions` API

**Why IP matching doesn't work:**
- moonlight-web runs as a proxy in Docker network
- From Wolf's perspective, ALL client connections have the SAME IP (moonlight-web container IP: 172.19.0.14)
- Cannot distinguish between different browser clients based on IP

**Why we can't match without Wolf changes:**
Wolf's `/api/v1/sessions` API response:
```json
{
  "app_id": "134906179",
  "client_id": "6318980640517831945",  // Certificate hash (what we need!)
  "client_ip": "172.19.0.14",          // Same for all connections (useless)
  // NO session_id field
  // NO client_unique_id field
}
```

moonlight-web sends but Wolf doesn't expose:
- `session_id`: `"agent-${helixSessionId}"`
- `client_unique_id`: `"helix-agent-${helixSessionId}"`

### The Solution: Three-Part Modification

**Backend (COMPLETED ✅):**
- `/api/v1/external-agents/{sessionID}/auto-join-lobby` endpoint now accepts optional `wolf_client_id` in POST body
- If provided: Uses exact client_id for precise matching
- If not provided: Falls back to first available Wolf UI session (may be wrong if multiple exist)
- Enhanced logging for debugging session matching issues

**Wolf (TODO):**
1. Modify `ConnectionComplete` message struct to include `client_id`:
```rust
// File: ~/pm/wolf/common/src/api_bindings.rs
ConnectionComplete {
    capabilities: StreamCapabilities,
    width: u32,
    height: u32,
    client_id: String,  // ADD THIS - Wolf knows this value
}
```

2. Populate it in Wolf's streaming code when sending ConnectionComplete message

**moonlight-web (TODO):**
1. Update api_bindings.rs to include new `client_id` field in `ConnectionComplete`
2. Modify Stream class to expose the received client_id:
```typescript
// File: ~/pm/moonlight-web-stream/moonlight-web/web-server/web/stream/index.ts
private wolfClientID: string | null = null;

// In onMessage handler for ConnectionComplete:
} else if ("ConnectionComplete" in message) {
    this.wolfClientID = message.ConnectionComplete.client_id;
    // ... existing code
}

// Add getter method:
getWolfClientID(): string | null {
    return this.wolfClientID;
}
```

**Frontend (TODO):**
Modify MoonlightStreamViewer.tsx to pass wolf_client_id to auto-join API:
```typescript
setTimeout(async () => {
  try {
    // Get Wolf client_id from stream instance
    const wolfClientID = streamRef.current?.getWolfClientID();

    const apiClient = helixApi.getApiClient();
    const response = await apiClient.v1ExternalAgentsAutoJoinLobbyCreate(
      sessionId,
      { wolf_client_id: wolfClientID }  // Pass it to backend
    );

    if (response.status === 200) {
      console.log('[AUTO-JOIN] ✅ Successfully auto-joined lobby');
    }
  } catch (err) {
    console.error('[AUTO-JOIN] Error:', err);
  }
}, 1000);
```

## Proposed Fix: Deferred Auto-Join API Endpoint (DEPRECATED - See above)

### Approach: Post-Connection Auto-Join

Create a new API endpoint that frontend calls AFTER moonlight-web has connected:

**Endpoint:** `POST /api/v1/external-agents/{sessionID}/auto-join-lobby`

**Flow:**
1. User clicks "Live Stream" button
2. Frontend loads moonlight-web iframe with lobby context
3. moonlight-web connects to Wolf UI → session created
4. Frontend detects connection complete (iframe load event)
5. Frontend calls auto-join endpoint
6. Backend finds moonlight session and calls `JoinLobby()`
7. User is automatically switched to their lobby

**Implementation:**

```go
// In external_agent_handlers.go

// autoJoinExternalAgentLobby handles POST /api/v1/external-agents/{sessionID}/auto-join-lobby
func (apiServer *HelixAPIServer) autoJoinExternalAgentLobby(res http.ResponseWriter, req *http.Request) {
    user := getRequestUser(req)
    if user == nil {
        http.Error(res, "unauthorized", http.StatusUnauthorized)
        return
    }

    vars := mux.Vars(req)
    sessionID := vars["sessionID"]

    // Get session
    session, err := apiServer.Controller.Options.Store.GetSession(req.Context(), sessionID)
    if err != nil {
        http.Error(res, "session not found", http.StatusNotFound)
        return
    }

    // Authorize user
    if session.Owner != user.ID && !user.Admin {
        http.Error(res, "forbidden", http.StatusForbidden)
        return
    }

    // Get lobby ID and PIN from session metadata
    lobbyID := session.Metadata.WolfLobbyID
    lobbyPIN := session.Metadata.WolfLobbyPIN
    if lobbyID == "" {
        http.Error(res, "no lobby associated with this session", http.StatusBadRequest)
        return
    }

    // Call the auto-join function (restored from removed commit)
    err = apiServer.autoJoinWolfLobby(req.Context(), lobbyID, lobbyPIN)
    if err != nil {
        log.Error().Err(err).
            Str("session_id", sessionID).
            Str("lobby_id", lobbyID).
            Msg("Failed to auto-join lobby")
        http.Error(res, fmt.Sprintf("failed to join lobby: %v", err), http.StatusInternalServerError)
        return
    }

    log.Info().
        Str("session_id", sessionID).
        Str("lobby_id", lobbyID).
        Msg("✅ Successfully auto-joined lobby")

    res.Header().Set("Content-Type", "application/json")
    json.NewEncoder(res).Encode(map[string]interface{}{
        "success":  true,
        "lobby_id": lobbyID,
    })
}

// Restore autoJoinWolfLobby function from commit 46f27524a
// (same code as before, just called at different time)
```

**Frontend changes:**

```typescript
// In MoonlightWebPlayer.tsx

useEffect(() => {
  const handleLoad = async () => {
    setIsLoading(false);
    onConnectionChange?.(true);

    // Auto-join lobby if in lobbies mode
    if (wolfLobbyId) {
      console.log('[AUTO-JOIN] Connection established, triggering auto-join');
      try {
        const response = await fetch(`/api/v1/external-agents/${sessionId}/auto-join-lobby`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${account.user?.token || ''}`,
          },
        });

        if (response.ok) {
          console.log('[AUTO-JOIN] Successfully auto-joined lobby');
        } else {
          console.warn('[AUTO-JOIN] Failed to auto-join lobby:', await response.text());
        }
      } catch (err) {
        console.error('[AUTO-JOIN] Error calling auto-join:', err);
      }
    }
  };

  const iframe = iframeRef.current;
  if (iframe) {
    iframe.addEventListener('load', handleLoad);
    return () => iframe.removeEventListener('load', handleLoad);
  }
}, [sessionId, wolfLobbyId, account.user?.token]);
```

### Advantages of This Approach

1. **Correct timing:** Auto-join happens AFTER session exists
2. **Minimal frontend changes:** Just one API call after iframe loads
3. **Reuses existing infrastructure:** `wolf.Client.JoinLobby()` already works
4. **Graceful degradation:** If auto-join fails, user can still join manually
5. **Secure:** Uses existing RBAC authorization
6. **Debugging:** Clear separation of concerns (connection vs auto-join)

### Testing Plan

1. **Verify session exists before auto-join:**
   ```bash
   # After clicking "Live Stream", check Wolf sessions
   docker compose -f docker-compose.dev.yaml exec api \
     curl --unix-socket /var/run/wolf/wolf.sock \
     http://localhost/api/v1/sessions | jq '.sessions[] | select(.app_id == "134906179")'
   ```

2. **Test auto-join API call:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/external-agents/ses_xxx/auto-join-lobby \
     -H "Authorization: Bearer $TOKEN"
   ```

3. **Verify lobby join in Wolf logs:**
   ```bash
   docker compose -f docker-compose.dev.yaml logs --tail 50 -f wolf | grep -i "lobby.*join"
   ```

### Next Steps

1. Restore `autoJoinWolfLobby()` function (copy from commit 46f27524a)
2. Add new API route: `POST /api/v1/external-agents/{sessionID}/auto-join-lobby`
3. Update frontend to call auto-join after iframe loads
4. Add debugging logs to track timing
5. Test with dev environment
6. Deploy to production
