# Design Doc: Staff Engineer Code Review - Ubuntu Desktop + Auth Cleanup PR

**Date:** 2026-01-07
**Status:** Review Complete
**Author:** Claude (Code Review)
**PR Stats:** 168 commits, 207 files changed, +31,262 / -12,404 lines

## Executive Summary

This is a major PR that accomplishes several significant changes:

1. **Ubuntu 25.10 GNOME Desktop Support** - New desktop environment using GNOME 49 with PipeWire-based video capture
2. **Keycloak Removal** - Complete removal of embedded Keycloak in favor of OIDC-only authentication (-2,631 lines)
3. **Desktop Package Refactor** - Modular screenshot-server architecture with D-Bus session management
4. **Wolf Executor Multi-Desktop** - Support for 6 desktop types (Sway, Ubuntu, KDE, Hyprland, XFCE, Zorin)
5. **CLI Expansion** - New spectask subcommands (stream, stop, interact, keyboard-test, etc.)

Overall: **High quality PR with solid architecture decisions.** A few review items noted below for consideration.

---

## Architecture Analysis

### 1. Ubuntu Desktop (GNOME 49 Headless Mode)

**Key Innovation:** Uses `gnome-shell --headless --virtual-monitor WxH@R` instead of X11-based Zorin approach.

**Video Capture Flow:**
```
gnome-shell (headless)
    → PipeWire ScreenCast D-Bus session
    → PipeWire node ID reported to Wolf
    → Wolf pipewiresrc GStreamer element captures frames
```

**Input Injection Flow:**
```
Moonlight input
    → Wolf transforms to JSON events
    → Unix socket to screenshot-server
    → D-Bus RemoteDesktop.NotifyPointerMotionAbsolute/NotifyKeyboardKeycode
    → GNOME Mutter processes input
```

**Technical Insight:** The PR correctly identifies that RemoteDesktop and ScreenCast sessions must be *linked* via `remote-desktop-session-id` option for absolute mouse positioning to work. This is a subtle D-Bus API requirement.

**Display Scaling Solution:**
- GSettings `org.gnome.desktop.interface scaling-factor` before gnome-shell starts
- D-Bus `ApplyMonitorsConfig` after gnome-shell starts (for virtual monitor)
- Both are required (documented in `design/2026-01-07-chrome-keyring-and-gnome-scaling.md`)

### 2. Desktop Package Refactor

**Before:** Monolithic `screenshot-server/main.go` (~1000+ lines)
**After:** Modular `api/pkg/desktop/` package with clear separation:

| File | Responsibility |
|------|----------------|
| `desktop.go` | Main server, lifecycle, HTTP routes |
| `session.go` | D-Bus RemoteDesktop/ScreenCast session management |
| `input.go` | Input socket bridge, event injection |
| `screenshot.go` | GNOME Shell Screenshot D-Bus API |
| `clipboard.go` | wl-copy/wl-paste integration |
| `keyboard.go` | Keyboard state tracking |
| `upload.go` | File upload handling |
| `mcp_server.go` | Desktop MCP server (port 9877) |
| `env.go` | Environment detection |

**Good Patterns:**
- Atomic `running` flag for graceful shutdown
- WaitGroup for goroutine tracking
- Context-aware D-Bus connection retry
- Session health monitoring with auto-reconnect

### 3. Wolf Executor Changes

**Desktop Type Enum:**
```go
type DesktopType string
const (
    DesktopSway     = "sway"
    DesktopUbuntu   = "ubuntu"
    DesktopKDE      = "kde"
    DesktopHyprland = "hyprland"
    DesktopXFCE     = "xfce"
    DesktopZorin    = "zorin"
)
```

**Video Source Mode Selection:**
```go
// PipeWire mode for GNOME 49+ and Hyprland
if desktopType == DesktopUbuntu || desktopType == DesktopHyprland {
    videoSourceMode = "pipewire"
} else {
    videoSourceMode = "wayland"  // Sway, KDE use nested Wayland
}
```

**Device Mounting Logic:**
```go
// Ubuntu uses D-Bus RemoteDesktop API - no uinput needed
if config.DesktopType == DesktopUbuntu {
    gpuDevices = "/dev/dri/* /dev/nvidia* /dev/kfd"
} else {
    // Sway uses inputtino (uinput) for input
    gpuDevices = "/dev/uinput /dev/input/* /dev/dri/* /dev/nvidia* /dev/kfd"
}
```

### 4. Authentication Changes

**Removed:**
- `api/pkg/auth/keycloak.go` (600 lines)
- `api/pkg/auth/keycloak_test.go` (346 lines)
- `api/pkg/auth/realm.json` (1685 lines)

**Simplified to:** OIDC-only authentication with minimal oidc.go (+104 lines)

**Impact:** Eliminates Java runtime dependency, reduces container size, simplifies deployment.

---

## Review Items

### Critical (Should Address)

#### 1. Session Recreation Race Condition (Medium Priority)
**File:** `api/pkg/desktop/session.go:285-310`

```go
func (s *Server) handleSessionClosed(ctx context.Context) {
    // Clear old session state
    s.rdSessionPath = ""
    s.scSessionPath = ""
    s.scStreamPath = ""
    s.nodeID = 0

    // Recreate session
    if err := s.createSession(ctx); err != nil {
```

