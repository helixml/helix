#!/bin/bash -e
# GOW startup script for Helix Desktop (Ubuntu GNOME Wayland)
# Extracted from Dockerfile.ubuntu-helix for consistency with Sway's startup-app.sh
source /opt/gow/bash-lib/utils.sh

gow_log "[start] Starting Helix Desktop (Ubuntu GNOME Wayland)..."

# GPU detection - export HELIX_RENDER_NODE and LIBVA_DRIVER_NAME for GStreamer
# The udev rule for Mutter was already created by the root init script,
# but we need the env vars for the GStreamer pipelines in desktop-bridge
if [ -f /usr/local/bin/detect-render-node.sh ]; then
    source /usr/local/bin/detect-render-node.sh
    gow_log "[start] GPU: HELIX_RENDER_NODE=${HELIX_RENDER_NODE:-not set}, LIBVA_DRIVER_NAME=${LIBVA_DRIVER_NAME:-not set}"
fi

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    gow_log "[start] Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# Workspace setup (same pattern as KDE/Sway)
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

# Create Zed config symlinks
WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state
cd /home/retro/work

mkdir -p $ZED_STATE_DIR/config $ZED_STATE_DIR/local-share $ZED_STATE_DIR/cache

rm -rf ~/.config/zed && mkdir -p ~/.config && ln -sf $ZED_STATE_DIR/config ~/.config/zed
rm -rf ~/.local/share/zed && mkdir -p ~/.local/share && ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed
rm -rf ~/.cache/zed && mkdir -p ~/.cache && ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

gow_log "[start] Zed state symlinks created"

# Configure fontconfig for grayscale antialiasing
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

# Note: RevDial client is now integrated into desktop-bridge

# Disable IBus Input Method Framework
# IBus can interfere with keyboard input in remote streaming environments
# This fixes Shift key and other modifier keys not working properly
export GTK_IM_MODULE=gtk-im-context-simple
export QT_IM_MODULE=gtk-im-context-simple
export XMODIFIERS=@im=none
gow_log "[start] IBus disabled (using simple input context)"

# GNOME Wayland session setup
export GAMESCOPE_WIDTH=${GAMESCOPE_WIDTH:-1920}
export GAMESCOPE_HEIGHT=${GAMESCOPE_HEIGHT:-1080}
export GAMESCOPE_REFRESH=${GAMESCOPE_REFRESH:-60}

# Display scaling support
# HELIX_DISPLAY_SCALE (e.g., "1.5" or "2") sets the scale factor for GTK/Qt apps
# GNOME Shell's --virtual-monitor sets the compositor resolution
if [ -n "$HELIX_DISPLAY_SCALE" ] && [ "$HELIX_DISPLAY_SCALE" != "1" ]; then
    export GDK_SCALE=$HELIX_DISPLAY_SCALE
    export GDK_DPI_SCALE=1  # Prevent double-scaling with GDK_SCALE
    export QT_SCALE_FACTOR=$HELIX_DISPLAY_SCALE
    gow_log "[start] Display scaling enabled: ${HELIX_DISPLAY_SCALE}x"
else
    gow_log "[start] Display scaling: 1x (default)"
fi

# Enable nested mode for Mutter (running inside Wolf's compositor)
export MUTTER_ALLOW_NESTED=1

# Save Wolf's Wayland display (wayland-1) before we change it for client apps
WOLF_WAYLAND_DISPLAY=$WAYLAND_DISPLAY

# Create GNOME start script
# Note: Using non-quoted heredoc so env vars are captured at script creation
cat <<GNOME_EOF > $XDG_RUNTIME_DIR/start_gnome
#!/bin/bash
source /opt/gow/bash-lib/utils.sh

# Set GNOME environment
export XDG_CURRENT_DESKTOP=GNOME
export XDG_SESSION_DESKTOP=gnome
export XDG_SESSION_TYPE=wayland
export DESKTOP_SESSION=gnome

# Ensure IBus is disabled (fixes keyboard input issues with modifier keys)
export GTK_IM_MODULE=gtk-im-context-simple
export QT_IM_MODULE=gtk-im-context-simple
export XMODIFIERS=@im=none

# HiDPI cursor support - larger cursors for better streaming quality
# This matches cursor-size=48 in dconf-settings.ini
export XCURSOR_SIZE=48

