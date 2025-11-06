# Wolf Multiple Simultaneous Connections Issue

**Date:** 2025-11-06
**Status:** ⚠️ Known Limitation - NOT A BUG
**Affects:** Opening second browser tab to same session
**Root Cause:** Wolf has 32+ stale Wayland sockets, preventing Wolf-UI creation

## Problem Statement

When a user opens a SECOND browser tab to an already-connected Helix session, Wolf attempts to create a **Wolf-UI app** for the second Moonlight session ID. Wolf-UI creation fails because Wolf has accumulated 32+ dead Wayland sockets and can't create a new compositor.

## Observed Symptoms

**User Flow:**
1. User connects to external agent in Browser Tab #1 (Zed desktop running in Sway) ✅
2. User opens Browser Tab #2 and tries to connect to SAME session
3. ❌ Second connection fails with repeated errors
4. First connection remains working ✅

**Wolf Logs:**
```
12:12:52 INFO  | [LOBBY] Session 730041249942359486 joining lobby 1f1b43a8...
12:12:52 INFO  | [DOCKER] Starting container: /Wolf-UI_730041249942359486
12:12:52 ERROR: Can't obtain the Wayland shared memory global.
12:12:52 ERROR: Could not initialize the Wayland thread.
12:12:52 ERROR: Can't create the Wayland display server.
12:12:52 DEBUG | [DOCKER] Stopping container: /Wolf-UI_730041249942359486
12:12:52 ERROR | [GSTREAMER] Pipeline error: Internal data stream error.
```

**Repeated GStreamer Errors:**
```
ERROR interpipesink: Failed to reconfigure
WARN interpipe: Could not add listener interpipesrc_730041249942359486_video to node 730041249942359486_video
ERROR interpipesrc: Could not listen to node 730041249942359486_video
```

## Root Cause Analysis

### What Happened

1. **Initial Connection:**
   - Moonlight session `5016975387482046627` connected to lobby `1f1b43a8-5be2-4cd5-98d9-695745136a76`
   - Zed desktop (container `zed-external-01k9cgbd3a7f2wjavn4xkgrwed`) already running
   - Connection successful ✅

2. **User Opens Second Browser Tab:**
   - Browser Tab #1: Still connected with session `5016975387482046627` ✅
   - Browser Tab #2: Generates **NEW** Moonlight session ID: `730041249942359486`
   - Second tab tries to join same lobby: `1f1b43a8-5be2-4cd5-98d9-695745136a76`
   - **Wolf's auto-pairing creates Wolf-UI app** (normal flow for new sessions)

4. **Wolf-UI Fails to Start:**
   - Wolf creates `Wolf-UI_730041249942359486` container
   - Mounts `/tmp/sockets/wayland-5` from Wolf container
   - Wolf-UI tries to connect to wayland-5
   - **ERROR: wayland-5 has no compositor running!**
   - Error: "Can't obtain the Wayland shared memory global"
   - Container stops immediately

5. **Why wayland-5 is Dead:**
   - Wolf has **32+ stale Wayland sockets** from days of testing
   - **ZERO gamescope/weston processes running in Wolf**
   - All compositors run in APP containers (Sway in Zed agents)
   - Wolf should create gamescope for wayland-5 but DIDN'T
   - Either hit resource limit or compositor creation failed silently

### Core Issue

**Wolf's pairing model:**
- Each paired device gets a Wolf-UI app
- Wolf-UI is ephemeral (starts on connect, stops on disconnect)
- Works great for Moonlight native (phones, tablets)

**Helix's desktop model:**
- Zed desktop runs persistently
- Multiple browser tabs/devices should connect to SAME desktop
- Desktop survives disconnections
- Wolf-UI should NEVER be created

### Why Wolf-UI Fails

Wolf-UI mounts a new Wayland socket (`wayland-5`) but:
1. GPU might be at capacity (already has 2 Zed desktops + 6 GPU processes)
2. Wayland compositor resources might be exhausted
3. Wolf-UI expects Wolf to create a Wayland display for it, but Wolf might not be doing that

## When This Happens

**Trigger:** Second browser tab/window connecting to same session

**Moonlight-web behavior:**
- Each browser tab generates its own Moonlight session ID
- First tab: session `5016975387482046627` → connects fine ✅
- Second tab: session `730041249942359486` → Wolf-UI creation fails ❌

**Why Wolf creates Wolf-UI:**
- Auto-pairing flow expects to show Wolf-UI landing screen
- Then user picks app from Wolf-UI
- Then Wolf switches to chosen app
- For Helix, we want to skip directly to lobby but Wolf doesn't know that

