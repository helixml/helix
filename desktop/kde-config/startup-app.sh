#!/bin/bash -e
# GOW base-app startup script for Helix Desktop (KDE Plasma)
# Based on games-on-whales/gow dev-kde branch
# Used for: Desktop sessions, Spec Task sessions, and Exploratory sessions

source /opt/gow/bash-lib/utils.sh

gow_log "[start] Starting Helix Desktop (KDE Plasma)..."

# Run additional startup scripts (GOW pattern)
for file in /opt/gow/startup.d/* ; do
    if [ -f "$file" ] ; then
        gow_log "[start] Sourcing $file"
        source $file
    fi
done

# ====================================================================
# Helix-specific setup (before KDE starts)
# ====================================================================

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    gow_log "[start] Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# Workspace setup: Wolf executor mounts workspace at BOTH paths via bind mount
if [ -z "$WORKSPACE_DIR" ]; then
    gow_log "[start] FATAL: WORKSPACE_DIR environment variable not set"
    exit 1
fi
if [ ! -d "$WORKSPACE_DIR" ]; then
    gow_log "[start] FATAL: WORKSPACE_DIR does not exist: $WORKSPACE_DIR"
    exit 1
fi
if [ ! -d /home/retro/work ]; then
    gow_log "[start] FATAL: /home/retro/work bind mount not present"
    exit 1
fi
sudo chown retro:retro "$WORKSPACE_DIR"
sudo chown retro:retro /home/retro/work
gow_log "[start] Workspace mounted at both $WORKSPACE_DIR and /home/retro/work"

# Create Zed config symlinks BEFORE KDE starts
WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state
cd /home/retro/work

mkdir -p $ZED_STATE_DIR/config $ZED_STATE_DIR/local-share $ZED_STATE_DIR/cache

rm -rf ~/.config/zed && mkdir -p ~/.config && ln -sf $ZED_STATE_DIR/config ~/.config/zed
rm -rf ~/.local/share/zed && mkdir -p ~/.local/share && ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed
rm -rf ~/.cache/zed && mkdir -p ~/.cache && ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

gow_log "[start] Zed state symlinks created"

# Configure fontconfig for grayscale antialiasing (streaming optimization)
mkdir -p ~/.config/fontconfig
cat > ~/.config/fontconfig/fonts.conf << 'FONTCONFIG_EOF'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <match target="font">
    <edit name="rgba" mode="assign"><const>none</const></edit>
    <edit name="antialias" mode="assign"><bool>true</bool></edit>
    <edit name="hinting" mode="assign"><bool>true</bool></edit>
    <edit name="hintstyle" mode="assign"><const>hintslight</const></edit>
  </match>
</fontconfig>
FONTCONFIG_EOF

# Configure Qwen Code session persistence
export QWEN_DATA_DIR=$WORK_DIR/.qwen-state
mkdir -p $QWEN_DATA_DIR
rm -rf ~/.qwen && ln -sf $QWEN_DATA_DIR ~/.qwen
gow_log "[start] Qwen data directory set: QWEN_DATA_DIR=$QWEN_DATA_DIR"

# Start RevDial client for reverse proxy (API ↔ sandbox communication)
if [ -n "$HELIX_API_BASE_URL" ] && [ -n "$HELIX_SESSION_ID" ] && [ -n "$USER_API_TOKEN" ]; then
    # Note: RevDial is now integrated into desktop-bridge
    gow_log "[start] Starting RevDial client..."
        -server "$REVDIAL_SERVER" \
        -runner-id "$RUNNER_ID" \
        -token "$USER_API_TOKEN" \
        -local "localhost:9876" \
    gow_log "[start] RevDial client started (PID: $!)"
fi

# ====================================================================
# KDE Plasma startup (from GOW dev-kde branch)
# ====================================================================

export GAMESCOPE_WIDTH=${GAMESCOPE_WIDTH:-1920}
export GAMESCOPE_HEIGHT=${GAMESCOPE_HEIGHT:-1080}
export GAMESCOPE_REFRESH=${GAMESCOPE_REFRESH:-60}

# Shadow kwin_wayland_wrapper so that we can pass args to kwin wrapper
# whilst being launched by plasma-session
mkdir -p $XDG_RUNTIME_DIR/nested_kde
cat <<EOF > $XDG_RUNTIME_DIR/nested_kde/kwin_wayland_wrapper
#!/bin/sh
/usr/bin/kwin_wayland_wrapper --width $GAMESCOPE_WIDTH --height $GAMESCOPE_HEIGHT --wayland-display $WAYLAND_DISPLAY --xwayland --no-lockscreen \$@
EOF
chmod a+x $XDG_RUNTIME_DIR/nested_kde/kwin_wayland_wrapper
export PATH=$XDG_RUNTIME_DIR/nested_kde:$PATH

# For xwayland
mkdir -p /tmp/.X11-unix
chmod +t /tmp/.X11-unix
chmod 700 $XDG_RUNTIME_DIR

# Create KDE start script with Helix additions
cat <<EOF > $XDG_RUNTIME_DIR/gow_start_kde
#!/bin/bash

source /opt/gow/bash-lib/utils.sh

# Set KDE environment variables BEFORE starting any services
# These are normally set by startplasma-wayland, but services that start
# before KDE need them set explicitly for desktop environment detection
export XDG_CURRENT_DESKTOP=KDE
export KDE_SESSION_VERSION=6
export DESKTOP_SESSION=plasma

# CRITICAL: Set WAYLAND_DISPLAY for KDE session to use KWin's client socket
# Architecture explanation (see design/2025-12-28-kde-vs-sway-compositor-architecture.md):
# - Wolf creates wayland-1 as the parent compositor for video streaming
# - KWin connects to wayland-1 as its parent (via --wayland-display in kwin_wayland_wrapper)
# - KWin creates wayland-0 for its client applications (default nested socket name)
# - All KDE apps (plasmashell, dolphin, Zed) must connect to wayland-0 for window decorations
# - The kwin_wayland_wrapper already captured wayland-1 in the heredoc, so KWin still
#   connects to the correct parent even though we're changing WAYLAND_DISPLAY here
export WAYLAND_DISPLAY=wayland-0
gow_log "[start] Set WAYLAND_DISPLAY=wayland-0 (KWin client socket)"

# Display scaling support for nested Wayland
# KDE's display settings can't change Wolf's output, so we use environment-based scaling
# Set HELIX_DISPLAY_SCALE (e.g., "1.5" or "2") in Wolf executor to enable scaling
if [ -n "\$HELIX_DISPLAY_SCALE" ] && [ "\$HELIX_DISPLAY_SCALE" != "1" ]; then
    export QT_SCALE_FACTOR=\$HELIX_DISPLAY_SCALE
    export QT_ENABLE_HIGHDPI_SCALING=1
    export PLASMA_USE_QT_SCALING=1
    export GDK_SCALE=\$HELIX_DISPLAY_SCALE
    export GDK_DPI_SCALE=1  # Prevent double-scaling with GDK_SCALE
    gow_log "[start] Display scaling enabled: \${HELIX_DISPLAY_SCALE}x (via environment)"
else
    gow_log "[start] Display scaling: 1x (default)"
fi

gow_log "[start] Starting pipewire"
pipewire &

# Start Helix services
if [ -n "$HELIX_API_BASE_URL" ] && [ -n "$USER_API_TOKEN" ]; then
  gow_log "[start] Starting settings-sync-daemon..."
  XDG_CURRENT_DESKTOP=KDE KDE_SESSION_VERSION=6 /usr/local/bin/settings-sync-daemon >> /tmp/settings-sync-daemon.log 2>&1 &
fi

if [ -x /usr/local/bin/desktop-bridge ]; then
  gow_log "[start] Starting screenshot server with KDE environment..."
  XDG_CURRENT_DESKTOP=KDE KDE_SESSION_VERSION=6 /usr/local/bin/desktop-bridge >> /tmp/desktop-bridge.log 2>&1 &
fi

# Launch Zed in background after KDE panel (plasmashell) is ready
# We wait for plasmashell process to exist, which indicates KDE desktop is initialized
# This prevents Zed from starting before the panel exists, which causes it to fill the entire screen
if [ -x /zed-build/zed ]; then
  (
    gow_log "[start] Waiting for plasmashell to start before launching Zed..."
    # Wait up to 60 seconds for plasmashell to be running
    for i in \$(seq 1 60); do
      if pgrep -x plasmashell > /dev/null 2>&1; then
        gow_log "[start] plasmashell detected, waiting 2 more seconds for panel to initialize..."
        sleep 2
        break
      fi
      sleep 1
    done
    # WAYLAND_DISPLAY=wayland-0 is already set for the entire KDE session
    gow_log "[start] Launching Zed (WAYLAND_DISPLAY=\$WAYLAND_DISPLAY)..."
    /usr/local/bin/start-zed-helix.sh
  ) &
fi

gow_log "[start] Starting KDE Plasma"
startplasma-wayland
EOF

chmod +x $XDG_RUNTIME_DIR/gow_start_kde

dbus-run-session -- $XDG_RUNTIME_DIR/gow_start_kde
