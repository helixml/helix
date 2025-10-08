# Moonlight Streaming - Issues Found During Review

## Critical Issues

### ❌ ISSUE #1: WolfLobbyID Missing from SessionMetadata (FIXED)

**Problem**: SessionMetadata had `WolfLobbyPIN` but not `WolfLobbyID`

**Impact**:
- Frontend expects `session.data.wolf_lobby_id`
- Backend couldn't return proper lobby ID in token response

**Fix Applied**:
```go
// api/pkg/types/types.go:343
WolfLobbyID string `json:"wolf_lobby_id,omitempty"` // Added
WolfLobbyPIN string `json:"wolf_lobby_pin,omitempty"` // Existing
```

**Status**: ✅ FIXED

---

### ❌ ISSUE #2: WolfLobbyID Not Saved to Session (FIXED)

**Problem**: `session_handlers.go` only saved WolfLobbyPIN, not WolfLobbyID

**Impact**:
- WolfLobbyID field would always be empty
- Streaming couldn't identify which lobby to connect to

**Fix Applied**:
```go
// api/pkg/server/session_handlers.go:433-447
session.Metadata.WolfLobbyID = agentResp.WolfLobbyID  // Added
session.Metadata.WolfLobbyPIN = agentResp.WolfLobbyPIN // Existing
```

**Status**: ✅ FIXED

---

### ❌ ISSUE #3: Wrong Field in Token Response (FIXED)

**Problem**: Line 92 set `WolfLobbyID` to PIN value instead of actual lobby ID

**Before**:
```go
WolfLobbyID: session.Metadata.WolfLobbyPIN, // WRONG!
```

**After**:
```go
WolfLobbyID: session.Metadata.WolfLobbyID, // Correct
```

**Status**: ✅ FIXED

---

## Potential Issues (Need Verification)

### ⚠️ ISSUE #4: Dynamic Import Paths

**File**: `frontend/src/components/external-agent/MoonlightStreamViewer.tsx:66`

**Code**:
```typescript
const streamModule = await import('/moonlight-static/stream/index.js');
```

**Potential Problem**:
- Vite serves assets from `/` (publicDir = 'assets')
- Path `/moonlight-static/` might work, but could have CORS issues
- Dynamic imports may need relative paths or explicit base URL

**Test Needed**:
```bash
curl http://localhost:3000/moonlight-static/stream/index.js
```

**Possible Fix** (if 404):
```typescript
const streamModule = await import('/assets/moonlight-static/stream/index.js');
// or
const streamModule = await import('./moonlight-static/stream/index.js');
```

**Status**: ⚠️ NEEDS TESTING

---

### ⚠️ ISSUE #5: Moonlight Web Credentials

**File**: `moonlight-web-config/config.json`

**Code**:
```json
{
  "credentials": "helix"
}
```

**Problem**:
- Hardcoded insecure credential
- moonlight-web requires authentication matching this password
- Frontend doesn't send credentials in current implementation

**Impact**:
- When MoonlightStreamViewer calls `getApi('/moonlight/api')`, moonlight-web will require auth
- Current code doesn't pass credentials

**Fix Needed**:
```typescript
// In MoonlightStreamViewer.tsx, when creating API:
const api = await getApi('/moonlight/api');
api.credentials = 'helix'; // Must match config.json
```

**Status**: ⚠️ NEEDS FIX

---

### ⚠️ ISSUE #6: Missing Wolf Host Pairing

**File**: `moonlight-web-config/data.json`

**Code**:
```json
{
  "hosts": [{
    "address": "wolf",
    "http_port": 47989,
    "client_private_key": null,  // Not paired!
    "client_certificate": null,
    "server_certificate": null
  }]
}
```

**Problem**:
- Wolf host is added but not paired
- moonlight-web requires pairing before streaming
- Pairing generates certificates for authentication

**Impact**:
- First stream attempt will fail with "Host not paired"
- User must manually pair through moonlight-web UI

**Fixes**:

**Option A: Manual Pairing** (Quick):
1. Open http://localhost:8080/moonlight/
2. Click on Wolf host
3. Enter pairing PIN from Wolf logs
4. Certificates saved to data.json

**Option B: Auto-Pairing** (Better):
```go
// Add to moonlight_proxy.go or startup initialization
func (apiServer *HelixAPIServer) initializeMoonlightWebHost() {
    // Call moonlight-web API to pair with Wolf
    // POST /api/host/pair with Wolf's pairing PIN
    // Saves certificates automatically
}
```

**Status**: ⚠️ NEEDS MANUAL PAIRING (or auto-pair implementation)

---

### ⚠️ ISSUE #7: Unused Files

**Files Created But Not Used**:
- `frontend/src/hooks/useMoonlightStream.ts` - Custom hook (368 lines)
- `frontend/src/components/external-agent/MoonlightWebPlayer.tsx` - Iframe fallback

**Problem**:
- Created during initial implementation
- Superseded by direct module import approach
- Dead code in repository

