#!/bin/bash
# GOW GNOME startup script for Helix Desktop (Ubuntu)
# Used for: Desktop sessions, Spec Task sessions, and Exploratory sessions
# This version uses vanilla Ubuntu GNOME with NO custom HiDPI scaling

# ============================================================================
# FEATURE FLAGS - Set to "true" to enable, "false" to disable
# ============================================================================
# For debugging, set all to false for minimal Ubuntu, then enable one at a time
ENABLE_SCREENSHOT_SERVER="true"     # Screenshot/clipboard server
ENABLE_DEVILSPIE2="false"           # Window rule daemon (disabled: doesn't work with GNOME/Wayland)
ENABLE_POSITION_WINDOWS="true"      # wmctrl window positioning (works with GNOME/Xwayland)
ENABLE_SETTINGS_SYNC="true"         # Zed settings sync daemon
ENABLE_ZED_AUTOSTART="true"         # Auto-launch Zed editor
ENABLE_TERMINAL_STARTUP="false"     # Terminal with startup script
ENABLE_REVDIAL="true"               # RevDial client for API communication

# ============================================================================
# CRITICAL DEBUG SECTION - MUST BE FIRST (before set -e)
# ============================================================================
DEBUG_LOG=/tmp/ubuntu-startup-debug.log

# Redirect all output to both stdout and debug log file
exec 1> >(tee -a "$DEBUG_LOG")
exec 2>&1

echo "=== UBUNTU GNOME STARTUP DEBUG START $(date) ==="
echo "User: $(whoami)"
echo "UID: $(id -u)"
echo "Home: $HOME"
echo "PWD: $PWD"

echo ""
echo "=== ENVIRONMENT VARIABLES ==="
echo "XDG_RUNTIME_DIR: ${XDG_RUNTIME_DIR:-NOT SET}"
echo "HELIX_SESSION_ID: ${HELIX_SESSION_ID:-NOT SET}"
echo "HELIX_API_URL: ${HELIX_API_URL:-NOT SET}"
echo "GAMESCOPE_WIDTH: ${GAMESCOPE_WIDTH:-NOT SET (will default to 1920)}"
echo "GAMESCOPE_HEIGHT: ${GAMESCOPE_HEIGHT:-NOT SET (will default to 1080)}"
echo "GAMESCOPE_REFRESH: ${GAMESCOPE_REFRESH:-NOT SET (will default to 60)}"

echo ""
echo "=== CRITICAL FILE CHECKS ==="
echo "Zed binary exists: $([ -f /zed-build/zed ] && echo YES || echo NO)"
echo "Workspace mount exists: $([ -d /home/retro/work ] && echo YES || echo NO)"
echo "GOW xorg script exists: $([ -f /opt/gow/xorg.sh ] && echo YES || echo NO)"
echo "dconf settings exist: $([ -f /opt/gow/dconf-settings.ini ] && echo YES || echo NO)"

# Trap EXIT to show exit code
trap 'EXIT_CODE=$?; echo ""; echo "=== SCRIPT EXITING WITH CODE $EXIT_CODE at $(date) ===" ' EXIT

echo ""
echo "=== DEBUG SETUP COMPLETE - NOW ENABLING STRICT ERROR CHECKING ==="

# NOW enable strict error checking (after debug setup is complete)
set -e

echo ""
echo "=== UBUNTU GNOME STARTUP BEGINS ==="
echo "Starting Helix Ubuntu GNOME environment (vanilla, no custom scaling)..."

# ============================================================================
# CRITICAL: Fix home directory ownership FIRST
# ============================================================================
# Wolf mounts /wolf-state/agent-xxx:/home/retro which may be owned by ubuntu:ubuntu
# We need write permission to /home/retro before creating any symlinks or files
echo "Fixing /home/retro ownership..."
sudo chown retro:retro /home/retro
echo "Home directory ownership fixed"

# ============================================================================
# Workspace Directory Setup (Hydra Compatibility)
# ============================================================================
# CRITICAL: Create workspace symlink for Hydra bind-mount compatibility
# When Hydra is enabled, Docker CLI resolves symlinks before sending to daemon.
# By mounting workspace at its actual path (e.g., /filestore/workspaces/spec-tasks/{id})
# and symlinking /home/retro/work -> that path, user bind-mounts work correctly.
# See: design/2025-12-01-hydra-bind-mount-symlink.md
if [ -n "$WORKSPACE_DIR" ] && [ -d "$WORKSPACE_DIR" ]; then
    echo "[Workspace] Setting up workspace symlink: /home/retro/work -> $WORKSPACE_DIR"

    # Remove existing symlink or directory if it exists
    if [ -L /home/retro/work ]; then
        rm -f /home/retro/work
    elif [ -d /home/retro/work ]; then
        # If it's a real directory (not a symlink), remove it only if empty
        rmdir /home/retro/work 2>/dev/null || true
    fi

    # Create symlink: /home/retro/work -> $WORKSPACE_DIR
    if [ ! -e /home/retro/work ]; then
        ln -sf "$WORKSPACE_DIR" /home/retro/work
        echo "Created workspace symlink: /home/retro/work -> $WORKSPACE_DIR"
    fi

    # Ensure correct ownership on the actual workspace directory
    sudo chown retro:retro "$WORKSPACE_DIR"