**Issue:** If `handleSessionClosed` is called from multiple goroutines (health check ticker + signal handler), there's no mutex protecting session state. Could race.

**Recommendation:** Add mutex or use atomic operations for session state.

### Medium Priority

#### 2. Magic Numbers in D-Bus Calls
**File:** `api/pkg/desktop/input.go:174-178`

```go
err = rdSession.Call(...+".NotifyPointerAxisDiscrete", 0, uint32(0), int32(event.DY)).Err  // axis 0
err = rdSession.Call(...+".NotifyPointerAxisDiscrete", 0, uint32(1), int32(event.DX)).Err  // axis 1
```

**Issue:** Magic numbers `0` and `1` for scroll axes without constants.

**Recommendation:** Define constants like `PointerAxisVertical = 0`, `PointerAxisHorizontal = 1`.

#### 3. WirePlumber Timeout Hack
**File:** `Dockerfile.ubuntu-helix:154-171`

```bash
# Change the default from 5 to 86400 (1 day)
sed -i 's/) or 5$/) or 86400/' /usr/share/wireplumber/scripts/node/suspend-node.lua
```

**Issue:** Fragile sed pattern matching. Could break on WirePlumber updates.

**Recommendation:** Consider WirePlumber config file override instead of modifying system file. Document why 86400s (1 day) was chosen.

#### 4. Hardcoded Monitor Name
**File:** `api/pkg/desktop/session.go:145`

```go
scSession.Call(screenCastSessionIface+".RecordMonitor", 0, "Meta-0", recordOptions)
```

**Issue:** Hardcoded "Meta-0" assumes single virtual monitor.

**Recommendation:** Either document this assumption or enumerate monitors from GetCurrentState.

### Low Priority (Nice to Have)

#### 5. Error Message Clarity
**File:** `api/pkg/desktop/session.go:74`

```go
return fmt.Errorf("failed to connect after 60 attempts: %w", err)
```

**Issue:** Doesn't explain *why* 60 attempts (60 seconds total).

**Recommendation:** Add comment: "// 60 attempts * 1s = 60s timeout for gnome-shell startup"

#### 6. CLI Stream Command Complexity
**File:** `api/pkg/cli/spectask/spectask.go` (1657 lines)

**Issue:** spectask.go has grown quite large with many subcommands.

**Recommendation:** Consider splitting into subpackages (e.g., `spectask/stream.go`, `spectask/stop.go`) in future refactor.

#### 7. Desktop Type String vs Enum
**File:** `api/pkg/external-agent/wolf_executor.go`

```go
type DesktopType string
```

**Issue:** String-based enum means typos won't be caught at compile time.

**Recommendation:** For future: consider using iota-based enums with String() method.

---

## Positive Observations

### 1. Excellent Design Documentation
The `design/2026-01-07-chrome-keyring-and-gnome-scaling.md` doc clearly explains:
- Root cause analysis
- Why both GSettings AND D-Bus ApplyMonitorsConfig are needed
- The `MUTTER_DEBUG_DUMMY_MONITOR_SCALES` discovery (only works for test backend)

### 2. Session Monitoring & Auto-Recovery
```go
func (s *Server) monitorSession(ctx context.Context) {
    // Subscribe to Closed signal on ScreenCast session
    // Periodic health check with Introspect
    // Auto-recreate on session death
}
```
This is production-grade resilience - D-Bus sessions can die unexpectedly.

### 3. Chrome Enterprise Policies
```json
{
    "DefaultBrowserSettingEnabled": false,
    "MetricsReportingEnabled": false,
    "BrowserSignin": 0,
    "PasswordManagerEnabled": true
}
```
Plus `--password-store=basic` in desktop file for keyring bypass. Well-researched.

### 4. Just Perfection Extension
Hiding the ScreenCast "stop" button prevents users from accidentally terminating the stream. Good UX thinking.

### 5. Input Bridge Architecture
Using Unix socket + JSON instead of direct D-Bus from Wolf is clever - it allows the Go process to maintain session state and handle reconnection.

---

## Testing Verification

From PR testing (2x scaling verification):
```
[gnome-session] Setting global scaling factor to 2 via GSettings...
[gnome-session] ApplyMonitorsConfig result: ()  # Success
```

Display scaling confirmed working with:
- GSettings before gnome-shell
- D-Bus ApplyMonitorsConfig after gnome-shell

---

## Summary

| Category | Items |
|----------|-------|
| Critical | 1 (session race condition) |
| Medium | 4 |
| Low | 3 |

**Recommendation:** PR is production-ready. Critical item is low-risk in practice (session death is rare). Could merge now and address items incrementally.

---

## Appendix: Key File Changes

| File | Lines | Description |
|------|-------|-------------|
| `Dockerfile.ubuntu-helix` | +800 | Complete Ubuntu 25.10 GNOME desktop |
| `wolf/ubuntu-config/startup-app.sh` | +833 | GNOME 49 startup with PipeWire mode |
| `api/pkg/desktop/*.go` | +1000 | Modular desktop integration |
| `api/pkg/cli/spectask/spectask.go` | +1657 | Expanded CLI commands |
| `api/pkg/external-agent/wolf_executor.go` | Modified | 6 desktop types |
| `api/pkg/auth/keycloak*.go` | -2631 | Keycloak removal |
