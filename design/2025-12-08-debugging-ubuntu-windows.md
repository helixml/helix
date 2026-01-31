# Debugging Ubuntu Desktop Windows - Incremental Build Guide

**Date:** 2025-12-08
**Goal:** Reset to minimal Ubuntu GNOME experience, then incrementally add features

## Current Issues

1. **inotify error** - "Too many open files (os error 24)" - CRITICAL, breaks Zed
2. **Wallpaper not showing** - Solid purple instead of Ubuntu default
3. **Window positioning complex** - Multiple systems (devilspie2, wmctrl, position-windows.sh)
4. **Slow terminal startup** - Much slower than Sway

## Strategy: Minimal First, Add Incrementally

### Phase 1: Minimal Ubuntu (Current Target)
- [ ] Default GNOME Shell with top panel and clock
- [ ] Default Ubuntu wallpaper visible
- [ ] No custom autostart apps
- [ ] Just a working desktop - user can manually open apps

### Phase 2: Add Basic Apps
- [ ] Add gnome-terminal to dock/autostart
- [ ] Verify terminal works without issues
- [ ] Test inotify limits

### Phase 3: Add Zed
- [ ] Add Zed to autostart
- [ ] Test without window positioning
- [ ] Verify no inotify errors

### Phase 4: Add Window Positioning
- [ ] Add position-windows.sh (wmctrl-based)
- [ ] Test tiling works
- [ ] Consider removing devilspie2 if wmctrl is sufficient

### Phase 5: Add Helix Services
- [ ] screenshot-server
- [ ] settings-sync-daemon
- [ ] RevDial client

---

## Fixing inotify Error

### Root Cause
The error "inotify_init returned Too many open files (os error 24)" means the system has hit the limit on inotify instances or watches.

### Check Current Limits (on host)
```bash
# Check current values
cat /proc/sys/fs/inotify/max_user_instances
cat /proc/sys/fs/inotify/max_user_watches

# Typical defaults:
# max_user_instances: 128
# max_user_watches: 65536
```

### Increase Limits (on host - requires root)
```bash
# Temporary (until reboot)
sudo sysctl fs.inotify.max_user_instances=512
sudo sysctl fs.inotify.max_user_watches=524288

# Permanent (add to /etc/sysctl.conf or /etc/sysctl.d/)
echo "fs.inotify.max_user_instances=512" | sudo tee -a /etc/sysctl.d/99-inotify.conf
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.d/99-inotify.conf
sudo sysctl -p /etc/sysctl.d/99-inotify.conf
```

### Why This Happens
- Each container shares the host's inotify limits
- Multiple Ubuntu containers running = shared limit exhausted
- Zed uses inotify heavily for file watching
- GNOME also uses inotify for various features

### Container-Level Mitigation
Can't increase limits inside container - must be done on host. But can reduce usage:
- Don't run multiple desktop containers simultaneously
- Reduce number of watched directories in Zed

---

## Testing Procedure

### Launch Minimal Container
1. Build with minimal startup: `./stack build-ubuntu`
2. Launch container via UI
3. Verify: Top panel with clock visible? Wallpaper showing?

### Debug Commands
```bash
# Get container ID
docker exec helix-sandbox-nvidia-1 docker ps | grep ubuntu

# Check processes running
docker exec helix-sandbox-nvidia-1 docker exec <ID> ps aux

# Check GNOME settings
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 gsettings list-recursively org.gnome.desktop.background"

# Check inotify usage (from host)
find /proc/*/fd -lname 'anon_inode:inotify' 2>/dev/null | wc -l

# Check per-user inotify instances
for p in /proc/[0-9]*/fd/*; do readlink $p 2>/dev/null; done | grep inotify | wc -l
```

### Manual Window Testing
```bash
# List windows with class
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 wmctrl -lx"

# Position a window manually
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 wmctrl -i -r <WINDOW_ID> -e 0,640,30,640,1050"
```

---

## Files to Modify

### Feature Flags in startup-app.sh
File: `wolf/ubuntu-config/startup-app.sh`

At the top of the file, there are feature flags to easily enable/disable components:

```bash
# FEATURE FLAGS - Set to "true" to enable, "false" to disable
ENABLE_SCREENSHOT_SERVER="false"    # Screenshot/clipboard server
ENABLE_DEVILSPIE2="false"           # Window rule daemon
ENABLE_POSITION_WINDOWS="false"     # wmctrl window positioning
ENABLE_SETTINGS_SYNC="false"        # Zed settings sync daemon
ENABLE_ZED_AUTOSTART="false"        # Auto-launch Zed editor
ENABLE_TERMINAL_STARTUP="false"     # Terminal with startup script
ENABLE_REVDIAL="false"              # RevDial client for API communication
```

### To Test Minimal Ubuntu:
1. Set all flags to `"false"` (already done)
2. Build: `./stack build-ubuntu`
3. Launch container
4. Verify: Just GNOME desktop with top panel, wallpaper, no custom apps