else
    echo "[Workspace] Warning: WORKSPACE_DIR not set or doesn't exist, using /home/retro/work directly"
    # Fallback: ensure /home/retro/work exists
    mkdir -p /home/retro/work
    sudo chown retro:retro /home/retro/work
fi

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    echo "Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# CRITICAL: Create Zed config symlinks BEFORE desktop starts
# This ensures settings can persist even without settings-sync-daemon
WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state

# Ensure workspace directory exists with correct ownership
cd /home/retro
sudo chown -R retro:retro /home/retro/work 2>/dev/null || true
cd /home/retro/work

# Create persistent state directory structure
mkdir -p $ZED_STATE_DIR/config
mkdir -p $ZED_STATE_DIR/local-share
mkdir -p $ZED_STATE_DIR/cache

# Fix ownership of common home subdirectories (GOW base image may have root-owned dirs)
sudo chown -R retro:retro ~/.config ~/.cache ~/.local 2>/dev/null || true

# Create symlinks BEFORE desktop starts
rm -rf ~/.config/zed
mkdir -p ~/.config
ln -sf $ZED_STATE_DIR/config ~/.config/zed

rm -rf ~/.local/share/zed
mkdir -p ~/.local/share
ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed

rm -rf ~/.cache/zed
mkdir -p ~/.cache
ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

echo "Zed state symlinks created"

# Configure fontconfig for grayscale antialiasing (affects all apps including Zed)
# RGB subpixel rendering looks bad when desktop is scaled via streaming
mkdir -p ~/.config/fontconfig
cat > ~/.config/fontconfig/fonts.conf << 'FONTCONFIG_EOF'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <!-- Use grayscale antialiasing instead of subpixel (RGB) -->
  <!-- Subpixel rendering breaks when desktop is scaled via streaming -->
  <match target="font">
    <edit name="rgba" mode="assign"><const>none</const></edit>
    <edit name="antialias" mode="assign"><bool>true</bool></edit>
    <edit name="hinting" mode="assign"><bool>true</bool></edit>
    <edit name="hintstyle" mode="assign"><const>hintslight</const></edit>
  </match>
</fontconfig>
FONTCONFIG_EOF
echo "Fontconfig set to grayscale antialiasing"

# ============================================================================
# RevDial Client for API Communication
# ============================================================================
# Start RevDial client for reverse proxy (screenshot server, clipboard, git HTTP)
# CRITICAL: Starts BEFORE GNOME so API can reach sandbox immediately
# Uses user's API token for authentication (session-scoped, user-owned)
if [ "$ENABLE_REVDIAL" = "true" ] && [ -n "$HELIX_API_BASE_URL" ] && [ -n "$HELIX_SESSION_ID" ] && [ -n "$USER_API_TOKEN" ]; then
    REVDIAL_SERVER="${HELIX_API_BASE_URL}/api/v1/revdial"
    RUNNER_ID="sandbox-${HELIX_SESSION_ID}"

    echo "[RevDial] Starting client for API <-> sandbox communication..."
    echo "[RevDial] Server: $REVDIAL_SERVER"
    echo "[RevDial] Runner ID: $RUNNER_ID"

    /usr/local/bin/revdial-client \
        -server "$REVDIAL_SERVER" \
        -runner-id "$RUNNER_ID" \
        -token "$USER_API_TOKEN" \
        -local "localhost:9876" \
        >> /tmp/revdial-client.log 2>&1 &

    REVDIAL_PID=$!
    echo "RevDial client started (PID: $REVDIAL_PID) - API can now reach this sandbox"
else
    echo "Warning: RevDial client not started (missing HELIX_API_BASE_URL, HELIX_SESSION_ID, or USER_API_TOKEN)"
    echo "    HELIX_API_BASE_URL: ${HELIX_API_BASE_URL:-NOT SET}"
    echo "    HELIX_SESSION_ID: ${HELIX_SESSION_ID:-NOT SET}"
    echo "    USER_API_TOKEN: ${USER_API_TOKEN:+SET (hidden)}"
fi

# ============================================================================
# Disable GNOME Screensaver Proxy
# ============================================================================
# Prevent gsd-screensaver-proxy from showing "screen lock disabled" notification
# This daemon detects absence of GDM and shows persistent notification
# We don't need screen locking in containers, so disable it entirely
echo "Disabling GNOME screensaver proxy..."
mkdir -p ~/.config/autostart
cat > ~/.config/autostart/org.gnome.SettingsDaemon.ScreensaverProxy.desktop <<'SCREENSAVER_EOF'
[Desktop Entry]
Type=Application
Name=GNOME FreeDesktop screensaver
Exec=/bin/true
OnlyShowIn=GNOME;
NoDisplay=true
Hidden=true
SCREENSAVER_EOF