**Fix**:
```bash
git rm frontend/src/components/external-agent/MoonlightWebPlayer.tsx
git rm frontend/src/hooks/useMoonlightStream.ts
```

**Or**: Keep for documentation/fallback purposes

**Status**: ⚠️ CLEANUP RECOMMENDED (not critical)

---

### ⚠️ ISSUE #8: SQL Migrations Not Used

**Files Created**:
- `api/pkg/store/migrations/0003_add_streaming_rbac.up.sql`
- `api/pkg/store/migrations/0003_add_streaming_rbac.down.sql`

**Problem**:
- Helix uses GORM AutoMigrate, not SQL migrations
- These files won't be executed
- Potential confusion for future developers

**Fix**:
```bash
git rm api/pkg/store/migrations/0003_add_streaming_rbac.*.sql
```

**Note**: Types are correctly added to AutoMigrate in `postgres.go:161-162`

**Status**: ⚠️ CLEANUP RECOMMENDED (not critical)

---

## Minor Issues

### 🔹 ISSUE #9: Missing Error Handling

**File**: `MoonlightStreamViewer.tsx:69`

**Code**:
```typescript
return { Stream: streamModule.Stream, getApi: apiModule.getApi };
```

**Problem**:
- No validation that Stream class exists in module
- No validation that getApi function exists
- Could fail silently if moonlight-web build changes

**Fix**:
```typescript
if (!streamModule.Stream || !apiModule.getApi) {
  throw new Error('Moonlight modules incompatible - missing Stream or getApi');
}
```

**Status**: 🔹 MINOR (good practice)

---

### 🔹 ISSUE #10: Hardcoded Moonlight IDs

**File**: `MoonlightStreamViewer.tsx:41-42`

**Code**:
```typescript
hostId = 0,   // Wolf is always host 0
appId = 1,    // Default app
```

**Problem**:
- Assumes Wolf is always first host (ID 0)
- Assumes default app ID is 1
- Won't work if moonlight-web config changes

**Better Approach**:
- Map `wolfLobbyId` to moonlight app ID dynamically
- Or: Pre-configure mapping in database/config

**Status**: 🔹 MINOR (works for single-host setup)

---

### 🔹 ISSUE #11: Team/Role Grants Not Implemented

**File**: `streaming_access_handlers.go:198-206`

**Code**:
```go
// Check 3: Team grant
// TODO: Implement GetUserTeams when team system is ready

// Check 4: Role grant
// TODO: Implement GetUserRoles when role system is ready
```

**Impact**:
- Only owner and direct user grants work
- Team/role-based sharing not functional
- Documented in design but not implemented

**Status**: 🔹 DOCUMENTED TODO (planned feature)

---

### 🔹 ISSUE #12: Access Level Not Enforced in moonlight-web

**Problem**:
- Token contains access_level (view/control/admin)
- moonlight-web doesn't check X-Helix-Access-Level header
- All users get full control regardless of access level

**Impact**:
- "view" access users can still control input (security issue!)

**Fix Needed**:
Modify moonlight-web streamer to:
1. Check X-Helix-Access-Level header from proxy
2. Disable input data channels for "view" level
3. Only send video/audio tracks, not accept input

**Status**: 🔹 SECURITY TODO (documented in design)

---

## Summary

### Critical (Must Fix Before Testing)
- ✅ Issue #1: WolfLobbyID in SessionMetadata - FIXED
- ✅ Issue #2: WolfLobbyID not saved - FIXED
- ✅ Issue #3: Wrong field in response - FIXED
- ⚠️ Issue #5: Credentials not passed to moonlight-web - NEEDS FIX
- ⚠️ Issue #6: Wolf host not paired - NEEDS MANUAL PAIRING

### Testing Needed
- ⚠️ Issue #4: Dynamic import paths - NEEDS VERIFICATION

### Cleanup (Optional)
- Issue #7: Unused files (useMoonlightStream.ts, MoonlightWebPlayer.tsx)
- Issue #8: SQL migration files not used

### Future Work
- Issue #11: Team/role grants (documented TODO)
- Issue #12: Access level enforcement in moonlight-web

### Minor
- Issue #9: Missing validation
- Issue #10: Hardcoded IDs

---

## Action Items

### Immediate (Before Testing)

1. ✅ Add WolfLobbyID to SessionMetadata
2. ✅ Save WolfLobbyID in session_handlers.go
3. ✅ Fix token response to use correct lobby ID
4. ⚠️ **Add credentials to moonlight-web API calls**
5. ⚠️ **Pair Wolf host in moonlight-web** (manual or auto)
6. Test dynamic import paths work in browser

### Before Production

1. Implement team/role grant checking
2. Add access level enforcement in moonlight-web
3. Remove unused files
4. Remove SQL migration files
5. Change default credentials from "helix"
6. Add TURN server configuration

---

**Review Date**: 2025-10-08
**Reviewed By**: Claude Code
**Total Issues Found**: 12
**Critical Fixed**: 3/3
**Remaining Critical**: 2
**Status**: Needs credential fix and pairing before testing
