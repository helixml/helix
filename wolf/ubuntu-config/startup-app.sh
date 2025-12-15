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
exec env HELIX_SESSION_ID="$HELIX_SESSION_ID" HELIX_API_URL="$HELIX_API_URL" HELIX_API_TOKEN="$HELIX_API_TOKEN" /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1
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
# CRITICAL: StartupWMClass MUST match Zed's WM_CLASS for GNOME to show the correct icon
# Zed dev builds report WM_CLASS as "dev.zed.Zed-Dev"
if [ "$ENABLE_ZED_AUTOSTART" = "true" ]; then
    cat > ~/.config/autostart/zed-helix.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Zed
Exec=/usr/local/bin/start-zed-helix.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=2
NoDisplay=false
Icon=dev.zed.Zed-Dev
StartupWMClass=dev.zed.Zed-Dev
StartupNotify=true
EOF
    echo "Zed autostart entry created (with StartupWMClass=dev.zed.Zed-Dev)"
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
# GNOME Session Startup (Zorin-compatible pattern)
# ============================================================================
# Use Zorin's proven GOW scripts which properly initialize Xwayland on :9,
# D-Bus, and GNOME session with hardware GPU rendering.
# See: design/2025-12-08-ubuntu-based-on-zorin.md

echo "Launching GNOME via GOW xorg.sh..."
exec /opt/gow/xorg.sh