echo "Screensaver proxy disabled"

# ============================================================================
# Disable GNOME Initial Setup Wizard
# ============================================================================
# Mark initial setup as complete to prevent the "Connect Your Online Accounts" wizard
# The file format is: version-number (e.g., "42" for GNOME 42)
echo "Marking GNOME initial setup as complete..."
mkdir -p ~/.config
echo "42" > ~/.config/gnome-initial-setup-done
echo "GNOME initial setup marked as complete"

# ============================================================================
# Disable Ubuntu Update Notifier
# ============================================================================
# Prevent the "Ubuntu 24.04 LTS Upgrade Available" dialog from appearing
# The update-notifier daemon runs 60s after login and prompts for upgrades
echo "Disabling Ubuntu update notifier..."
cat > ~/.config/autostart/update-notifier.desktop <<'UPDATE_EOF'
[Desktop Entry]
Type=Application
Name=Update Notifier
Exec=/bin/true
NoDisplay=true
Hidden=true
UPDATE_EOF
echo "Update notifier disabled"

# ============================================================================
# Disable IBus Input Method Framework
# ============================================================================
# IBus can interfere with keyboard input in remote streaming environments
# This fixes Shift key and other modifier keys not working properly
echo "Disabling IBus input method framework..."
export GTK_IM_MODULE=gtk-im-context-simple
export QT_IM_MODULE=gtk-im-context-simple
export XMODIFIERS=@im=none
echo "IBus disabled (using simple input context)"

# ============================================================================
# Dynamic Keyboard Layout Configuration (Browser Locale Detection)
# ============================================================================
# If XKB_DEFAULT_LAYOUT is set (from browser locale detection in frontend),
# configure it as the primary layout while keeping other layouts available.
# See: design/2025-12-17-keyboard-layout-option.md
if [ -n "$XKB_DEFAULT_LAYOUT" ]; then
    echo "Setting default keyboard layout from browser: $XKB_DEFAULT_LAYOUT"

    # Build dconf-compatible sources array with detected layout first
    # Format: [('xkb', 'layout1'), ('xkb', 'layout2'), ...]
    LAYOUT_SOURCES="[('xkb', '$XKB_DEFAULT_LAYOUT')"

    # Add other common layouts (skip if already the default)
    for layout in us gb fr de es it pt ru jp cn; do
        if [ "$layout" != "$XKB_DEFAULT_LAYOUT" ]; then
            LAYOUT_SOURCES="$LAYOUT_SOURCES, ('xkb', '$layout')"
        fi
    done
    LAYOUT_SOURCES="$LAYOUT_SOURCES]"

    # Create override dconf file that will be loaded after static settings
    cat > /tmp/keyboard-override.ini << KEYBOARD_EOF
[org/gnome/desktop/input-sources]
sources=$LAYOUT_SOURCES
KEYBOARD_EOF

    echo "Keyboard layout sources: $LAYOUT_SOURCES"
    echo "Created /tmp/keyboard-override.ini for dconf"
else
    echo "No XKB_DEFAULT_LAYOUT set, using default layouts from dconf-settings.ini"
fi

# ============================================================================
# Window Management (devilspie2 + wmctrl)
# ============================================================================
# devilspie2 daemon watches for new windows and applies geometry rules
# This positions Firefox (launched by startup script via xdg-open) in the right third
# wmctrl is used by position-windows.sh to tile Terminal and Zed

if [ "$ENABLE_DEVILSPIE2" = "true" ]; then
    # Copy devilspie2 config from /etc/skel to user home
    mkdir -p ~/.config/devilspie2
    if [ -f /etc/skel/.config/devilspie2/helix-tiling.lua ]; then
        cp /etc/skel/.config/devilspie2/helix-tiling.lua ~/.config/devilspie2/
        echo "Devilspie2 config copied to ~/.config/devilspie2/"
    fi

    # NOTE: Firefox positioning is handled by helix-tiling.lua (along with Terminal and Zed)
    echo "devilspie2 config ready (helix-tiling.lua handles all window positioning)"
else
    echo "devilspie2 DISABLED by feature flag"
fi

if [ "$ENABLE_POSITION_WINDOWS" = "true" ]; then
    # Create window positioning script
    # Uses xdotool --sync for event-driven waiting (no arbitrary sleep delays)
    # Layout:
    #   Desktop 1 (Editor): Zed fullscreen
    #   Desktop 2 (Debug): Startup script terminal (left), Debug terminal (right)
    #   Chrome: Stays on current desktop when opened (overlays Zed on desktop 1)
    cat > /tmp/position-windows.sh << 'POSITION_EOF'
#!/bin/bash
# Position windows across virtual desktops
# Screen: 1920x1080 (no HiDPI scaling - vanilla Ubuntu)
#
# Desktop 1 (Editor): Zed - fullscreen
# Desktop 2 (Debug): Terminals side-by-side (left: startup, right: debug)
#
# Uses xdotool search --sync to wait for each window to appear (event-driven)
# Each wait runs in parallel so windows can appear in any order

