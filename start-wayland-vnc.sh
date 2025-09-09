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

# Weston configuration for RDP backend with only real documented options
mkdir -p /home/ubuntu/.config
cat > /home/ubuntu/.config/weston.ini << 'WESTONCONF'
[core]
backend=rdp-backend.so
renderer=gl

[rdp]
bind-address=0.0.0.0
port=3389
width=3840
height=2160
refresh-rate=60
# force-no-compression=true
cursor=client

[output]
scale=2

[shell]
background-color=0xff1b1b26
panel-color=0xff241c2e
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

# Verify files were created
if [ ! -f \"tls.key\" ] || [ ! -f \"tls.crt\" ]; then
    echo \"ERROR: Failed to generate TLS key or certificate\"
    exit 1
fi

echo \"TLS files generated successfully:\"
ls -la tls.key tls.crt

# Start Weston with RDP backend for 4K@60Hz using TLS
weston --backend=rdp-backend.so --rdp-tls-key=/tmp/rdp-keys/tls.key --rdp-tls-cert=/tmp/rdp-keys/tls.crt &
COMPOSITOR_PID=\$!

# Wait for Weston to start
sleep 8

echo \"Weston started, launching terminal...\"

# Start Ghostty terminal in Weston
WAYLAND_DISPLAY=wayland-0 ghostty &

# Wait for compositor
wait \$COMPOSITOR_PID
"

# RDP is built into Weston, no separate server needed
echo "RDP server started on port 3389"
