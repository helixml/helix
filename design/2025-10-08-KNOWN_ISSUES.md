# Known Issues

## Screenshots Not Working with Lobbies

**Symptom:**
- External agent sessions return 500 error for screenshots
- API error: "Screenshot server returned error status=500"
- Container logs: "Failed to capture screenshot: exit status 1"
- Wayland errors: "[wlr] [render/swapchain.c:98] No free output buffer slot"

**Root Cause:**
- Wolf-UI lobbies create their own Wayland compositor (wayland-2)
- Screenshot server points to wayland-2
- But Sway/Zed runs on wayland-1
- Screenshot server started before Sway, picked wrong display
- Startup ordering issue with lobbies vs old app-based approach

**Workaround:**
- Use Moonlight streaming to view agent sessions (works perfectly)
- Screenshots are for preview only - not critical functionality

**Potential Fixes:**
1. **Update screenshot server to use WAYLAND_DISPLAY=wayland-1**
   - Modify screenshot-server startup script
   - Or pass WAYLAND_DISPLAY env var explicitly

2. **Fix startup script ordering**
   - Ensure Sway starts before screenshot server
   - Or make screenshot server detect correct Wayland display

3. **Use Wolf's built-in screenshot API** (if available)
   - Lobbies may have screenshot capability via Wolf API
   - Check `/api/v1/sessions/{id}/screenshot` endpoint

**Priority:** Medium - Screenshots nice-to-have, streaming is primary access method

**Temporary:** Screenshots work for old app-based sessions, only broken for new lobby-based sessions

---

## Pairing Request