# CRITICAL: Set DISPLAY for X11 commands (autostart doesn't inherit session env)
export DISPLAY=:9

echo "Starting window positioning (event-driven with xdotool --sync)..."

# Position Zed - fullscreen on desktop 1 (index 0)
# Zed class is "dev.zed.Zed-Dev" (dev) or "dev.zed.Zed" (release)
(
    ZED_WID=$(timeout 60 xdotool search --sync --onlyvisible --class "Zed" 2>/dev/null | head -1)
    if [ -n "$ZED_WID" ]; then
        # Move to desktop 1 (index 0)
        wmctrl -i -r "$ZED_WID" -t 0
        # Maximize (fullscreen)
        wmctrl -i -r "$ZED_WID" -b add,maximized_vert,maximized_horz
        echo "Positioned Zed: $ZED_WID -> desktop 1 (fullscreen)"
    else
        echo "WARNING: Zed window not found (timeout)"
    fi
) &

# Position first terminal (startup script) - left half of desktop 2 (index 1)
# xdotool --sync blocks until window appears, with 60s timeout
(
    # Wait for at least one terminal to appear
    TERMINAL_WID=$(timeout 60 xdotool search --sync --onlyvisible --class "gnome-terminal" 2>/dev/null | head -1)
    if [ -n "$TERMINAL_WID" ]; then
        # Move to desktop 2 (index 1)
        wmctrl -i -r "$TERMINAL_WID" -t 1
        # Position on left half: x=0, y=30 (account for top bar), width=960, height=1050
        wmctrl -i -r "$TERMINAL_WID" -e 0,0,30,960,1050
        echo "Positioned startup terminal: $TERMINAL_WID -> desktop 2 left"

        # Now look for a second terminal window for debug
        sleep 2  # Give time for debug terminal to spawn
        ALL_TERMINALS=$(xdotool search --onlyvisible --class "gnome-terminal" 2>/dev/null)
        for WID in $ALL_TERMINALS; do
            if [ "$WID" != "$TERMINAL_WID" ]; then
                # This is the second terminal - put it on the right
                wmctrl -i -r "$WID" -t 1
                wmctrl -i -r "$WID" -e 0,960,30,960,1050
                echo "Positioned debug terminal: $WID -> desktop 2 right"
                break
            fi
        done
    else
        echo "WARNING: Terminal window not found (timeout)"
    fi
) &

# Chrome/browser will open on desktop 1 overlaying Zed (user can rearrange)
# No positioning needed - just let it open normally

# Wait for all positioning jobs to complete
wait

echo "Window positioning complete at $(date)"
POSITION_EOF
    chmod +x /tmp/position-windows.sh
    echo "Window positioning script created (event-driven with xdotool --sync)"
else
    echo "position-windows.sh DISABLED by feature flag"
fi

# ============================================================================
# GNOME Autostart Entries Configuration
# ============================================================================
# Create GNOME autostart directory
mkdir -p ~/.config/autostart

echo "Creating GNOME autostart entries for Helix services..."

# NOTE: dconf settings are now loaded directly before GNOME starts (see below)
# instead of via autostart, to ensure wallpaper and theme are set early.

# Create autostart entry for screenshot server (starts immediately for fast screenshots)
if [ "$ENABLE_SCREENSHOT_SERVER" = "true" ]; then
    # CRITICAL: Pass DISPLAY=:9 for X11 clipboard support (Ubuntu GNOME runs on Xwayland)
    cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/bin/bash -c "DISPLAY=:9 /usr/local/bin/screenshot-server"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF
    echo "screenshot-server autostart entry created (with DISPLAY=:9 for X11 clipboard)"
else
    echo "screenshot-server autostart DISABLED by feature flag"
fi

# Autostart devilspie2 (window rule daemon - must start early, before Firefox)
# CRITICAL: Set DISPLAY=:9 for X11 window management (autostart doesn't inherit session env)
if [ "$ENABLE_DEVILSPIE2" = "true" ]; then
    cat > ~/.config/autostart/devilspie2.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Devilspie2 Window Rules
Exec=/bin/bash -c "DISPLAY=:9 devilspie2"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF
    echo "devilspie2 autostart entry created (with DISPLAY=:9 for X11 window management)"
else
    echo "devilspie2 autostart DISABLED by feature flag"
fi

# Autostart window positioning (runs after Zed and Terminal have launched)
if [ "$ENABLE_POSITION_WINDOWS" = "true" ]; then
    cat > ~/.config/autostart/position-windows.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Position Windows
Exec=/tmp/position-windows.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF
    echo "Window positioning autostart entry created (no delay - xdotool --sync waits for windows)"
else
    echo "position-windows autostart DISABLED by feature flag"
fi

# Create autostart entry for settings-sync-daemon
if [ "$ENABLE_SETTINGS_SYNC" = "true" ]; then
    # Pass environment variables via script wrapper
    cat > /tmp/start-settings-sync-daemon.sh <<EOF