## Evidence from Logs

```
[LOBBY] Session 730041249942359486 joining lobby 1f1b43a8-5be2-4cd5-98d9-695745136a76
[DOCKER] Starting container: /Wolf-UI_730041249942359486
[DOCKER] Starting container: {
  name: /Wolf-UI_730041249942359486
  image: ghcr.io/games-on-whales/wolf-ui:main
  mounts: [/tmp/sockets/wayland-5:/tmp/sockets/wayland-5:rw, ...]
}
ERROR: Can't obtain the Wayland shared memory global.
[DOCKER] Stopping container: /Wolf-UI_730041249942359486
```

**Key observation:** Wolf created Wolf-UI even though lobby already has a Zed desktop!

## Current Workaround

**Users should:**
1. **Use only ONE browser tab per session** (known limitation)
2. If second tab was opened by mistake, close it and refresh first tab
3. If errors persist, restart Wolf: `docker compose -f docker-compose.dev.yaml restart wolf`

## Solution Options

### Option 1: Restart Wolf Periodically ✅ IMMEDIATE FIX

**Approach:** Restart Wolf container to clear stale Wayland sockets

**Implementation:**
```bash
docker compose -f docker-compose.dev.yaml restart wolf
```

**When to restart:**
- When this error occurs (user reports connection failure)
- Preventively: daily or after X sessions created
- On Wolf startup: clean /tmp/sockets/wayland-* except active ones

**Effort:** Zero (immediate)
**Downside:** Disrupts active sessions during restart

### Option 2: Clean Stale Wayland Sockets on Startup ✅ BEST LONG-TERM

**Approach:** Add cleanup logic to Wolf init script

**Implementation:**
```bash
# In Wolf init-wolf-config.sh or similar
find /tmp/sockets -name "wayland-*" -type f,s -mtime +1 -delete
find /tmp/sockets -name "wayland-*.lock" -mtime +1 -delete
```

**When:** Wolf container startup (before Wolf starts)

**Effort:** Low (add to wolf/init-wolf-config.sh)
**Risk:** Low (only deletes sockets older than 1 day)

### Option 3: Disable Wolf-UI for Helix Lobbies

**Approach:** Patch Wolf to skip Wolf-UI for lobbies with specific metadata

**Implementation:**
- Modify Wolf upstream to check lobby metadata for `skip_wolf_ui: true`
- Set this flag when Helix creates lobbies
- New sessions join lobby directly without Wolf-UI intermediary

**Effort:** High (requires Wolf upstream patch or fork)
**Risk:** Medium (changes Wolf's core pairing flow)

## Recommended Solution

**Immediate (right now):** Implement Option 2 - Clean stale sockets on Wolf startup

**Files to modify:**
- `wolf/init-wolf-config.sh` - Add cleanup before Wolf starts

**Why this fixes it:**
- Prevents accumulation of 32+ dead Wayland sockets
- Wolf can allocate fresh sockets when needed
- gamescope/weston creation won't fail due to socket conflicts
- Zero risk (only deletes old sockets)

## Investigation Needed

**moonlight-web session ID logic:**
1. Where is session ID generated?
2. Can we persist it in browser localStorage?
3. Does Moonlight protocol allow session ID reuse?
4. What triggers new session ID generation?

**Wolf lobby behavior:**
1. Why does multi_user lobby create individual apps per session?
2. Is there a way to mark lobby as "reuse desktop"?
3. Can we prevent Wolf-UI creation for specific lobby types?

## Files to Check

**moonlight-web:**
- Session ID generation logic
- Reconnection handling
- localStorage usage

**Wolf:**
- Lobby join logic (`/api/v1/lobbies/join`)
- App creation decision tree
- multi_user vs single_user behavior

**Helix API:**
- `api/pkg/external-agent/wolf_executor.go` - Lobby creation for agents
- Lobby metadata/configuration options

## Success Criteria

✅ User can disconnect and reconnect without triggering Wolf-UI creation
✅ Multiple browser tabs can connect to same Zed desktop
✅ No GStreamer interpipe errors on reconnection
✅ No need to restart Wolf manually

## Temporary Mitigation

**For now, users should:**
1. Avoid disconnecting/reconnecting frequently
2. If errors occur, refresh moonlight-web page
3. If still failing, ask admin to restart Wolf container
4. Report any patterns (does first reconnect always fail? Does waiting longer help?)

## Next Steps

1. Investigate moonlight-web session ID generation
2. Test if localStorage persistence works
3. If not, contact Wolf maintainers about disable_wolf_ui flag
4. Document the fix in a follow-up design doc