# Display scaling
# HELIX_ZOOM_LEVEL is a percentage (100, 150, 200) set by hydra_executor
# Convert to scale factor: 100→1, 150→1.5, 200→2
#
# Scaling works at TWO levels:
# 1. Compositor level: org.gnome.desktop.interface scaling-factor (GSettings)
#    - This makes GNOME Shell/Mutter use a global scale factor for all monitors
#    - Required for proper DPI scaling of the entire desktop
#    - Note: MUTTER_DEBUG_DUMMY_MONITOR_SCALES only works for dummy/test backend, not headless
# 2. Client app level: GDK_SCALE/QT_SCALE_FACTOR for GTK/Qt applications
#    - These tell individual apps to render at the correct scale
#
# Reference: mutter src/backends/meta-settings.c meta_settings_get_global_scaling_factor()
ZOOM_LEVEL="\${HELIX_ZOOM_LEVEL:-100}"
if [ "\$ZOOM_LEVEL" -gt 100 ]; then
    # Calculate integer scale factor from zoom percentage (e.g., 200% → 2)
    # Note: scaling-factor is an integer, so 150% becomes 1 (rounded down) or we use 2
    HELIX_SCALE_FACTOR=\$((\$ZOOM_LEVEL / 100))

    # Client app scaling (GTK and Qt applications)
    export GDK_SCALE=\$HELIX_SCALE_FACTOR
    export GDK_DPI_SCALE=1  # Prevent double-scaling with GDK_SCALE
    export QT_SCALE_FACTOR=\$HELIX_SCALE_FACTOR

    gow_log "[start] Display scaling: \${HELIX_SCALE_FACTOR}x (GDK_SCALE=\$HELIX_SCALE_FACTOR)"
else
    HELIX_SCALE_FACTOR=""
    gow_log "[start] Display scaling: 1x (default)"
fi

# GNOME 49 Mutter SDK (--devkit) Architecture:
# =============================================
# PipeWire mode: gnome-shell --headless + pipewiresrc capture
# - gnome-shell --headless creates a virtual display
# - Container creates ScreenCast session via D-Bus
# - Wolf/Moonlight captures via pipewiresrc

# Don't inherit any parent Wayland display
unset WAYLAND_DISPLAY
gow_log "[start] PipeWire mode: WAYLAND_DISPLAY unset (using pipewiresrc)"

gow_log "[start] Starting PipeWire + WirePlumber"
pipewire &
sleep 0.5
wireplumber &
sleep 0.5

# Load Ubuntu desktop theming (Yaru dark theme, fonts, Helix background)
if [ -f /opt/gow/dconf-settings.ini ]; then
    gow_log "[start] Loading Ubuntu desktop theming from dconf-settings.ini"
    dconf load / < /opt/gow/dconf-settings.ini || gow_log "[start] Warning: dconf load failed"
fi

# Enable extensions before gnome-shell starts so they are loaded:
# - Just Perfection: Hides the ScreenCast "stop" button that would crash Wolf if clicked
# - Helix Cursor: Sends cursor shape data to desktop-bridge via Unix socket
# - Ubuntu Dock: Keep the Ubuntu dock experience
gow_log "[start] Enabling GNOME Shell extensions..."
gsettings set org.gnome.shell enabled-extensions "['ubuntu-dock@ubuntu.com', 'just-perfection-desktop@just-perfection', 'helix-cursor@helix.ml']"
gsettings set org.gnome.shell.extensions.just-perfection screen-recording-indicator false
gsettings set org.gnome.shell.extensions.just-perfection screen-sharing-indicator false
gow_log "[start] Just Perfection extension configured"

# Set global scaling factor before gnome-shell starts
# This tells Mutter to use this scale for ALL monitors (including virtual ones)
# The scaling-factor gsetting is read by meta_settings_get_global_scaling_factor()
# which overrides the calculated per-monitor scale
if [ -n "\$HELIX_SCALE_FACTOR" ] && [ "\$HELIX_SCALE_FACTOR" -gt 1 ]; then
    gow_log "[start] Setting global scaling factor to \$HELIX_SCALE_FACTOR via GSettings..."
    gsettings set org.gnome.desktop.interface scaling-factor \$HELIX_SCALE_FACTOR
    # Also enable fractional scaling feature (needed for the UI to show scale options)
    gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer']"
fi

# Start Helix services via shared init scripts
# These scripts handle D-Bus/Wayland waiting and log prefixing consistently
# across both Sway and Ubuntu desktops
gow_log "[start] Starting Helix services via shared init scripts..."
/usr/local/bin/start-desktop-bridge.sh &
/usr/local/bin/start-settings-sync-daemon.sh &

