#!/bin/bash
# GOW GNOME startup script for Helix Personal Dev Environment (Ubuntu)
# This version uses vanilla Ubuntu GNOME with NO custom HiDPI scaling

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

# ============================================================================
# RevDial Client for API Communication
# ============================================================================
# Start RevDial client for reverse proxy (screenshot server, clipboard, git HTTP)
# CRITICAL: Starts BEFORE GNOME so API can reach sandbox immediately
# Uses user's API token for authentication (session-scoped, user-owned)
if [ -n "$HELIX_API_BASE_URL" ] && [ -n "$HELIX_SESSION_ID" ] && [ -n "$USER_API_TOKEN" ]; then
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

# Copy devilspie2 config from /etc/skel to user home
mkdir -p ~/.config/devilspie2
if [ -f /etc/skel/.config/devilspie2/helix-tiling.lua ]; then
    cp /etc/skel/.config/devilspie2/helix-tiling.lua ~/.config/devilspie2/
    echo "Devilspie2 config copied to ~/.config/devilspie2/"
fi

# Firefox window rule - position in right third of screen
cat > ~/.config/devilspie2/firefox.lua << 'DEVILSPIE_EOF'
-- Position Firefox windows in right third of screen
-- Right third: x=1280, width=640, full height
if (get_application_name() == "Firefox" or get_class_instance_name() == "firefox") then
    set_window_geometry(1280, 0, 640, 1080)
end
DEVILSPIE_EOF

echo "devilspie2 Firefox rule created"

# Create window positioning script (positions Terminal and Zed after they launch)
cat > /tmp/position-windows.sh << 'POSITION_EOF'
#!/bin/bash
# Tile windows in thirds after they appear
# Screen: 1920x1080 (no HiDPI scaling - vanilla Ubuntu)
# Left third (0-639): Terminal
# Middle third (640-1279): Zed
# Right third (1280-1919): Firefox (handled by devilspie2)

sleep 8  # Wait for Zed and Terminal to launch

# Position Terminal (gnome-terminal) - left third
TERMINAL_WID=$(wmctrl -l | grep -i "terminal\|startup" | head -1 | awk '{print $1}')
if [ -n "$TERMINAL_WID" ]; then
    wmctrl -i -r "$TERMINAL_WID" -e 0,0,0,640,1080
    echo "Positioned terminal: $TERMINAL_WID"
fi

# Position Zed - middle third
ZED_WID=$(wmctrl -l | grep -i "zed" | head -1 | awk '{print $1}')
if [ -n "$ZED_WID" ]; then
    wmctrl -i -r "$ZED_WID" -e 0,640,0,640,1080
    echo "Positioned Zed: $ZED_WID"
fi

echo "Window positioning complete"
POSITION_EOF

chmod +x /tmp/position-windows.sh
echo "Window positioning script created"

# ============================================================================
# GNOME Autostart Entries Configuration
# ============================================================================
# Create GNOME autostart directory
mkdir -p ~/.config/autostart

echo "Creating GNOME autostart entries for Helix services..."

# Create autostart entry for dconf settings loading (runs first, before other services)
cat > ~/.config/autostart/helix-dconf-settings.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Helix GNOME Settings
Exec=/bin/bash -c "dconf load / < /opt/gow/dconf-settings.ini"
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF

echo "dconf settings autostart entry created"

# Create autostart entry for screenshot server (starts immediately for fast screenshots)
cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/usr/local/bin/screenshot-server
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF

echo "screenshot-server autostart entry created"

# Autostart devilspie2 (window rule daemon - must start early, before Firefox)
cat > ~/.config/autostart/devilspie2.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Devilspie2 Window Rules
Exec=devilspie2
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=0
NoDisplay=true
EOF

echo "devilspie2 autostart entry created"

# Autostart window positioning (runs after Zed and Terminal have launched)
cat > ~/.config/autostart/position-windows.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Position Windows
Exec=/tmp/position-windows.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=12
NoDisplay=true
EOF

echo "Window positioning autostart entry created"

# Create autostart entry for settings-sync-daemon
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

# Create autostart entry for Zed (starts after settings are ready)
cat > ~/.config/autostart/zed-helix.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Zed Helix Editor
Exec=/usr/local/bin/start-zed-helix.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=5
NoDisplay=false
Icon=zed
EOF

echo "Zed autostart entry created"

# NOTE: Firefox is NOT auto-started here - the project's startup script
# (from .helix/startup.sh in the cloned repo) handles opening Firefox
# with the correct app URL via xdg-open. Adding Firefox autostart here
# would create duplicate windows.

# ============================================================================
# GNOME Session Startup (adapted from XFCE launch-comp.sh)
# ============================================================================
# The GOW XFCE base image uses launch-comp.sh to start XFCE.
# We adapt this approach for GNOME, using the same D-Bus and X11 infrastructure.

echo "Launching GNOME session..."

# Source GOW utilities
source /opt/gow/bash-lib/utils.sh

# Set up environment (adapted from launch-comp.sh launcher() function)
export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/

# Start D-Bus (required for GNOME)
sudo /opt/gow/startdbus

# Configure environment for GNOME on X11
export DESKTOP_SESSION=gnome
export XDG_CURRENT_DESKTOP=GNOME
export XDG_SESSION_TYPE="x11"
export _JAVA_AWT_WM_NONREPARENTING=1
export GDK_BACKEND=x11
export MOZ_ENABLE_WAYLAND=0
export QT_QPA_PLATFORM="xcb"
export QT_AUTO_SCREEN_SCALE_FACTOR=1
export QT_ENABLE_HIGHDPI_SCALING=1
export DISPLAY=:0
export $(dbus-launch)
export REAL_WAYLAND_DISPLAY=$WAYLAND_DISPLAY
unset WAYLAND_DISPLAY

# Launch Xwayland and GNOME session
# This mirrors the XFCE approach but uses gnome-session instead of startxfce4
echo "Starting Xwayland and gnome-session..."
exec dbus-run-session -- bash -E -c "WAYLAND_DISPLAY=\$REAL_WAYLAND_DISPLAY Xwayland :0 & sleep 2 && gnome-session"
