#!/bin/bash
# GOW GNOME startup script for Helix Personal Dev Environment
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
sudo chown retro:retro /home/retro/work
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

# Create GNOME autostart directory
mkdir -p ~/.config/autostart

echo "Creating GNOME autostart entries for Helix services..."

# Create autostart entry for applying GNOME settings
# This runs AFTER D-Bus is available
cat > ~/.config/autostart/helix-gnome-settings.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Helix GNOME Settings
Exec=/usr/local/bin/apply-gnome-settings.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=2
NoDisplay=true
EOF

# Create the settings application script
cat > /tmp/apply-gnome-settings.sh <<'EOF'
#!/bin/bash
# Apply GNOME settings after D-Bus is available

echo "Applying Helix GNOME settings..."

# Load dconf settings from config file
if [ -f /cfg/gnome/dconf-settings.ini ]; then
    dconf load / < /cfg/gnome/dconf-settings.ini
fi

# Set Helix wallpaper
gsettings set org.gnome.desktop.background picture-uri "file:///usr/share/backgrounds/helix-logo.png"
gsettings set org.gnome.desktop.background picture-uri-dark "file:///usr/share/backgrounds/helix-logo.png"

# Configure dark theme
gsettings set org.gnome.desktop.interface gtk-theme "Adwaita-dark"
gsettings set org.gnome.desktop.interface color-scheme "prefer-dark"

# Disable Activities overview (single-app focus mode)
gsettings set org.gnome.shell disable-user-extensions true

echo "✅ GNOME settings applied successfully"
EOF

sudo mv /tmp/apply-gnome-settings.sh /usr/local/bin/apply-gnome-settings.sh
sudo chmod +x /usr/local/bin/apply-gnome-settings.sh

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

echo "✅ GNOME autostart entries created"

# Source GOW's launch-comp.sh and use Zorin's default startup flow
# This will start: Xwayland → D-Bus → GNOME desktop (via /opt/gow/xorg.sh)
echo "Starting GNOME via Zorin's default startup mechanism..."

# CRITICAL: Unset RUN_SWAY to prevent GOW launcher from starting Sway
# The base Zorin image includes both GNOME and Sway
# GOW's launcher checks "if [ -n $RUN_SWAY ]" and starts Sway if set
unset RUN_SWAY

source /opt/gow/launch-comp.sh
launcher /opt/gow/xorg.sh
