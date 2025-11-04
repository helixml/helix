#!/bin/bash
# GOW XFCE startup script for Helix Personal Dev Environment
#
# ===================================================================
# XFCE Desktop Environment for Helix
# ===================================================================
#
# This script starts the XFCE desktop environment with Helix tools.
# XFCE provides a traditional desktop experience with overlapping windows.
#
# ===================================================================

set -e

echo "Starting Helix Personal Dev Environment with XFCE..."

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    echo "Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# CRITICAL: Create Zed config symlinks BEFORE desktop starts
# Settings-sync-daemon needs the symlink to exist
WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state

# Ensure workspace directory exists with correct ownership
cd /home/retro
sudo chown retro:retro work
cd /home/retro/work

# Create persistent state directory structure
mkdir -p $ZED_STATE_DIR/config
mkdir -p $ZED_STATE_DIR/local-share
mkdir -p $ZED_STATE_DIR/cache

# Create symlinks BEFORE desktop starts (so settings-sync-daemon can write immediately)
rm -rf ~/.config/zed
mkdir -p ~/.config
ln -sf $ZED_STATE_DIR/config ~/.config/zed

rm -rf ~/.local/share/zed
mkdir -p ~/.local/share
ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed

rm -rf ~/.cache/zed
mkdir -p ~/.cache
ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

echo "✅ Zed state symlinks created (settings-sync-daemon can write immediately)"

# Configure GTK dark theme for all applications
mkdir -p $HOME/.config/gtk-3.0
echo "[Settings]" > $HOME/.config/gtk-3.0/settings.ini
echo "gtk-application-prefer-dark-theme=1" >> $HOME/.config/gtk-3.0/settings.ini

# Configure XFCE with Helix branding
mkdir -p $HOME/.config/xfce4/xfconf/xfce-perchannel-xml

# Set Helix wallpaper
mkdir -p $HOME/.config/xfce4
cat > $HOME/.config/xfce4/xfconf/xfce-perchannel-xml/xfce4-desktop.xml << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<channel name="xfce4-desktop" version="1.0">
  <property name="backdrop" type="empty">
    <property name="screen0" type="empty">
      <property name="monitordp-1" type="empty">
        <property name="workspace0" type="empty">
          <property name="color-style" type="int" value="0"/>
          <property name="image-style" type="int" value="5"/>
          <property name="last-image" type="string" value="/usr/share/backgrounds/helix-logo.png"/>
        </property>
      </property>
    </property>
  </property>
</channel>
EOF

# Start screenshot-server and settings-sync-daemon in background
# Wait for X server to be ready first
echo "Waiting for X/Wayland display server to initialize..."
WAIT_COUNT=0
while [ $WAIT_COUNT -lt 30 ]; do
    if [ -n "$DISPLAY" ] || [ -n "$WAYLAND_DISPLAY" ]; then
        echo "Display server ready: DISPLAY=$DISPLAY WAYLAND_DISPLAY=$WAYLAND_DISPLAY"
        break
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

# Start screenshot server (uses grim for Wayland, can fallback to X11)
if [ -f /usr/local/bin/screenshot-server ]; then
    echo "Starting screenshot-server..."
    /usr/local/bin/screenshot-server > /tmp/screenshot-server.log 2>&1 &
fi

# Start settings-sync-daemon (syncs Zed config from Helix)
if [ -f /usr/local/bin/settings-sync-daemon ]; then
    echo "Starting settings-sync-daemon..."
    env HELIX_SESSION_ID=$HELIX_SESSION_ID HELIX_API_URL=$HELIX_API_URL HELIX_API_TOKEN=$HELIX_API_TOKEN \
        /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1 &
fi

# Source GOW's launch-comp.sh for the launcher function
echo "Starting XFCE and launching Zed via GOW launcher..."
source /opt/gow/launch-comp.sh

# Use GOW's default XFCE launcher, then start Zed
exec /usr/local/bin/start-zed-helix.sh
