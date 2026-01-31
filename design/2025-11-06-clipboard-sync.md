# Clipboard Synchronization for Remote Desktops

**Date:** 2025-11-06
**Status:** Design + Implementation
**Context:** Moonlight protocol doesn't provide clipboard sync, making copy/paste between local and remote desktops very annoying

## Problem Statement

Users streaming Wayland desktops via Moonlight cannot copy/paste between their local browser and the remote desktop. This is a critical workflow issue:
- Can't copy code snippets from browser docs into Zed
- Can't copy error messages from Zed to paste in local Slack
- Can't copy URLs from local browser to remote Firefox
- Forces manual retyping or awkward workarounds

## Goals

1. **Enable bidirectional clipboard sync** between browser and remote Wayland desktop
2. **Work within browser security model** (Clipboard API requires user gestures)
3. **Make sync operations explicit and obvious** to user
4. **No automatic background polling** that could raise security concerns
5. **Simple, intuitive UX** with keyboard shortcuts for power users

## Non-Goals

- RDP-style automatic bidirectional sync (browser API doesn't allow this)
- Clipboard format preservation (rich text, images) - text-only for MVP
- Clipboard history/manager functionality

## UX Design

### Approach: Hybrid Auto/Manual Sync

**Automatic Remote → Local Sync:**
- Polls remote clipboard every 2 seconds when streaming is active
- Automatically writes to local browser clipboard when remote clipboard changes
- Silent operation - no notifications unless errors occur
- Covers 90% of use case: copying from Zed → pasting in browser/Slack/etc

**Smart Local → Remote Sync on Paste Keystrokes:**
- Intercepts paste keystrokes in browser:
  - **Ctrl+V** / **Cmd+V** - Regular paste (Zed, browser, most apps)
  - **Ctrl+Shift+V** / **Cmd+Shift+V** - Terminal paste
- Reads local clipboard and syncs to remote
- Forwards keystroke to remote application
- Application executes normal paste with synced clipboard
- **Single keystroke** for paste to remote!
- No separate button needed - just use standard paste shortcuts

**Cross-platform Support:**
- Windows/Linux: Ctrl+V, Ctrl+Shift+V
- Mac: Cmd+V, Cmd+Shift+V
- Auto-detects platform and handles both

**Why this approach is optimal:**
- Browser Clipboard API requires user gesture for `navigator.clipboard.readText()` - paste keystrokes provide that
- Auto remote→local covers primary use case (copying FROM remote)
- Paste interception makes pasting TO remote seamless (single keystroke)
- No cognitive overhead - just use standard paste shortcuts everywhere
- Works consistently across all remote applications (terminals, Zed, browsers, etc.)

**Visual Design:**
```
┌───────────────────────────────────────────────────┐
│ [🎥] [🔊] [🔄] [⛶]                                │
│       Video Audio Refresh Fullscreen              │
└───────────────────────────────────────────────────┘

No clipboard buttons needed!
- Ctrl+C in remote → auto-syncs → Ctrl+V locally
- Ctrl+Shift+V in browser → syncs → pastes in remote terminal
```

**User Experience:**
- Completely transparent - clipboard "just works"
- No new UI elements to learn
- No manual sync buttons
- Use standard shortcuts everywhere

## Technical Architecture

### Component Overview

```
┌──────────────────┐
│  Browser         │
│  Clipboard API   │
└────────┬─────────┘
         │ navigator.clipboard.readText/writeText
         │
┌────────▼─────────────────────┐
│  MoonlightStreamViewer.tsx   │
│  - Copy/Paste buttons        │
│  - Keyboard shortcuts        │
│  - Toast notifications       │
└────────┬─────────────────────┘
         │ HTTP POST/GET /api/v1/sessions/{id}/clipboard
         │
┌────────▼─────────────────────┐
│  Helix API                   │
│  session_handlers.go         │
│  - RBAC checks               │
│  - Proxy to container        │
└────────┬─────────────────────┘
         │ HTTP to container hostname:9876/clipboard
         │
┌────────▼─────────────────────┐
│  screenshot-server (Go)      │
│  - GET /clipboard            │
│  - POST /clipboard           │
│  - wl-paste / wl-copy cmds   │
└────────┬─────────────────────┘
         │ Wayland protocol
         │
┌────────▼─────────────────────┐
│  Wayland Compositor (Sway)   │
│  - System clipboard          │
└──────────────────────────────┘
```

### 1. Container Side: screenshot-server

**New Dependencies:**
```dockerfile
# In Dockerfile.sway-helix
RUN apt-get install -y wl-clipboard
```

**New Endpoints in screenshot-server:**

```go
// GET /clipboard - Returns current Wayland clipboard content
func handleGetClipboard(w http.ResponseWriter, r *http.Request) {
    cmd := exec.Command("wl-paste", "--no-newline")
    cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY=wayland-1")

    output, err := cmd.Output()
    if err != nil {
        // Empty clipboard or error
        w.Header().Set("Content-Type", "text/plain")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(""))
        return
    }

    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    w.Write(output)
}

// POST /clipboard - Sets Wayland clipboard content
func handleSetClipboard(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read request body", http.StatusBadRequest)
        return
    }

    cmd := exec.Command("wl-copy")
    cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY=wayland-1")
    cmd.Stdin = bytes.NewReader(body)

    if err := cmd.Run(); err != nil {
        http.Error(w, "Failed to set clipboard", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}
```

### 2. API Side: session_handlers.go

**New Endpoints:**

```go
// @Summary Get session clipboard content
// @Description Fetch current clipboard content from remote desktop
// @Tags sessions
// @Produce plain
// @Param id path string true "Session ID"
// @Success 200 {string} string "Clipboard text content"
// @Router /api/v1/sessions/{id}/clipboard [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSessionClipboard(res http.ResponseWriter, req *http.Request) {
    user := getRequestUser(req)
    sessionID := chi.URLParam(req, "id")

    // Get session and check access
    session, err := apiServer.store.GetSession(ctx, sessionID)
    if err != nil {
        http.Error(res, "Session not found", http.StatusNotFound)
        return
    }

    // Check authorization
    if err := apiServer.authorizeUserToSession(ctx, user, session); err != nil {
        http.Error(res, "Unauthorized", http.StatusForbidden)
        return
    }

    // Find container hostname from session
    containerName, err := apiServer.externalAgent.FindContainerBySessionID(ctx, sessionID)
    if err != nil {
        http.Error(res, "Container not found", http.StatusNotFound)
        return
    }

    // Fetch from screenshot server
    clipboardURL := fmt.Sprintf("http://%s:9876/clipboard", containerName)
    resp, err := http.Get(clipboardURL)
    if err != nil {
        http.Error(res, "Failed to fetch clipboard", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Stream response
    res.Header().Set("Content-Type", "text/plain")
    io.Copy(res, resp.Body)
}

// @Summary Set session clipboard content
// @Description Send clipboard content to remote desktop
// @Tags sessions
// @Accept plain
// @Param id path string true "Session ID"
// @Param clipboard body string true "Clipboard text to set"
// @Success 200
// @Router /api/v1/sessions/{id}/clipboard [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) setSessionClipboard(res http.ResponseWriter, req *http.Request) {
    // Similar structure but POST to screenshot server
}
```

### 3. Frontend: MoonlightStreamViewer.tsx

**New State:**
```typescript
const [clipboardSyncing, setClipboardSyncing] = useState(false)
const lastRemoteClipboard = useRef<string>('')
```

**Auto-Sync Remote → Local (polling every 2s):**
```typescript
useEffect(() => {
    if (!isConnected || !sessionId) return

    const syncInterval = setInterval(async () => {
        try {
            const apiClient = helixApi.getApiClient()
            const response = await apiClient.v1SessionsClipboardDetail(sessionId)
            const remoteClipboard = response.data || ''

            // Only update if clipboard changed (avoid unnecessary writes)
            if (remoteClipboard !== lastRemoteClipboard.current) {
                await navigator.clipboard.writeText(remoteClipboard)
                lastRemoteClipboard.current = remoteClipboard
                console.log(`[Clipboard] Auto-synced from remote (${remoteClipboard.length} chars)`)
            }
        } catch (err) {
            // Silent failure - don't spam user with clipboard sync errors
            console.warn('[Clipboard] Auto-sync failed:', err)
        }
    }, 2000) // Poll every 2 seconds

    return () => clearInterval(syncInterval)
}, [isConnected, sessionId, helixApi])
```

**Smart Paste Keystroke Interception:**
```typescript
useEffect(() => {
    if (!isConnected || !containerRef.current) return

    const handleKeyDown = async (event: KeyboardEvent) => {
        // Detect paste keystrokes (cross-platform)
        const isCtrlV = event.ctrlKey && !event.shiftKey && event.code === 'KeyV'
        const isCmdV = event.metaKey && !event.shiftKey && event.code === 'KeyV'
        const isCtrlShiftV = event.ctrlKey && event.shiftKey && event.code === 'KeyV'
        const isCmdShiftV = event.metaKey && event.shiftKey && event.code === 'KeyV'

        const isPasteKeystroke = isCtrlV || isCmdV || isCtrlShiftV || isCmdShiftV

        if (isPasteKeystroke) {
            event.preventDefault()
            event.stopPropagation()

            try {
                // 1. Read local clipboard (user gesture satisfied by keypress)
                const clipboardText = await navigator.clipboard.readText()

                // 2. Sync to remote clipboard
                const apiClient = helixApi.getApiClient()
                await apiClient.v1SessionsClipboardCreate(sessionId, clipboardText)

                console.log(`[Clipboard] Synced to remote (${clipboardText.length} chars)`)

                // 3. Send BOTH Ctrl+V AND Ctrl+Shift+V to remote (Linux)
                // This ensures paste works in BOTH regular apps and terminals:
                // - Regular apps (Zed, Firefox) ignore Ctrl+Shift+V, respond to Ctrl+V
                // - Terminals ignore Ctrl+V, respond to Ctrl+Shift+V
                const input = streamRef.current?.getInput()
                if (input) {
                    // Send Ctrl+V (for regular apps)
                    const ctrlV = new KeyboardEvent('keydown', {
                        code: 'KeyV',
                        key: 'v',
                        ctrlKey: true,
                        shiftKey: false,
                        metaKey: false,
                        bubbles: true,
                        cancelable: true
                    })
                    input.onKeyDown(ctrlV)

                    // Send Ctrl+Shift+V (for terminals)
                    const ctrlShiftV = new KeyboardEvent('keydown', {
                        code: 'KeyV',
                        key: 'v',
                        ctrlKey: true,
                        shiftKey: true,
                        metaKey: false,
                        bubbles: true,
                        cancelable: true
                    })
                    input.onKeyDown(ctrlShiftV)
                }
            } catch (err) {
                console.error('[Clipboard] Failed to sync for paste:', err)
            }

            return
        }

        // Other keyboard shortcuts handled by existing code...
    }

    containerRef.current.addEventListener('keydown', handleKeyDown)
    return () => containerRef.current?.removeEventListener('keydown', handleKeyDown)
}, [isConnected, sessionId, helixApi])
```

**Paste keystroke translation:**
- **Mac Cmd+V / Cmd+Shift+V** → Sends **Ctrl+V + Ctrl+Shift+V** to remote Linux
- **Windows/Linux Ctrl+V / Ctrl+Shift+V** → Sends **Ctrl+V + Ctrl+Shift+V** to remote Linux

**Why send both Ctrl+V and Ctrl+Shift+V?**
- Regular apps (Zed, Firefox) **ignore** Ctrl+Shift+V, **respond** to Ctrl+V
- Terminals **ignore** Ctrl+V (used for shell control), **respond** to Ctrl+Shift+V
- Sending both ensures paste works regardless of focused application!

**No toolbar button needed** - clipboard sync is completely transparent!

## Security Considerations

1. **RBAC Enforcement:**
   - Clipboard endpoints check session access via `authorizeUserToSession`
   - Users can only access clipboard for sessions they own or have access to

2. **Browser Security Model:**
   - `navigator.clipboard.readText()` requires user gesture (click/keyboard)
   - Our approach naturally complies by using button clicks

3. **Data Validation:**
   - Clipboard content is text-only (no code execution risk)
   - Size limits could be added if needed (e.g., 1MB max)

4. **No Background Polling:**
   - Explicit user actions prevent invisible data exfiltration
   - Clear when clipboard sync is happening

## Testing Plan

**Manual Testing:**
1. Start exploratory session
2. Copy text in Zed (Ctrl+C)
3. Click "Copy from Remote" button in Helix UI
4. Paste in local text editor (Ctrl+V) - should see Zed text
5. Copy text locally
6. Click "Paste to Remote" button
7. Paste in Zed (Ctrl+V) - should see local text
8. Test keyboard shortcuts (Ctrl+Shift+C, Ctrl+Shift+V)
9. Test with different text sizes (small, large)
10. Test with empty clipboard

**Error Cases:**
- Container not running - should show error toast
- Session not found - should show error toast
- Clipboard access denied by browser - should show helpful error
- Empty clipboard - should succeed silently

## Future Enhancements

**Out of scope for MVP:**
1. **Rich text/HTML clipboard** - would need format negotiation
2. **Image clipboard** - would need base64 encoding and size limits
3. **Automatic sync** - could add polling if browser APIs improve
4. **Clipboard history** - could cache last N clipboard items
5. **Clipboard notifications** - show preview of what was copied

## Implementation Checklist

- [x] Install wl-clipboard in Dockerfile.sway-helix
- [x] Add clipboard endpoints to screenshot-server (GET/POST /clipboard)
- [x] Add clipboard proxy endpoints to external_agent_handlers.go
- [x] Add Swagger docs for clipboard endpoints
- [x] Run `./stack update_openapi` to regenerate TypeScript client
- [x] Implement auto-sync remote → local (polling every 2s)
- [x] Implement paste keystroke interception (Ctrl+V, Cmd+V, Ctrl+Shift+V, Cmd+Shift+V)
- [x] Support both text and image clipboard formats
- [x] Cross-platform keystroke handling (Mac Cmd → Linux Ctrl)
- [x] Send both Ctrl+V and Ctrl+Shift+V to handle all remote apps
- [x] Rebuild Sway image: `./stack build-sway`
- [ ] Manual testing of full workflow
- [ ] Test text copy/paste (Zed → browser → Zed)
- [ ] Test image copy/paste (screenshots)

## Open Questions

1. **Clipboard size limits?**
   - Browser clipboard API doesn't have hard limits
   - Could add 1MB limit to prevent abuse
   - **Decision:** Start without limits, add if needed

2. **What if wl-paste/wl-copy fail?**
   - Return empty string for GET
   - Return 500 error for POST
   - **Decision:** Graceful degradation for GET, error for POST

3. **Should we show clipboard preview in UI?**
   - Could show first 50 chars in tooltip
   - **Decision:** Not for MVP, could add later

4. **Handle clipboard formats other than text?**
   - wl-paste supports `-t <type>` for MIME types
   - **Decision:** Text-only for MVP