#!/bin/bash
exec env HELIX_SESSION_ID="$HELIX_SESSION_ID" HELIX_API_URL="$HELIX_API_URL" HELIX_API_TOKEN="$USER_API_TOKEN" /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1
EOF
    sudo mv /tmp/start-settings-sync-daemon.sh /usr/local/bin/start-settings-sync-daemon.sh
    sudo chmod +x /usr/local/bin/start-settings-sync-daemon.sh

    cat > ~/.config/autostart/settings-sync-daemon.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Settings Sync Daemon
Exec=/usr/local/bin/start-settings-sync-daemon.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
NoDisplay=true
EOF
    echo "settings-sync-daemon autostart entry created"
else
    echo "settings-sync-daemon autostart DISABLED by feature flag"
fi

# NOTE: Display scaling is configured via monitors.xml (see above), NOT via gsettings
# gsettings text-scaling-factor only affects GTK text, not the actual display scale

# Create autostart entry for Zed (starts after settings are ready)
# NOTE: Autostart entry is just for launching Zed at login.
# Icon/dock matching is handled by system desktop file at /usr/share/applications/dev.zed.Zed-Dev.desktop
if [ "$ENABLE_ZED_AUTOSTART" = "true" ]; then
    # IMPORTANT: Autostart entries should NOT have StartupWMClass or NoDisplay=false
    # Having StartupWMClass here conflicts with the system desktop file at
    # /usr/share/applications/dev.zed.Zed-Dev.desktop, preventing GNOME from
    # properly associating the Zed window with the correct application entry.
    # The system desktop file handles dock icon and app search matching.
    cat > ~/.config/autostart/zed-helix.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Zed Helix Startup
Exec=/usr/local/bin/start-zed-helix.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=2
NoDisplay=true
EOF
    echo "Zed autostart entry created (NoDisplay=true, no StartupWMClass - system .desktop handles icon)"
else
    echo "Zed autostart DISABLED by feature flag"
fi

# NOTE: Firefox is NOT auto-started here - the project's startup script
# (from .helix/startup.sh in the cloned repo) handles opening Firefox
# with the correct app URL via xdg-open. Adding Firefox autostart here
# would create duplicate windows.

# ============================================================================
# Set Chrome as Default Browser
# ============================================================================
# Configure Chrome as the default handler for HTTP/HTTPS URLs so xdg-open works
echo "Setting Chrome as default browser..."
xdg-mime default google-chrome.desktop x-scheme-handler/http
xdg-mime default google-chrome.desktop x-scheme-handler/https
xdg-mime default google-chrome.desktop text/html
echo "Chrome set as default browser for HTTP/HTTPS URLs"

# ============================================================================
# dconf Settings Loaded in desktop.sh
# ============================================================================
# NOTE: dconf load is now done in /opt/gow/desktop.sh AFTER D-Bus is started.
# Trying to load dconf here fails because D-Bus isn't available yet.
# The desktop.sh script runs after xorg.sh starts Xwayland and D-Bus.

# CRITICAL: Create monitors.xml to configure Mutter's display resolution and scale
# This is the proper way to tell Mutter what resolution and scale to use.
# Without this, Mutter picks the highest available resolution (5120x2880)
# which doesn't match Gamescope's expected resolution.

# Calculate display scale from HELIX_ZOOM_LEVEL (default: 100% = scale 1)
# GNOME/Mutter supports integer scales (1, 2) in monitors.xml
# For fractional scaling, experimental-features would be needed
ZOOM_LEVEL=${HELIX_ZOOM_LEVEL:-100}
GNOME_SCALE=$(echo "scale=0; $ZOOM_LEVEL / 100" | bc)
# Ensure minimum scale of 1
if [ "$GNOME_SCALE" -lt 1 ]; then
    GNOME_SCALE=1
fi
echo "GNOME display scale: $GNOME_SCALE (from HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)"

mkdir -p ~/.config
cat > ~/.config/monitors.xml <<EOF
<monitors version="2">
  <configuration>
    <logicalmonitor>
      <x>0</x>
      <y>0</y>
      <scale>$GNOME_SCALE</scale>
      <primary>yes</primary>
      <monitor>
        <monitorspec>
          <connector>XWAYLAND0</connector>
          <vendor>unknown</vendor>
          <product>unknown</product>
          <serial>unknown</serial>
        </monitorspec>
        <mode>
          <width>${GAMESCOPE_WIDTH:-1920}</width>
          <height>${GAMESCOPE_HEIGHT:-1080}</height>
          <rate>59.96</rate>
        </mode>
      </monitor>
    </logicalmonitor>
  </configuration>
</monitors>
EOF

echo "Created monitors.xml for ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080} at scale $GNOME_SCALE"