# Verify display scale after GNOME Shell is ready (if scaling is enabled)
# Global scaling-factor from GSettings is applied at Mutter startup
if [ -n "\$HELIX_SCALE_FACTOR" ]; then
  (
    gow_log "[start] Waiting to verify display scale..."
    # Wait for wayland-0 socket (more reliable than pgrep)
    for i in \$(seq 1 30); do
      if [ -S "\${XDG_RUNTIME_DIR}/wayland-0" ]; then
        sleep 2
        break
      fi
      sleep 1
    done

    # Log the current display configuration for debugging
    STATE=\$(gdbus call --session \\
        --dest org.gnome.Mutter.DisplayConfig \\
        --object-path /org/gnome/Mutter/DisplayConfig \\
        --method org.gnome.Mutter.DisplayConfig.GetCurrentState 2>&1 || echo "D-Bus call failed")

    gow_log "[start] Display config (scaling-factor=\$HELIX_SCALE_FACTOR): \${STATE:0:400}"
  ) &
fi

# Launch code editor after GNOME Shell is ready - it needs wayland-0
# HELIX_AGENT_HOST_TYPE controls which editor to launch:
#   - zed (default): Launch Zed IDE with Qwen Code agent
#   - vscode: Launch VS Code with Roo Code extension
#   - cursor: Launch Cursor IDE with built-in AI agent
#   - headless: No editor (future: custom ACP client)
AGENT_HOST_TYPE="\${HELIX_AGENT_HOST_TYPE:-zed}"
gow_log "[start] Agent host type: \$AGENT_HOST_TYPE"

(
    gow_log "[start] Waiting for GNOME Wayland socket before launching editor..."
    # Wait for wayland-0 socket instead of pgrep - more reliable
    for i in \$(seq 1 60); do
      if [ -S "\${XDG_RUNTIME_DIR}/wayland-0" ]; then
        gow_log "[start] wayland-0 socket ready, launching editor..."
        break
      fi
      if [ \$((i % 10)) -eq 0 ]; then
        gow_log "[start] Still waiting for wayland-0... (\${i}s)"
      fi
      sleep 1
    done
    if [ ! -S "\${XDG_RUNTIME_DIR}/wayland-0" ]; then
      gow_log "[start] WARNING: wayland-0 not found after 60s, launching editor anyway..."
    fi

    export WAYLAND_DISPLAY=wayland-0

    case "\$AGENT_HOST_TYPE" in
      vscode)
        gow_log "[start] Launching VS Code with Roo Code extension..."
        # Use the VS Code startup script (handles workspace setup, Roo Code config, restart loop)
        /usr/local/bin/start-vscode-helix.sh
        ;;
      cursor)
        gow_log "[start] Launching Cursor IDE..."
        # Use the Cursor startup script (handles workspace setup, Cursor CLI config, restart loop)
        /usr/local/bin/start-cursor-helix.sh
        ;;
      headless)
        gow_log "[start] Headless mode - no editor launched (ACP client runs in desktop-bridge)"
        ;;
      zed|*)
        if [ -x /zed-build/zed ]; then
          gow_log "[start] Launching Zed IDE..."
          /usr/local/bin/start-zed-helix.sh
        else
          gow_log "[start] WARNING: Zed binary not found at /zed-build/zed"
        fi
        ;;
    esac
) &

gow_log "[start] Virtual monitor: ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}@${GAMESCOPE_REFRESH}"

# Enable experimental features for better frame pacing:
# - variable-refresh-rate: VRR support (may help with frame sync)
# - triple-buffering: Additional buffer for smoother frame delivery
# These are set unconditionally to help diagnose frame rate issues
gsettings set org.gnome.mutter experimental-features "['variable-refresh-rate', 'triple-buffering']"
gow_log "[start] Enabled mutter experimental features: variable-refresh-rate, triple-buffering"

# Start GNOME Shell in headless mode with virtual monitor
# --headless: No display output - captured via pipewiresrc from ScreenCast
# --unsafe-mode: Allow desktop-bridge to use org.gnome.Shell.Screenshot D-Bus API
# --virtual-monitor WxH@R: Creates virtual display at specified resolution
gow_log "[start] Starting GNOME Shell in HEADLESS mode (PipeWire capture)"
gnome-shell --headless --unsafe-mode --virtual-monitor ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}@${GAMESCOPE_REFRESH}
GNOME_EOF

chmod +x $XDG_RUNTIME_DIR/start_gnome

dbus-run-session -- $XDG_RUNTIME_DIR/start_gnome
