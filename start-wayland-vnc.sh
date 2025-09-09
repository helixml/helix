#!/bin/bash
set -e

# Weston + VNC startup script for 4K@60Hz with proper input support

# Ensure proper ownership of config directories (run as root before su)
mkdir -p /home/ubuntu/.config
chown -R ubuntu:ubuntu /home/ubuntu/.config

# Start Weston with proper VNC support
su ubuntu -c "
export USER=ubuntu
export HOME=/home/ubuntu
export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu
mkdir -p /tmp/runtime-ubuntu
chmod 700 /tmp/runtime-ubuntu

# GPU acceleration
export NVIDIA_VISIBLE_DEVICES=all
export NVIDIA_DRIVER_CAPABILITIES=all
export GBM_BACKEND=nvidia-drm
export __GLX_VENDOR_LIBRARY_NAME=nvidia

# Weston configuration for VNC backend without TLS for easier connection
mkdir -p /home/ubuntu/.config
cat > /home/ubuntu/.config/weston.ini << 'WESTONCONF'
[core]
backend=vnc-backend.so
renderer=gl

[vnc]
refresh-rate=60

[output]
name=vnc
mode=3840x2160
resizeable=false

[libinput]
enable-tap=true

[shell]
background-color=0xff1a1a1a
panel-color=0x90ffffff
locking=false
WESTONCONF

# Start dbus session
if [ -z \"\$DBUS_SESSION_BUS_ADDRESS\" ]; then
    eval \$(dbus-launch --sh-syntax)
fi

echo \"Starting Weston with 4K@60Hz RDP support...\"

# Generate TLS key and certificate for Weston RDP backend
mkdir -p /tmp/rdp-keys
cd /tmp/rdp-keys

echo \"Generating TLS key and certificate for RDP backend...\"

# Generate RSA private key
openssl genrsa -out tls.key 2048

# Generate certificate signing request (non-interactive)
openssl req -new -key tls.key -out tls.csr -subj \"/C=US/ST=CA/L=SF/O=Helix/CN=localhost\"

# Generate self-signed certificate
openssl x509 -req -days 365 -signkey tls.key -in tls.csr -out tls.crt

# Generate client certificate for Remmina
echo \"Generating client certificate for VNC client...\"
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj \"/C=US/ST=CA/L=SF/O=Helix/CN=client\"
openssl x509 -req -days 365 -signkey client.key -in client.csr -out client.crt

# Verify files were created
if [ ! -f \"tls.key\" ] || [ ! -f \"tls.crt\" ] || [ ! -f \"client.key\" ] || [ ! -f \"client.crt\" ]; then
    echo \"ERROR: Failed to generate TLS certificates\"
    exit 1
fi

echo \"TLS files generated successfully:\"
ls -la tls.key tls.crt client.key client.crt

# Start Weston with VNC backend for 4K@60Hz using TLS
#weston --backend=vnc-backend.so --address=0.0.0.0 --port=5900 --width=3840 --height=2160 --vnc-tls-key=/tmp/rdp-keys/tls.key --vnc-tls-cert=/tmp/rdp-keys/tls.crt &
weston --backend=vnc-backend.so --address=0.0.0.0 --port=5900 --width=3840 --height=2160 --disable-transport-layer-security &
COMPOSITOR_PID=\$!

# Wait for Weston to start
sleep 8

echo \"Weston started, launching terminal...\"

# Start terminal - try multiple options for Wayland compatibility
if command -v weston-terminal >/dev/null 2>&1; then
    echo \"Starting weston-terminal...\"
    WAYLAND_DISPLAY=wayland-1 weston-terminal &
elif command -v kitty >/dev/null 2>&1; then
    echo \"Starting kitty terminal...\"
    WAYLAND_DISPLAY=wayland-1 kitty &
else
    echo \"Starting ghostty with explicit Wayland backend...\"
    WAYLAND_DISPLAY=wayland-1 GDK_BACKEND=wayland ghostty &
fi

# Wait for compositor
wait \$COMPOSITOR_PID
"

# RDP is built into Weston, no separate server needed
echo "RDP server started on port 3389"