# Also create an autostart entry that runs xrandr AFTER Mutter is fully initialized
# The monitors.xml should work, but this is a backup that runs with longer delay
cat > ~/.config/autostart/helix-resolution.desktop <<EOF
[Desktop Entry]
Type=Application
Name=Fix Display Resolution
Exec=/bin/bash -c "sleep 5 && xrandr --output XWAYLAND0 --mode ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080} && echo 'Resolution set to ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}' >> /tmp/ubuntu-startup-debug.log"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
NoDisplay=true
EOF

echo "Resolution fix autostart entry created (${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080})"

# Backup: Also set wallpaper via gsettings autostart entry
# This runs after GNOME starts AND after resolution is fixed
cat > ~/.config/autostart/helix-background.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Set Ubuntu Background
Exec=/bin/bash -c "sleep 3 && gsettings set org.gnome.desktop.background picture-uri 'file:///usr/share/backgrounds/warty-final-ubuntu.png' && gsettings set org.gnome.desktop.background picture-uri-dark 'file:///usr/share/backgrounds/warty-final-ubuntu.png'"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=2
NoDisplay=true
EOF

echo "Wallpaper backup autostart entry created"

# ============================================================================
# Cursor Environment Variables
# ============================================================================
# Try to fix double cursor issue (Gamescope software cursor + GNOME cursor)
# WLR_NO_HARDWARE_CURSORS=1 is used in Hyprland for similar VNC cursor issues
export WLR_NO_HARDWARE_CURSORS=1
echo "Cursor environment variables set (WLR_NO_HARDWARE_CURSORS=1)"

# ============================================================================
# GNOME Session Startup (Mutter SDK for GNOME 49)
# ============================================================================
# GNOME 49 removed --nested mode. We support two video source modes:
#
# 1. "wayland" mode (default, for backward compatibility):
#    - gnome-shell --devkit spawns mutter-devkit
#    - mutter-devkit renders to Wolf's Wayland display
#    - Wolf uses waylanddisplaysrc to capture
#
# 2. "pipewire" mode (new, for pure pipewiresrc capture):
#    - gnome-shell --devkit runs standalone
#    - Container creates ScreenCast session and reports PipeWire node ID
#    - Wolf uses pipewiresrc to capture directly from PipeWire
#
# See: design/2025-12-29-pipewire-wolf-bridge.md

# Determine video source mode from Wolf environment variable
VIDEO_SOURCE_MODE="${WOLF_VIDEO_SOURCE_MODE:-wayland}"
echo "Video source mode: $VIDEO_SOURCE_MODE"

# Save Wolf's Wayland display
export WOLF_WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"
echo "Wolf Wayland display: $WOLF_WAYLAND_DISPLAY"

# Create a script that runs inside dbus-run-session
# This ensures all D-Bus dependent services have access to the session bus
cat > /tmp/gnome-session.sh << GNOME_SESSION_EOF
#!/bin/bash
set -e

# Environment variables captured from outer script
export WOLF_WAYLAND_DISPLAY="$WOLF_WAYLAND_DISPLAY"
export HELIX_API_BASE_URL="$HELIX_API_BASE_URL"
export USER_API_TOKEN="$USER_API_TOKEN"
export WOLF_SESSION_ID="$WOLF_SESSION_ID"
export XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR"
VIDEO_SOURCE_MODE="$VIDEO_SOURCE_MODE"

echo "[gnome-session] Starting inside dbus-run-session..."
echo "[gnome-session] Video source mode: \$VIDEO_SOURCE_MODE"

# Set WAYLAND_DISPLAY based on video source mode
if [ "\$VIDEO_SOURCE_MODE" = "pipewire" ]; then
    # PipeWire mode: Don't inherit Wolf's Wayland display
    # mutter-devkit will have nowhere to output (which is fine - we use pipewiresrc)
    unset WAYLAND_DISPLAY
    echo "[gnome-session] PipeWire mode: WAYLAND_DISPLAY unset (using pipewiresrc)"
else
    # Wayland mode: mutter-devkit outputs to Wolf's display
    export WAYLAND_DISPLAY="\$WOLF_WAYLAND_DISPLAY"
    echo "[gnome-session] Wayland mode: WAYLAND_DISPLAY=\$WAYLAND_DISPLAY (mutter-devkit outputs here)"
fi

# Start PipeWire + WirePlumber (needed for both modes)
echo "[gnome-session] Starting PipeWire + WirePlumber..."
pipewire &
sleep 0.5
wireplumber &
sleep 0.5

