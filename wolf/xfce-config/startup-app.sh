#!/bin/bash
# GOW XFCE startup script for Helix Personal Dev Environment (Ubuntu)
# Simplified from Zorin - XFCE is lighter than GNOME and doesn't need systemd workarounds

# ============================================================================
# CRITICAL DEBUG SECTION - MUST BE FIRST (before set -e)
# ============================================================================
DEBUG_LOG=/tmp/ubuntu-startup-debug.log

# Redirect all output to both stdout and debug log file
exec 1> >(tee -a "$DEBUG_LOG")
exec 2>&1

echo "=== UBUNTU/XFCE STARTUP DEBUG START $(date) ==="
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

# Trap EXIT to show exit code
trap 'EXIT_CODE=$?; echo ""; echo "=== SCRIPT EXITING WITH CODE $EXIT_CODE at $(date) ===" ' EXIT

echo ""
echo "=== DEBUG SETUP COMPLETE - NOW ENABLING STRICT ERROR CHECKING ==="

# NOW enable strict error checking (after debug setup is complete)
set -e

echo ""
echo "=== UBUNTU/XFCE STARTUP BEGINS ==="
echo "Starting Helix Ubuntu/XFCE environment..."

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
# CRITICAL: Starts BEFORE desktop so API can reach sandbox immediately
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
# Devilspie2 Window Positioning Setup
# ============================================================================
# Devilspie2 auto-tiles windows (Firefox, Zed, Terminal) in 3 columns like Sway
# Copy config from /etc/skel to user home (skel only used for new user creation)
echo "Setting up devilspie2 window positioning..."
mkdir -p ~/.config/devilspie2
if [ -f /etc/skel/.config/devilspie2/helix-tiling.lua ]; then
    cp /etc/skel/.config/devilspie2/helix-tiling.lua ~/.config/devilspie2/
    echo "✅ Devilspie2 config copied to ~/.config/devilspie2/"
else
    echo "⚠️  Devilspie2 config not found at /etc/skel/.config/devilspie2/helix-tiling.lua"
fi

# ============================================================================
# XFCE HiDPI Configuration
# ============================================================================
# Configure XFCE for 2x HiDPI scaling to match remote streaming resolution
# This ensures consistent font/UI sizing across all components
echo "Configuring XFCE HiDPI settings..."

# Create script to apply XFCE settings after desktop starts (needs xfconf-query)
cat > /tmp/configure-xfce-hidpi.sh <<'XFCE_HIDPI_EOF'
#!/bin/bash
# Wait for XFCE to be fully started
sleep 3

# Set DPI for XFCE (96 * 2 = 192 for 2x scaling)
xfconf-query -c xsettings -p /Xft/DPI -s 192 2>/dev/null || true

# Set GTK window scaling factor
xfconf-query -c xsettings -p /Gdk/WindowScalingFactor -s 2 2>/dev/null || true

# Set cursor size for HiDPI
xfconf-query -c xsettings -p /Gtk/CursorThemeSize -s 48 2>/dev/null || true

# Configure panel for HiDPI - set panel height
xfconf-query -c xfce4-panel -p /panels/panel-1/size -s 48 2>/dev/null || true

# Set Helix wallpaper
xfconf-query -c xfce4-desktop -p /backdrop/screen0/monitor0/workspace0/last-image -s /usr/share/backgrounds/helix-hero.png 2>/dev/null || true

echo "✅ XFCE HiDPI settings applied"
XFCE_HIDPI_EOF

sudo mv /tmp/configure-xfce-hidpi.sh /usr/local/bin/configure-xfce-hidpi.sh
sudo chmod +x /usr/local/bin/configure-xfce-hidpi.sh
echo "✅ XFCE HiDPI configuration script created"

# ============================================================================
# XFCE Autostart Entries Configuration
# ============================================================================
# Create XFCE autostart directory
mkdir -p ~/.config/autostart

echo "Creating XFCE autostart entries for Helix services..."

# Create autostart entry for screenshot server (starts immediately for fast screenshots)
cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/usr/local/bin/screenshot-server
Hidden=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF

echo "screenshot-server autostart entry created"

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
Hidden=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF

echo "settings-sync-daemon autostart entry created"

# Create autostart entry for Zed (starts after settings are ready)
cat > ~/.config/autostart/zed-helix.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Zed Helix Editor
Exec=/usr/local/bin/start-zed-helix.sh
Hidden=false
NoDisplay=false
Icon=zed
EOF

echo "Zed autostart entry created"

# Create autostart entry for devilspie2 window positioning
# Devilspie2 watches for new windows and positions them in 3 columns
cat > ~/.config/autostart/devilspie2.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Devilspie2 Window Manager
Exec=devilspie2
Hidden=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF

echo "✅ devilspie2 autostart entry created"

# Create autostart entry for XFCE HiDPI configuration
cat > ~/.config/autostart/xfce-hidpi.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=XFCE HiDPI Configuration
Exec=/usr/local/bin/configure-xfce-hidpi.sh
Hidden=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF

echo "✅ XFCE HiDPI autostart entry created"

# ============================================================================
# XFCE Session Startup via GOW launch-comp.sh
# ============================================================================
# Launch XFCE via GOW's launcher() function from launch-comp.sh
# This handles: Xwayland startup -> D-Bus -> XFCE session
# The XFCE base image uses launch-comp.sh (not xorg.sh like Zorin)

echo "Launching XFCE via GOW launcher..."
source /opt/gow/launch-comp.sh
launcher
