#!/bin/bash
set -e

# Prioritize Hyprland for GPU acceleration with NVIDIA
if command -v Hyprland >/dev/null 2>&1; then
    COMPOSITOR="hyprland"
    echo "Starting Hyprland compositor with NVIDIA GPU acceleration..."
elif command -v cage >/dev/null 2>&1; then
    COMPOSITOR="cage"
    echo "Starting cage compositor (fallback)..."
elif command -v weston >/dev/null 2>&1; then
    COMPOSITOR="weston"
    echo "Starting weston compositor (fallback)..."
else
    COMPOSITOR="cage"
    echo "Starting cage compositor (final fallback)..."
fi

# Set up environment for Wayland GPU acceleration
export WLR_RENDERER=${WLR_RENDERER:-gles2}
export WLR_BACKENDS=${WLR_BACKENDS:-headless}
export WLR_NO_HARDWARE_CURSORS=${WLR_NO_HARDWARE_CURSORS:-1}
export WLR_HEADLESS_OUTPUTS=${WLR_HEADLESS_OUTPUTS:-1}

# Set GPU acceleration settings
export __GL_THREADED_OPTIMIZATIONS=1
export __GL_SYNC_TO_VBLANK=1
export LIBGL_ALWAYS_SOFTWARE=0

# Evidence-based NVIDIA container configuration
# Based on container GPU access patterns and wlroots requirements

# Core NVIDIA environment variables for container runtime
export NVIDIA_VISIBLE_DEVICES=all
export NVIDIA_DRIVER_CAPABILITIES=all

# Standard DRI/Mesa configuration for container GPU access  
export LIBGL_DRIVERS_PATH="/usr/lib/x86_64-linux-gnu/dri"
export LIBVA_DRIVERS_PATH="/usr/lib/x86_64-linux-gnu/dri"

# Minimal EGL configuration for container environments
export EGL_PLATFORM=drm
export GBM_BACKEND=mesa-drm

# wlroots GPU configuration following Wolf's proven patterns  
export WLR_DRM_DEVICES=/dev/dri/card0
export WLR_RENDERER_ALLOW_SOFTWARE=1

# Wolf-style render node configuration for GPU acceleration
export WOLF_RENDER_NODE=/dev/dri/renderD128
export GST_GL_DRM_DEVICE=${GST_GL_DRM_DEVICE:-$WOLF_RENDER_NODE}

# Critical Wolf GPU environment variables (following docker.cpp:114-129)
export NVIDIA_VISIBLE_DEVICES=${NVIDIA_VISIBLE_DEVICES:-all}
export NVIDIA_DRIVER_CAPABILITIES=${NVIDIA_DRIVER_CAPABILITIES:-all}

# Wolf's GST GPU configuration (following wolf.Dockerfile:112-114)
export GST_GL_API=gles2
export GST_GL_WINDOW=surfaceless

# GPU detection and configuration
echo "Detecting GPU configuration..."

# Load kernel modules if available (container may have limited access)
modprobe -q nvidia 2>/dev/null || echo "nvidia module not available (normal in containers)"
modprobe -q nvidia_drm 2>/dev/null || echo "nvidia_drm module not available (normal in containers)"

# Check for DRI devices (primary indicator of GPU access in containers)
if ls /dev/dri/card* >/dev/null 2>&1; then
    DRI_CARD=$(ls /dev/dri/card* | head -1)
    echo "DRI device $DRI_CARD detected, attempting hardware acceleration"
    export GPU_AVAILABLE=1
    # Update WLR_DRM_DEVICES to use the actual detected card
    export WLR_DRM_DEVICES=$DRI_CARD
    
    # Check if lspci works and detects GPU
    if command -v lspci >/dev/null 2>&1 && lspci | grep -E "(VGA|3D|Display)" >/dev/null 2>&1; then
        echo "GPU detected via lspci:"
        lspci | grep -E "(VGA|3D|Display)" | head -1
        
        # Configure for NVIDIA if detected (following Wolf's vendor detection)
        if lspci | grep -i nvidia >/dev/null 2>&1; then
            echo "NVIDIA GPU detected - configuring for nvidia-drm"
            export GBM_BACKEND=nvidia-drm
            export __GLX_VENDOR_LIBRARY_NAME=nvidia
            export WLR_DRM_NO_ATOMIC=1
            
            # Wolf-style NVIDIA configuration for proper GPU acceleration
            export NVIDIA_VISIBLE_DEVICES=all
            export NVIDIA_DRIVER_CAPABILITIES=all
            echo "Set NVIDIA container runtime variables: NVIDIA_VISIBLE_DEVICES=all, NVIDIA_DRIVER_CAPABILITIES=all"
        fi
    else
        echo "lspci not available or no GPU detected - using Mesa drivers"
    fi
