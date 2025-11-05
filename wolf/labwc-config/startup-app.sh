#!/bin/bash
set -e

echo "========================================"
echo "Starting Helix Personal Dev Environment with labwc (Wayland)"
echo "========================================"

# Environment setup
export HOME="/home/retro"
export USER="retro"
export XDG_RUNTIME_DIR="/tmp/sockets"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export XDG_CACHE_HOME="$HOME/.cache"

# Wayland environment variables
export XDG_SESSION_TYPE=wayland
export WAYLAND_DISPLAY=wayland-0
export GDK_BACKEND=wayland
export QT_QPA_PLATFORM=wayland
export MOZ_ENABLE_WAYLAND=1
export CLUTTER_BACKEND=wayland

# HiDPI scaling (200%)
export GDK_SCALE=2
export GDK_DPI_SCALE=1

# Create necessary directories
mkdir -p "$XDG_RUNTIME_DIR"
mkdir -p "$XDG_CONFIG_HOME"
mkdir -p "$XDG_DATA_HOME"
mkdir -p "$XDG_STATE_HOME"
mkdir -p "$XDG_CACHE_HOME"
mkdir -p "$HOME/.local/bin"
mkdir -p "$HOME/.zed"

# CRITICAL: Unset RUN_SWAY to prevent GOW launcher from starting Sway
# This ensures labwc compositor starts instead
unset RUN_SWAY

echo "✅ Environment variables configured for Wayland"

# Symlink Zed state directory if it exists on the host
if [ -d "/opt/zed-state" ]; then
    # Remove existing .zed directory if it exists
    rm -rf "$HOME/.zed"
    # Create symlink to persistent state
    ln -sf /opt/zed-state "$HOME/.zed"
    echo "✅ Zed state symlinks created"
else
    echo "⚠️  /opt/zed-state not found, Zed will use ephemeral state"
fi

# Create labwc config directory
mkdir -p "$XDG_CONFIG_HOME/labwc"

# Copy labwc configuration files if they exist
if [ -d "/cfg/labwc" ]; then
    cp -r /cfg/labwc/* "$XDG_CONFIG_HOME/labwc/"
    echo "✅ labwc configuration copied"
fi

# Create waybar config directory and copy config
mkdir -p "$XDG_CONFIG_HOME/waybar"
if [ -d "/cfg/waybar" ]; then
    cp -r /cfg/waybar/* "$XDG_CONFIG_HOME/waybar/"
    echo "✅ waybar configuration copied"
fi

# Start settings-sync-daemon in background
if [ -x "/usr/local/bin/settings-sync-daemon" ]; then
    /usr/local/bin/settings-sync-daemon &
    echo "✅ settings-sync-daemon started"
fi

# Start screenshot-server in background
if [ -x "/usr/local/bin/screenshot-server" ]; then
    /usr/local/bin/screenshot-server &
    echo "✅ screenshot-server started"
fi

# Start Zed launcher script in background (with delay)
if [ -x "/usr/local/bin/start-zed-helix.sh" ]; then
    /usr/local/bin/start-zed-helix.sh &
    echo "✅ Zed launcher started (will launch after initialization)"
fi

echo "========================================"
echo "Starting labwc compositor via GOW launcher..."
echo "========================================"

# Source GOW functions and start labwc via launcher
source /opt/gow/launch-comp.sh

# Start labwc directly (not via xorg.sh)
# labwc will create its own Wayland display
exec launcher labwc
