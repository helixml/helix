#!/bin/bash
# GOW GNOME startup script for Helix Personal Dev Environment
#
# ===================================================================
# GNOME Desktop Environment for Helix (via Zorin OS)
# ===================================================================
#
# This script starts the GNOME desktop environment with Helix tools.
# GNOME provides a polished, full-featured desktop experience.
#
# ===================================================================

set -e

echo "Starting Helix Personal Dev Environment with GNOME/Zorin..."

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

# Configure GNOME with Helix branding via dconf
# Apply settings from our config file
if [ -f /cfg/gnome/dconf-settings.ini ]; then
    echo "Applying GNOME dconf settings..."
    dconf load / < /cfg/gnome/dconf-settings.ini
fi

# Set Helix wallpaper using gsettings
echo "Setting Helix wallpaper..."
gsettings set org.gnome.desktop.background picture-uri "file:///usr/share/backgrounds/helix-logo.png"
gsettings set org.gnome.desktop.background picture-uri-dark "file:///usr/share/backgrounds/helix-logo.png"

# Configure GNOME to disable Activities overview (single-app focus mode)
# This makes GNOME behave more like a single-application workspace
gsettings set org.gnome.shell disable-user-extensions true
gsettings set org.gnome.desktop.interface gtk-theme "Adwaita-dark"
gsettings set org.gnome.desktop.interface color-scheme "prefer-dark"

# Start screenshot-server and settings-sync-daemon in background
# Wait for Wayland display server to be ready first
echo "Waiting for Wayland display server to initialize..."
WAIT_COUNT=0
while [ $WAIT_COUNT -lt 30 ]; do
    if [ -n "$WAYLAND_DISPLAY" ]; then
        echo "Wayland display server ready: WAYLAND_DISPLAY=$WAYLAND_DISPLAY"
        break
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
done

# Start screenshot server (uses grim for Wayland)
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

# Wait for GNOME services to be ready before starting Zed
# This ensures settings-sync-daemon has time to write config.json
echo "Waiting for GNOME desktop services to initialize..."
sleep 5

# Source GOW's launch-comp.sh for the launcher function
echo "Starting GNOME and launching Zed via GOW launcher..."
source /opt/gow/launch-comp.sh

# Use GOW's default GNOME launcher, then start Zed
exec /usr/local/bin/start-zed-helix.sh