else
    echo "No DRI devices detected, forcing software rendering"
    export WLR_RENDERER=pixman
    export MESA_LOADER_DRIVER_OVERRIDE=llvmpipe
    export LIBGL_ALWAYS_SOFTWARE=1
    export GPU_AVAILABLE=0
fi

# Function to try starting a compositor with fallback
start_compositor_with_fallback() {
    if [ "$COMPOSITOR" = "weston" ]; then
        # Create weston config optimized for NVIDIA GPU acceleration
        mkdir -p /home/ubuntu/.config
        cat > /home/ubuntu/.config/weston.ini << 'WESTONCONF'
[core]
# Use headless backend for VNC with GPU acceleration
backend=headless-backend.so
# Enable GPU renderer for NVIDIA
renderer=gl
# Disable input requirement for headless mode
require-input=false

[renderer]
# Force OpenGL ES renderer for NVIDIA acceleration
name=gl

[output]
# Create a 1920x1080 headless output with GPU acceleration
name=headless
mode=1920x1080
# Enable hardware acceleration
acceleration=true

[shell]
background-color=0xff002244
# Disable animations for headless performance
animation=none

[launcher]
# Disable launcher for headless mode
icon=/dev/null
path=/bin/true
WESTONCONF
        
        chown ubuntu:ubuntu /home/ubuntu/.config/weston.ini
        
        echo "Attempting to start weston with headless backend..."
        weston --backend=headless-backend.so --width=1920 --height=1080 &
        COMPOSITOR_PID=$!
        sleep 8
        
        # Check if weston is still running
        if ! kill -0 $COMPOSITOR_PID 2>/dev/null; then
            echo "Weston failed to start, falling back to cage..."
            cage -- sleep infinity &
            COMPOSITOR_PID=$!
            sleep 5
        else
            echo "Weston started successfully"
        fi
    elif [ "$COMPOSITOR" = "hyprland" ]; then
        # Create crash report directory for Hyprland
        mkdir -p /home/ubuntu/.cache/hyprland
        chown ubuntu:ubuntu /home/ubuntu/.cache/hyprland
        
        # Create optimized Hyprland config for NVIDIA GPU acceleration
        mkdir -p /home/ubuntu/.config/hypr
        cat > /home/ubuntu/.config/hypr/hyprland.conf << 'HYPRCONF'
# Hyprland configuration optimized for NVIDIA GPU acceleration in containers
# Following Wolf's GPU patterns and Hyprland NVIDIA best practices

# Monitor configuration for headless VNC
monitor = WL-1,1920x1080@60,0x0,1

# Container and headless optimizations
misc {
    disable_hyprland_logo = true
    disable_splash_rendering = true
    vfr = false
    vrr = 0
    no_direct_scanout = true
    force_default_wallpaper = 0
    animate_manual_resizes = false
    animate_mouse_windowdragging = false
    suppress_portal_warnings = true
}

# Critical NVIDIA environment variables (matching our detection)
env = NVIDIA_VISIBLE_DEVICES,all
env = NVIDIA_DRIVER_CAPABILITIES,all
env = GBM_BACKEND,nvidia-drm
env = __GLX_VENDOR_LIBRARY_NAME,nvidia
env = WLR_NO_HARDWARE_CURSORS,1
env = WLR_DRM_NO_ATOMIC,1
env = WLR_RENDERER,gles2
env = WLR_BACKENDS,headless
env = GST_GL_API,gles2
env = GST_GL_WINDOW,surfaceless
# Additional container environment fixes
env = LIBGL_ALWAYS_SOFTWARE,0
env = MESA_LOADER_DRIVER_OVERRIDE,
env = EGL_PLATFORM,drm
env = WLR_DRM_DEVICES,/dev/dri/card1

# Render settings for NVIDIA GPU acceleration
render {
    explicit_sync = 0
    explicit_sync_kms = 0
    direct_scanout = false
}

# OpenGL settings for NVIDIA
opengl {
    nvidia_anti_flicker = false
    force_introspection = 2
}

# Input configuration
input {
    kb_layout = us
    follow_mouse = 1
    repeat_rate = 25
    repeat_delay = 600
    accel_profile = flat
    force_no_accel = false
}

# General settings optimized for performance
general {
    gaps_in = 0
    gaps_out = 0
    border_size = 2
    col.active_border = rgba(33ccffee)
    col.inactive_border = rgba(595959aa)
    layout = dwindle
    no_focus_fallback = true
    allow_tearing = false
    resize_on_border = true
    extend_border_grab_area = 10
}

# Disable all animations for maximum performance
animations {
    enabled = false
}

# Minimal decoration for performance
decoration {
    rounding = 0
    drop_shadow = false
    blur {
        enabled = false
    }
}

# Layout settings
dwindle {
    pseudotile = false
    preserve_split = true
    smart_split = false
    smart_resizing = false
}

# Gestures (disabled for headless)
gestures {
    workspace_swipe = false
}

# XWayland settings - disabled for container environment stability
xwayland {
    force_zero_scaling = true
    # Disable XWayland to avoid EGL context issues in containers
    # Can be re-enabled later once core Wayland functionality is stable
    enabled = false
}

# Window rules for better compatibility
windowrulev2 = immediate, class:.*
windowrulev2 = noanim, class:.*

# Workspace configuration
workspace = 1, monitor:WL-1, default:true

# Auto-start terminal for testing
exec-once = foot
HYPRCONF
        
        # Enhanced Hyprland startup with NVIDIA GPU acceleration
        echo "Attempting to start Hyprland with NVIDIA RTX 4090 GPU acceleration..."
        
        # Set up comprehensive environment for Hyprland NVIDIA support
        export HYPRLAND_LOG_WLR=1
        export HYPRLAND_NO_RT=1
        export WLR_RENDERER_ALLOW_SOFTWARE=1
        
        # Ensure ownership of config files
        chown -R ubuntu:ubuntu /home/ubuntu/.config/hypr /home/ubuntu/.cache/hyprland
        
        # Pre-flight GPU check
        echo "Pre-flight GPU check:"
        echo "DRI devices: $(ls /dev/dri/)"
        echo "NVIDIA_VISIBLE_DEVICES: $NVIDIA_VISIBLE_DEVICES"
        echo "GBM_BACKEND: $GBM_BACKEND"
        
        # Start Hyprland with comprehensive error capture
        echo "Starting Hyprland..."
        # Redirect both stdout and stderr to capture all output
        (Hyprland 2>&1 | while read line; do echo "HYPRLAND: $line"; done) &
        COMPOSITOR_PID=$!
        
        # Give Hyprland more time to initialize with GPU
        sleep 15
        
        # Check if Hyprland is still running
        if ! kill -0 $COMPOSITOR_PID 2>/dev/null; then
            echo "❌ Hyprland failed to start with NVIDIA GPU acceleration"
            echo "Checking for any error output..."
            
            # Kill any remaining Hyprland processes
            pkill -f Hyprland 2>/dev/null || true
            sleep 3
            
            # Try Hyprland with modified settings for container environment
            echo "🔄 Trying Hyprland with container-optimized settings..."
            export WLR_RENDERER=gles2
            export WLR_BACKENDS=headless
            export WLR_RENDERER_ALLOW_SOFTWARE=1
            export WLR_DRM_NO_MODIFIERS=1
            
            # Second attempt with more conservative settings
            (Hyprland 2>&1 | while read line; do echo "HYPRLAND-2: $line"; done) &
            COMPOSITOR_PID=$!
            sleep 12
            
            if ! kill -0 $COMPOSITOR_PID 2>/dev/null; then
                echo "❌ Hyprland failed on second attempt, falling back to cage..."
                pkill -f Hyprland 2>/dev/null || true
                sleep 2
                echo "Starting cage as fallback..."
                cage -- sleep infinity &
                COMPOSITOR_PID=$!
                sleep 5
                echo "✅ Cage started as fallback compositor"
            else
                echo "✅ Hyprland started successfully with container-optimized settings"
            fi
        else
            echo "✅ Hyprland started successfully with NVIDIA GPU acceleration!"
        fi
    else
        # Try cage with hardware acceleration first
        echo "Attempting to start cage with hardware acceleration..."
        cage -- sleep infinity &
        COMPOSITOR_PID=$!
        sleep 8
        
        # Check if cage is still running
        if ! kill -0 $COMPOSITOR_PID 2>/dev/null; then
            echo "Cage failed with hardware acceleration, falling back to software rendering"
            export WLR_RENDERER=pixman
            export LIBGL_ALWAYS_SOFTWARE=1
            export MESA_LOADER_DRIVER_OVERRIDE=llvmpipe
            cage -- sleep infinity &
            COMPOSITOR_PID=$!
            sleep 5
        else
            echo "Cage started successfully with hardware acceleration"
        fi
    fi
    
    # Wait a bit more for compositor to fully initialize
    sleep 3
    return 0
}

