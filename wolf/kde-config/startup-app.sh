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
    REVDIAL_SERVER="${HELIX_API_BASE_URL}/api/v1/revdial"
    RUNNER_ID="sandbox-${HELIX_SESSION_ID}"
    gow_log "[start] Starting RevDial client..."
    /usr/local/bin/revdial-client \
        -server "$REVDIAL_SERVER" \
        -runner-id "$RUNNER_ID" \
        -token "$USER_API_TOKEN" \
        -local "localhost:9876" \
        >> /tmp/revdial-client.log 2>&1 &
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

gow_log "[start] Starting pipewire"
pipewire &

# Start Helix services
if [ -n "$HELIX_API_BASE_URL" ] && [ -n "$USER_API_TOKEN" ]; then
  gow_log "[start] Starting settings-sync-daemon..."
  /usr/local/bin/settings-sync-daemon >> /tmp/settings-sync-daemon.log 2>&1 &
fi

if [ -x /usr/local/bin/screenshot-server ]; then
  gow_log "[start] Starting screenshot server..."
  /usr/local/bin/screenshot-server >> /tmp/screenshot-server.log 2>&1 &
fi

# Launch Zed in background after KDE starts
if [ -x /zed-build/zed ]; then
  (sleep 3 && /usr/local/bin/start-zed-helix.sh) &
fi

gow_log "[start] Starting KDE Plasma"
startplasma-wayland
EOF

chmod +x $XDG_RUNTIME_DIR/gow_start_kde

dbus-run-session -- $XDG_RUNTIME_DIR/gow_start_kde
