# Moonlight Pairing & Lobby Access - User Experience Design

## Problem Statement

Users currently face confusion between two different workflows:

### 1. **Pairing Moonlight Client** (One-Time Setup)
**What:** Establish trust between Moonlight client and Wolf server
**PIN Direction:** Moonlight client generates PIN → User enters in Helix UI
**Current Issue:** User must copy pairing URL from Wolf logs (impractical)

### 2. **Connecting to Lobby** (Every Use)
**What:** Join a specific agent session or PDE
**PIN Direction:** Helix generates PIN → User enters in Wolf UI (inside Moonlight)
**Current Status:** Working, but UI could be clearer

## Proposed Solution

### Clear UI Distinction

**Top-Level Navigation:**
```
┌─────────────────────────────────────────────────┐
│  Moonlight Access                                │
├─────────────────────────────────────────────────┤
│                                                  │
│  [🔗 Pair New Moonlight Client]  (One-time)     │
│                                                  │
│  [🌀 Connect to Lobby]            (Every use)   │
│                                                  │
└─────────────────────────────────────────────────┘
```

### Workflow 1: Pair New Moonlight Client (One-Time)

**Button Location:** Top navigation or settings page

**User Flow:**
1. User clicks **"Pair New Moonlight Client"** in Helix UI
2. Dialog shows:
   ```
   Pairing Moonlight Client

   Step 1: On your Moonlight client device:
   - Open Moonlight app
   - Click "Add PC Manually"
   - Enter: wolf.helix.local (or IP address)
   - Moonlight will show a 4-digit PIN

   Step 2: Enter the PIN from your Moonlight client:
   [____] PIN from Moonlight

   Pending Pairing Requests:
   - Device from 192.168.1.50 (waiting...)
   - Device from 192.168.1.75 (waiting...)

   [Complete Pairing]
   ```

3. User enters PIN from Moonlight client
4. Helix calls Wolf API to complete pairing
5. Success! Client is now paired and trusted

**Backend:**
- GET `/api/v1/wolf/pairing/pending` - List pending requests
- POST `/api/v1/wolf/pairing/complete` - Complete with PIN

**Frontend:** (Already exists!)
- `MoonlightPairingOverlay.tsx` component
- Just needs to be surfaced in main UI

### Workflow 2: Connect to Lobby (Every Use)

**Context:** User viewing an external agent session or PDE

**User Flow:**
1. User viewing session in Helix UI
2. Sees prominent **blue PIN box**:
   ```
   🔐 Moonlight Access PIN: 4403
   [Copy PIN]

   Launch "Wolf UI" or "Helix Lab 3D" in Moonlight
   → Select this lobby → Enter PIN
   ```

3. User:
   - Launches Moonlight
   - Selects "Wolf UI" or "Helix Lab 3D"
   - Sees lobby list
   - Clicks desired lobby
   - **Enters PIN from Helix UI**
   - Jumps into lobby!

**Current Status:** ✅ Working (just fixed field path)

## Implementation Plan

### Phase 1: Add "Pair Moonlight Client" Button ✅ (Backend Exists)

**Location Options:**
a) **Settings page** - "Moonlight Devices" section
b) **Top navigation** - "Connect Moonlight" menu item
c) **PDE list** - "Setup Moonlight" banner at top

**Recommended:** Settings page (keeps it out of the way for already-paired users)

**Implementation:**
1. Add route to Settings page for Moonlight devices
2. Reuse existing `MoonlightPairingOverlay` component
3. Add trigger button: "Pair New Device"
4. Poll pending requests every 2 seconds while dialog open
5. Show list of pending requests with timestamps
6. User selects request and enters PIN from their device

### Phase 2: Clarify Lobby Access UI ✅ (Already Done)

**Session Page:**
- ✅ Large blue PIN box
- ✅ Clear instructions
- ✅ Copy button
- ✅ Only visible to owner/admin

**Improvements Made:**
- Fixed field path (config not metadata)
- Added prominent styling
- Clear workflow explanation

### Phase 3: Improve Wolf UI Instructions

**In Wolf UI / Helix Lab 3D:**
- Show help text: "Get lobby PINs from Helix UI"
- Link/QR code to Helix UI? (if accessible from same network)

## API Endpoints

### Pairing (One-Time)

**Wolf API:**
```
GET /api/v1/pair/pending
Response: {
  "success": true,
  "requests": [
    {
      "pair_secret": "abc123...",
      "client_ip": "192.168.1.50"
    }
  ]
}

POST /api/v1/pair/client
Body: {
  "pair_secret": "abc123...",
  "pin": "1234"
}
Response: {"success": true}
```

**Helix API Wrapper:**
```
GET /api/v1/wolf/pairing/pending (already exists)
POST /api/v1/wolf/pairing/complete (already exists)
```

### Lobby Access (Every Use)

**Wolf API:**
```
GET /api/v1/lobbies
Response: {
  "success": true,
  "lobbies": [...]
}

POST /api/v1/lobbies/join
Body: {
  "lobby_id": "...",
  "moonlight_session_id": "...",
  "pin": [4, 4, 0, 3]
}
```

**Helix:** PINs displayed in session/PDE UI

## User Documentation

### For New Users

**First Time Setup:**
1. **Pair Your Device (One-Time)**
   - In Helix: Settings → Moonlight Devices → "Pair New Device"
   - In Moonlight: Add PC → Enter Wolf server address
   - Moonlight shows PIN
   - Enter PIN in Helix dialog
   - ✅ Device paired!

2. **Access Agent Sessions**
   - Create/view agent session in Helix
   - See lobby PIN in blue box
   - Copy PIN
   - Launch "Wolf UI" or "Helix Lab 3D" in Moonlight
   - Select lobby, enter PIN
   - ✅ You're in!

### For Returning Users

**Already Paired:** Just launch Wolf UI/Helix Lab 3D and enter lobby PINs

## Next Steps

1. ✅ Backend pairing API - Already exists
2. ✅ Frontend pairing component - Already exists (MoonlightPairingOverlay)
3. ⚠️ Surface pairing UI in main app (add button/page)
4. ✅ Lobby PIN display - Working
5. 📝 Update user documentation

**Recommendation:** Add "Pair Moonlight" button to Settings page using existing MoonlightPairingOverlay component.

---

**Status:** Backend complete, frontend component exists, just needs UI integration
**Priority:** Medium - improves new user onboarding
**Complexity:** Low - just wire up existing component