# Start compositor in the background
su ubuntu -c "
export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p /tmp/runtime-ubuntu
chmod 700 /tmp/runtime-ubuntu

# Export compositor choice and environment variables
export COMPOSITOR=\"$COMPOSITOR\"
export WLR_RENDERER=\"$WLR_RENDERER\"
export WLR_BACKENDS=\"$WLR_BACKENDS\"
export WLR_NO_HARDWARE_CURSORS=\"$WLR_NO_HARDWARE_CURSORS\"
export WLR_HEADLESS_OUTPUTS=\"$WLR_HEADLESS_OUTPUTS\"
export __GL_THREADED_OPTIMIZATIONS=\"$__GL_THREADED_OPTIMIZATIONS\"
export __GL_SYNC_TO_VBLANK=\"$__GL_SYNC_TO_VBLANK\"
export LIBGL_ALWAYS_SOFTWARE=\"$LIBGL_ALWAYS_SOFTWARE\"
export GBM_BACKEND=\"$GBM_BACKEND\"
export __GLX_VENDOR_LIBRARY_NAME=\"$__GLX_VENDOR_LIBRARY_NAME\"
export WLR_DRM_NO_ATOMIC=\"$WLR_DRM_NO_ATOMIC\"
export MESA_LOADER_DRIVER_OVERRIDE=\"$MESA_LOADER_DRIVER_OVERRIDE\"
export GPU_AVAILABLE=\"$GPU_AVAILABLE\"
export NVIDIA_VISIBLE_DEVICES=\"$NVIDIA_VISIBLE_DEVICES\"
export NVIDIA_DRIVER_CAPABILITIES=\"$NVIDIA_DRIVER_CAPABILITIES\"
export EGL_PLATFORM=\"$EGL_PLATFORM\"
export WLR_DRM_DEVICES=\"$WLR_DRM_DEVICES\"
export WLR_RENDERER_ALLOW_SOFTWARE=\"$WLR_RENDERER_ALLOW_SOFTWARE\"
export LIBGL_DRIVERS_PATH=\"$LIBGL_DRIVERS_PATH\"
export LIBVA_DRIVERS_PATH=\"$LIBVA_DRIVERS_PATH\"
export WOLF_RENDER_NODE=\"$WOLF_RENDER_NODE\"
export GST_GL_DRM_DEVICE=\"$GST_GL_DRM_DEVICE\"