### To Enable Features Incrementally:
1. Change one flag to `"true"`
2. Rebuild: `./stack build-ubuntu`
3. Launch new container
4. Test that feature works
5. Repeat for next feature

---

## Wallpaper Investigation

### ROOT CAUSE FOUND: Resolution Mismatch

The wallpaper not showing is caused by **Mutter resetting the display resolution after xorg.sh sets it**.

**Timeline:**
1. xorg.sh starts Xwayland at default resolution (5120x2880)
2. xorg.sh calls `xrandr --mode 1920x1080` to set the target resolution
3. GNOME Shell/Mutter starts and **resets resolution to 5120x2880** (its preferred mode)
4. Wallpaper (3840x2160 PNG) fails to render correctly at the wrong resolution

**Evidence:**
```bash
# Gamescope expects 1920x1080
docker exec ... env | grep GAMESCOPE
# GAMESCOPE_WIDTH=1920
# GAMESCOPE_HEIGHT=1080

# But Xwayland runs at 5120x2880
docker exec -u retro <ID> bash -c "DISPLAY=:9 xrandr"
# current 5120 x 2880  <-- WRONG!
```

**Fix:** Set resolution AFTER Mutter starts, or configure Mutter to use 1920x1080 by default via monitors.xml:
```bash
# Manual fix in running container:
docker exec -u retro <ID> bash -c "DISPLAY=:9 xrandr --output XWAYLAND0 --mode 1920x1080"
docker exec -u retro <ID> bash -c "DISPLAY=:9 gsettings set org.gnome.desktop.background picture-uri 'file:///usr/share/backgrounds/warty-final-ubuntu.png'"
```

### Proper Fix: Create monitors.xml for Mutter

Create `~/.config/monitors.xml` BEFORE GNOME session starts to tell Mutter the correct resolution:

```xml
<monitors version="2">
  <configuration>
    <logicalmonitor>
      <x>0</x>
      <y>0</y>
      <scale>1</scale>
      <primary>yes</primary>
      <monitor>
        <monitorspec>
          <connector>XWAYLAND0</connector>
          <vendor>unknown</vendor>
          <product>unknown</product>
          <serial>unknown</serial>
        </monitorspec>
        <mode>
          <width>1920</width>
          <height>1080</height>
          <rate>59.96</rate>
        </mode>
      </monitor>
    </logicalmonitor>
  </configuration>
</monitors>
```

### Alternative Fix: Set resolution in autostart

Create an autostart entry that runs AFTER GNOME Shell is ready:
```bash
# ~/.config/autostart/fix-resolution.desktop
[Desktop Entry]
Type=Application
Name=Fix Resolution
Exec=bash -c "sleep 2 && xrandr --output XWAYLAND0 --mode 1920x1080"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
```

### Previous Investigation (for reference)

#### Current State
- gsettings shows correct picture-uri
- PNG file exists and is valid (3840x2160)
- GNOME renders solid purple (#2C001E) as fallback

#### Debug Steps
```bash
# Check if gsd-background is running (it's not a separate process in GNOME 40+)
docker exec helix-sandbox-nvidia-1 docker exec <ID> ps aux | grep gsd

# Try forcing wallpaper refresh
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 gsettings set org.gnome.desktop.background picture-uri ''"
sleep 1
docker exec helix-sandbox-nvidia-1 docker exec -u retro <ID> \
  bash -c "DISPLAY=:9 gsettings set org.gnome.desktop.background picture-uri 'file:///usr/share/backgrounds/warty-final-ubuntu.png'"
```

---

## Incremental Changes Log

| Date | Change | Result |
|------|--------|--------|
| 2025-12-08 | Added DISPLAY=:9 to position-windows.sh | wmctrl can connect |
| 2025-12-08 | Changed wmctrl -l to wmctrl -lx | Zed found by class |
| 2025-12-08 | Added feature flags to startup-app.sh | Minimal mode works |
| 2025-12-08 | Increased inotify max_user_instances to 1024 | Zed inotify error fixed |
| 2025-12-08 | Found wallpaper root cause: resolution mismatch | Mutter resets to 5120x2880 |
| 2025-12-08 | Added helix-resolution.desktop autostart | Fixes resolution after GNOME starts |

---

## Next Steps

1. ~~**IMMEDIATE**: Increase inotify limits on host~~ ✅ DONE (set to 1024)
2. ~~Create minimal startup-app.sh that only launches GNOME~~ ✅ DONE (feature flags)
3. ~~Find wallpaper root cause~~ ✅ DONE (resolution mismatch - Mutter resets to 5120x2880)
4. **Rebuild and test**: `git commit && ./stack build-ubuntu` and launch new container
5. Verify wallpaper shows after resolution fix
6. Incrementally enable feature flags one at a time
