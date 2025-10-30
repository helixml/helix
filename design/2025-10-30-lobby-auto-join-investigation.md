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

## Summary

Auto-join is currently **NOT IMPLEMENTED** in the code. The frontend connects to Wolf UI (lobby browser), but doesn't pass lobby context to moonlight-web. To implement auto-join, we need to:

1. Understand desired UX (which approach to use)
2. Enhance URL parameters to include lobby context
3. Modify moonlight-web to call Wolf lobby join API after connection
4. Handle security (PIN in URL vs tokens)
5. Test multi-device scenarios

The "client_id/session_id confusion" likely refers to the difference between:
- Helix session IDs (external agent sessions)
- Wolf session IDs (streaming pipeline IDs)
- moonlight_session_id (connected client IDs)

These need to be mapped correctly for auto-join to work.