# Display scaling for PipeWire mode (headless gnome-shell with virtual monitor)
# HELIX_ZOOM_LEVEL is percentage (100, 150, 200) set by wolf_executor
# Scaling works via org.gnome.desktop.interface scaling-factor GSettings key
# Reference: mutter src/backends/meta-settings.c meta_settings_get_global_scaling_factor()
ZOOM_LEVEL="${HELIX_ZOOM_LEVEL:-100}"
echo "[DEBUG] ZOOM_LEVEL before comparison: [$ZOOM_LEVEL]"
if [ "$ZOOM_LEVEL" -gt 100 ]; then
    # Calculate integer scale factor (200% → 2, 150% → 1)
    # Using expr instead of $(( )) due to shell compatibility issues
    HELIX_SCALE_FACTOR=$(expr $ZOOM_LEVEL / 100)
    echo "[DEBUG] HELIX_SCALE_FACTOR after expr: [\$HELIX_SCALE_FACTOR]"

    # Client app scaling (GTK and Qt applications)
    export GDK_SCALE=\$HELIX_SCALE_FACTOR
    export GDK_DPI_SCALE=1  # Prevent double-scaling
    export QT_SCALE_FACTOR=\$HELIX_SCALE_FACTOR
    echo "[gnome-session] Display scaling: \${HELIX_SCALE_FACTOR}x (from HELIX_ZOOM_LEVEL=$ZOOM_LEVEL)"

    # Set global scaling factor before gnome-shell starts
    # This tells Mutter to use this scale for ALL monitors (including virtual ones)
    if [ "\$HELIX_SCALE_FACTOR" -gt 1 ]; then
        echo "[gnome-session] Setting global scaling factor to \$HELIX_SCALE_FACTOR via GSettings..."
        gsettings set org.gnome.desktop.interface scaling-factor \$HELIX_SCALE_FACTOR
    fi

    # Enable fractional scaling feature (needed for UI to show scale options)
    gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
else
    HELIX_SCALE_FACTOR=""
    echo "[gnome-session] Display scaling: 1x (default)"
fi

# Background task to set display scale via D-Bus ApplyMonitorsConfig after gnome-shell starts
# gnome-randr doesn't work with Mutter (it's for wlroots). Use D-Bus API directly.
# See: https://gitlab.gnome.org/GNOME/mutter/-/blob/main/data/dbus-interfaces/org.gnome.Mutter.DisplayConfig.xml
# Apply display scale via D-Bus ApplyMonitorsConfig after gnome-shell starts
# This is needed because gsettings scaling-factor may not affect virtual monitor scale
if [ -n "\$HELIX_SCALE_FACTOR" ]; then
    (
        echo "[gnome-session] Waiting for GNOME Shell to start before setting display scale..."
        for i in \$(seq 1 60); do
            if pgrep -x gnome-shell > /dev/null 2>&1; then
                echo "[gnome-session] gnome-shell detected, waiting 3 seconds for display initialization..."
                sleep 3
                break
            fi
            sleep 1
        done

        echo "[gnome-session] Setting display scale to \${HELIX_SCALE_FACTOR}x via D-Bus ApplyMonitorsConfig..."

        # Get current state to retrieve serial and monitor info
        # GetCurrentState returns: (serial, monitors, logical_monitors, properties)
        # gdbus format: (uint32 123, [...], [...], {...})
        STATE=\$(gdbus call --session \\
            --dest org.gnome.Mutter.DisplayConfig \\
            --object-path /org/gnome/Mutter/DisplayConfig \\
            --method org.gnome.Mutter.DisplayConfig.GetCurrentState 2>&1)

        # Log first 500 chars of state (full output is very long)
        echo "[gnome-session] Current state (first 500 chars): \${STATE:0:500}"

        # Extract serial - handle both formats:
        # Format 1: (uint32 123, ...) - GNOME's gdbus output
        # Format 2: (123, ...) - plain format
        # Use grep to find first number after opening paren
        SERIAL=\$(echo "\$STATE" | grep -oP '\(uint32\s+\K\d+|\(\K\d+' | head -1)
        if [ -z "\$SERIAL" ]; then
            echo "[gnome-session] ERROR: Failed to get serial from GetCurrentState"
            echo "[gnome-session] Raw state for debugging: \$STATE"
        else
            echo "[gnome-session] Serial: \$SERIAL"

            # Apply the new scale using ApplyMonitorsConfig
            # Parameters: serial, method (1=temporary), logical_monitors, properties
            # logical_monitors: [(x, y, scale, transform, primary, [(connector, mode_id, props)])]
            # For Meta-0 virtual monitor at ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}
            #
            # NOTE: scale must be a double (d) in GVariant notation
            # Method 1 = temporary (doesn't persist), Method 2 = persistent
            # Extract the actual mode_id from GetCurrentState (format varies between GNOME versions)
            # Example: '3840x2160@60.000' or '1920x1080@60.000000'
            MODE_ID=\$(echo "\$STATE" | grep -oP "'${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@[^']+'" | head -1 | tr -d "'")
            if [ -z "\$MODE_ID" ]; then
                # Fallback to .000 format (GNOME 49 style)
                MODE_ID="${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@${GAMESCOPE_REFRESH:-60}.000"
                echo "[gnome-session] WARNING: Could not extract mode_id from state, using fallback: \$MODE_ID"
            else
                echo "[gnome-session] Extracted mode_id from GetCurrentState: \$MODE_ID"
            fi

            echo "[gnome-session] Calling ApplyMonitorsConfig: serial=\$SERIAL scale=\$HELIX_SCALE_FACTOR mode=\$MODE_ID"
            RESULT=\$(gdbus call --session \\
                --dest org.gnome.Mutter.DisplayConfig \\
                --object-path /org/gnome/Mutter/DisplayConfig \\
                --method org.gnome.Mutter.DisplayConfig.ApplyMonitorsConfig \\
                \$SERIAL 1 \\
                "[(int32 0, int32 0, double \$HELIX_SCALE_FACTOR, uint32 0, true, [('Meta-0', '\$MODE_ID', {})])]" \\
                "{}" 2>&1)

            echo "[gnome-session] ApplyMonitorsConfig result: \$RESULT"

            # Check if it failed and log helpful debug info
            if echo "\$RESULT" | grep -qi "error"; then
                echo "[gnome-session] ERROR: ApplyMonitorsConfig failed. Trying alternative approach with monitor query..."

                # Try to list available monitors for debugging
                echo "[gnome-session] Available monitors in GetCurrentState:"
                echo "\$STATE" | grep -oP "'Meta-[^']*'" | head -5 || echo "(none found)"
            fi
        fi

        echo "[gnome-session] Display scale configured"
    ) &