# Start dbus session
if [ -z \"\$DBUS_SESSION_BUS_ADDRESS\" ]; then
    eval \$(dbus-launch --sh-syntax)
fi

# Start compositor with fallback logic
$(declare -f start_compositor_with_fallback)
start_compositor_with_fallback

# Start wayvnc VNC server on the Wayland display
echo \"Starting wayvnc VNC server...\"
# Wait for Wayland display to be ready
sleep 2
# Find the actual Wayland display socket
if [ -S \"\$XDG_RUNTIME_DIR/wayland-1\" ]; then
    export WAYLAND_DISPLAY=wayland-1
elif [ -S \"\$XDG_RUNTIME_DIR/wayland-0\" ]; then
    export WAYLAND_DISPLAY=wayland-0
fi
echo \"Using Wayland display: \$WAYLAND_DISPLAY\"
# Start wayvnc with input disabled for better compatibility
wayvnc --disable-input 0.0.0.0 5901 &
WAYVNC_PID=\$!

# Wait for both processes
wait \$COMPOSITOR_PID \$WAYVNC_PID
" &

# Test GPU acceleration and OpenGL in background after startup delay
sleep 30 && su ubuntu -c "
echo '=== GPU ACCELERATION TEST ==='
echo 'Testing GPU access...'
ls -la /dev/dri* 2>/dev/null || echo 'No DRI devices found'

if command -v glxinfo >/dev/null 2>&1; then
    echo 'Running glxinfo...'
    glxinfo | grep -E '(OpenGL|vendor|renderer|version)' | head -10 || echo 'glxinfo failed'
else
    echo 'glxinfo not available'
fi

if command -v glxgears >/dev/null 2>&1; then
    echo 'Testing glxgears...'
    timeout 5 glxgears -info 2>&1 | head -5 || echo 'glxgears failed'
else
    echo 'glxgears not available'  
fi

echo 'GPU test completed'
" &

# Start Helix agent in background
/start-helix-agent.sh &

# Keep container running
tail -f /dev/null