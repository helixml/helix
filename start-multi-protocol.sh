#!/bin/bash
set -e

# Multi-Protocol Remote Desktop Server
# Supports VNC, RDP, and Moonlight protocols for performance testing

echo "🚀 Starting Multi-Protocol Remote Desktop Server..."
echo "📊 Protocols: VNC (5901), RDP (3389), Moonlight (47984+)"

# Environment setup
export USER=${USER:-ubuntu}
export HOME=/home/$USER
export XDG_RUNTIME_DIR=/tmp/runtime-$USER
export WAYLAND_DISPLAY=wayland-0
export DISPLAY=:1

# GPU acceleration environment
export NVIDIA_VISIBLE_DEVICES=all
export NVIDIA_DRIVER_CAPABILITIES=all
export GBM_BACKEND=nvidia-drm
export __GLX_VENDOR_LIBRARY_NAME=nvidia
export WLR_NO_HARDWARE_CURSORS=1
export WLR_DRM_NO_ATOMIC=1
export WLR_RENDERER=gles2
export WLR_BACKENDS=headless

# Wolf/Moonlight environment
export WOLF_CFG_FOLDER=/etc/wolf/cfg
export WOLF_RENDER_NODE=/dev/dri/renderD128
export WOLF_ENCODER_NODE=$WOLF_RENDER_NODE
export GST_GL_DRM_DEVICE=$WOLF_ENCODER_NODE
export GST_PLUGIN_PATH=/usr/local/lib/x86_64-linux-gnu/gstreamer-1.0/

# Create necessary directories
mkdir -p $XDG_RUNTIME_DIR $WOLF_CFG_FOLDER
chmod 700 $XDG_RUNTIME_DIR
chown $USER:$USER $XDG_RUNTIME_DIR

# Set up audio (required for RDP and Moonlight)
echo "🔊 Setting up audio subsystem..."
# Start PipeWire for modern audio
if command -v pipewire >/dev/null 2>&1; then
    su $USER -c "pipewire &"
    su $USER -c "pipewire-pulse &"
    echo "✅ PipeWire audio started"
else
    # Fallback to PulseAudio
    su $USER -c "pulseaudio --start --exit-idle-time=-1" || echo "⚠️  Audio setup failed, continuing..."
    echo "✅ PulseAudio fallback started"
fi

# Protocol selection via environment variable
PROTOCOL=${REMOTE_PROTOCOL:-all}  # all, vnc, rdp, moonlight

echo "📡 Starting Hyprland compositor with 4K@120Hz GPU acceleration..."

# Start Hyprland compositor (shared by all protocols)
su $USER -c "
export XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR
export WAYLAND_DISPLAY=$WAYLAND_DISPLAY
export WLR_HEADLESS_OUTPUTS=1:3840x2160@120
export WLR_NO_HARDWARE_CURSORS=1
export WLR_RENDERER=gles2
export WLR_BACKENDS=headless

cd $HOME
Hyprland &
HYPR_PID=\$!
echo \"Hyprland started with PID \$HYPR_PID\"
" &

# Wait for Hyprland to initialize
sleep 3

# Start VNC server
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "vnc" ]]; then
    echo "📺 Starting VNC server on port 5901..."
    su $USER -c "
    export XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR
    export WAYLAND_DISPLAY=$WAYLAND_DISPLAY
    wayvnc --render-cursor 0.0.0.0 5901 &
    "
    echo "✅ VNC server started: vnc://localhost:5901 (password: ${VNC_PASSWORD:-helix123})"
fi

# Start RDP server  
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "rdp" ]]; then
    echo "🖥️  Starting RDP server on port 3389..."
    
    # Configure xrdp for Wayland session
    cat > /etc/xrdp/startwm.sh << 'EOF'
#!/bin/bash
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
export WAYLAND_DISPLAY=wayland-0
export WLR_BACKENDS=headless
export WLR_RENDERER=gles2
# Start a nested Weston session for RDP compatibility
exec weston --backend=headless-backend.so --width=3840 --height=2160
EOF
    chmod +x /etc/xrdp/startwm.sh
    
    # Set RDP password for ubuntu user
    echo "ubuntu:${RDP_PASSWORD:-helix123}" | chpasswd
    
    # Start xrdp service
    service xrdp start
    echo "✅ RDP server started: rdp://localhost:3389 (user: ubuntu, password: ${RDP_PASSWORD:-helix123})"
fi

# Start Wolf Moonlight server
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "moonlight" ]]; then
    echo "🌙 Starting Wolf Moonlight server for high-performance streaming..."
    
    # Create Wolf configuration
    mkdir -p $WOLF_CFG_FOLDER
    cat > $WOLF_CFG_FOLDER/config.toml << EOF
# Wolf Moonlight configuration for Helix integration
[moonlight]
port = 47989
port_https = 47984

[gstreamer]
min_log_level = 2

[rtsp]
port = 48010

[input]
# Disable host input isolation for development
mode = REAL

[apps]
# Enable desktop streaming
EOF

    # Generate SSL certificates for Moonlight
    if [[ ! -f "$WOLF_CFG_FOLDER/cert.pem" ]]; then
        openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout $WOLF_CFG_FOLDER/key.pem \
            -out $WOLF_CFG_FOLDER/cert.pem \
            -subj "/C=US/ST=CA/L=SF/O=Helix/CN=localhost"
        echo "🔐 Generated SSL certificates for Moonlight"
    fi
    
    # Set Wolf environment
    export WOLF_CFG_FILE=$WOLF_CFG_FOLDER/config.toml
    export WOLF_PRIVATE_KEY_FILE=$WOLF_CFG_FOLDER/key.pem  
    export WOLF_PRIVATE_CERT_FILE=$WOLF_CFG_FOLDER/cert.pem
    export WOLF_LOG_LEVEL=INFO
    export RUST_BACKTRACE=full
    
    # Start Wolf server
    wolf &
    WOLF_PID=$!
    echo "✅ Wolf Moonlight server started: moonlight://localhost:47989 (PID: $WOLF_PID)"
    echo "🔗 Moonlight pairing: http://localhost:47989/pin/"
fi

# Start Helix External Agent Runner
echo "🤖 Starting Helix External Agent Runner..."
su $USER -c "
export XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR
export WAYLAND_DISPLAY=$WAYLAND_DISPLAY
export DISPLAY=$DISPLAY
export API_HOST=${API_HOST:-http://api:8080}
export API_TOKEN=${API_TOKEN:-oh-hallo-insecure-token}
export LOG_LEVEL=${LOG_LEVEL:-debug}
export CONCURRENCY=${CONCURRENCY:-1}
export MAX_TASKS=${MAX_TASKS:-0}
export WORKSPACE_DIR=${WORKSPACE_DIR:-/tmp/workspace}

cd /workspace/helix
/usr/local/bin/helix external-agent-runner &
"

echo ""
echo "🎉 Multi-Protocol Remote Desktop Server Ready!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "vnc" ]]; then
    echo "📺 VNC:      vnc://localhost:5901 (password: ${VNC_PASSWORD:-helix123})"
fi
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "rdp" ]]; then
    echo "🖥️  RDP:      rdp://localhost:3389 (ubuntu/${RDP_PASSWORD:-helix123})"
fi  
if [[ "$PROTOCOL" == "all" || "$PROTOCOL" == "moonlight" ]]; then
    echo "🌙 Moonlight: http://localhost:47989 (pair via web UI)"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Wait for all background processes
wait