fi

# Start Helix services - they need wayland-0 (Mutter's client socket)
# Note: wayland-0 is created by gnome-shell, so we start these with a delay
(
    echo "[gnome-session] Waiting for Mutter to create wayland-0..."
    for i in \$(seq 1 30); do
        if [ -e "\$XDG_RUNTIME_DIR/wayland-0" ]; then
            echo "[gnome-session] wayland-0 is ready"
            break
        fi
        sleep 0.5
    done

    # Settings sync daemon
    if [ -n "\$HELIX_API_BASE_URL" ] && [ -n "\$USER_API_TOKEN" ]; then
        echo "[gnome-session] Starting settings-sync-daemon..."
        HELIX_API_TOKEN="\$USER_API_TOKEN" HELIX_API_URL="\$HELIX_API_BASE_URL" HELIX_SESSION_ID="\$HELIX_SESSION_ID" WAYLAND_DISPLAY=wayland-0 XDG_CURRENT_DESKTOP=GNOME /usr/local/bin/settings-sync-daemon >> /tmp/settings-sync-daemon.log 2>&1 &
    fi

    # Screenshot server (unified: handles RemoteDesktop+ScreenCast sessions, PipeWire node reporting, input bridge)
    # For PipeWire mode, this creates the D-Bus sessions and reports node ID to Wolf
    if [ -x /usr/local/bin/screenshot-server ]; then
        echo "[gnome-session] Starting screenshot server (includes D-Bus session + input bridge)..."
        WAYLAND_DISPLAY=wayland-0 XDG_CURRENT_DESKTOP=GNOME /usr/local/bin/screenshot-server >> /tmp/screenshot-server.log 2>&1 &
    fi

    # Launch Zed after GNOME Shell is ready - it needs wayland-0
    if [ -x /zed-build/zed ]; then
        echo "[gnome-session] Waiting for GNOME Shell to fully initialize..."
        sleep 2
        echo "[gnome-session] Launching Zed (WAYLAND_DISPLAY=wayland-0)..."
        WAYLAND_DISPLAY=wayland-0 /usr/local/bin/start-zed-helix.sh
    fi
) &

# Determine GNOME Shell mode based on video source
# - PipeWire mode: Use --headless (no display output, capture via pipewiresrc)
# - Wayland mode: Use --nested (outputs to Wolf's Wayland display for waylanddisplaysrc)
if [ "\$VIDEO_SOURCE_MODE" = "pipewire" ]; then
    echo "[gnome-session] Starting GNOME Shell in HEADLESS mode (PipeWire capture)..."
    echo "[gnome-session] Resolution: ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@${GAMESCOPE_REFRESH:-60}"
    # --headless: No display output (we capture via pipewiresrc ScreenCast)
    # --unsafe-mode: Allow screenshot-server to use org.gnome.Shell.Screenshot D-Bus API
    # --virtual-monitor WxH@R: Creates a virtual monitor at specified size and refresh rate
    gnome-shell --headless --unsafe-mode --virtual-monitor ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@${GAMESCOPE_REFRESH:-60}
else
    echo "[gnome-session] Starting GNOME Shell in NESTED mode (Wayland capture)..."
    echo "[gnome-session] Resolution: ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@${GAMESCOPE_REFRESH:-60}"
    # --nested: Outputs to parent Wayland display (Wolf's waylanddisplaysrc captures this)
    # --unsafe-mode: Allow screenshot-server to use org.gnome.Shell.Screenshot D-Bus API
    gnome-shell --nested --unsafe-mode --virtual-monitor ${GAMESCOPE_WIDTH:-1920}x${GAMESCOPE_HEIGHT:-1080}@${GAMESCOPE_REFRESH:-60}
fi
GNOME_SESSION_EOF

chmod +x /tmp/gnome-session.sh

echo "Starting GNOME session inside dbus-run-session..."
exec dbus-run-session -- /tmp/gnome-session.sh
