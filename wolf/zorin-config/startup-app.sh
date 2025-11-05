#!/bin/bash
# MINIMAL GOW GNOME startup script for Helix - Stage 1.5 Baseline
# This version has NO custom autostart entries - just basic setup and GNOME launch

# ============================================================================
# CRITICAL DEBUG SECTION - MUST BE FIRST (before set -e)
# ============================================================================
DEBUG_LOG=/tmp/zorin-startup-debug.log

# Redirect all output to both stdout and debug log file
exec 1> >(tee -a "$DEBUG_LOG")
exec 2>&1

echo "=== MINIMAL ZORIN STARTUP DEBUG START $(date) ==="
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
echo "=== MINIMAL STARTUP BEGINS ==="
echo "Starting Helix Zorin environment with MINIMAL custom configuration..."

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

echo "✅ Zed state symlinks created"

# ============================================================================
# GNOME Autostart Entries Configuration
# ============================================================================
# Create GNOME autostart directory
mkdir -p ~/.config/autostart

echo "Creating GNOME autostart entries for Helix services..."

# Create script to configure GNOME display settings
# This fixes HiDPI scaling artifacts by disabling experimental fractional scaling
cat > /tmp/configure-gnome-display.sh <<'EOF'
#!/bin/bash
# Configure GNOME display settings for proper HiDPI scaling

echo "Configuring GNOME display settings..."

# Disable experimental fractional scaling to use true integer scaling
# This fixes artifacts in Settings panel and other GTK apps at 200% scaling
# Without this, GNOME treats even 200% as fractional and upscales from 100%
gsettings set org.gnome.mutter experimental-features "[]"

# Set 200% display scaling for X11 (avoids artifacts from dynamic scaling changes)
# This must be set at boot to avoid compositor issues with runtime scale changes
gsettings set org.gnome.settings-daemon.plugins.xsettings overrides "[{'Gdk/WindowScalingFactor', <2>}]"
gsettings set org.gnome.desktop.interface scaling-factor 2

echo "✅ GNOME display settings configured (200% integer scaling enabled)"
EOF

sudo mv /tmp/configure-gnome-display.sh /usr/local/bin/configure-gnome-display.sh
sudo chmod +x /usr/local/bin/configure-gnome-display.sh

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

echo "✅ dconf settings autostart entry created"

# Create autostart entry for GNOME display configuration (runs after dconf)
cat > ~/.config/autostart/helix-display-config.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Helix Display Configuration
Exec=/usr/local/bin/configure-gnome-display.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=1
NoDisplay=true
EOF

echo "✅ GNOME display configuration autostart entry created"

# Create autostart entry for screenshot server
cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/usr/local/bin/screenshot-server
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
NoDisplay=true
EOF

echo "✅ screenshot-server autostart entry created"

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

echo "✅ settings-sync-daemon autostart entry created"

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

echo "✅ Zed autostart entry created"

# ============================================================================
# GNOME Session Startup via GOW xorg.sh
# ============================================================================
# Launch GNOME via GOW's proven xorg.sh script
# This handles: Xwayland startup → D-Bus → GNOME session
# Note: dconf settings are loaded via autostart entry AFTER GNOME starts

echo "Launching GNOME via GOW xorg.sh..."
exec /opt/gow/xorg.sh
