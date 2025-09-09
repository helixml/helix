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

# Weston configuration for RDP backend with 4K support
mkdir -p /home/ubuntu/.config
cat > /home/ubuntu/.config/weston.ini << 'WESTONCONF'
[core]
backend=rdp-backend.so
require-input=false

[rdp]
bind-address=0.0.0.0
port=3389
width=3840
height=2160
refresh-rate=60

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

# Generate temporary RDP keys for Weston
mkdir -p /tmp/rdp-keys
cd /tmp/rdp-keys

# Try winpr-makecert first, fallback to openssl if not available
if command -v winpr-makecert >/dev/null 2>&1; then
    echo \"Using winpr-makecert for RDP key generation\"
    winpr-makecert -rdp -silent -n rdp-security 2>/dev/null || echo \"winpr-makecert failed\"
    KEY_FILE=\"/tmp/rdp-keys/rdp-security.key\"
else
    echo \"Using openssl for RDP key generation\"
    openssl genrsa -out rdp.key 2048 2>/dev/null || echo \"OpenSSL key generation failed\"
    KEY_FILE=\"/tmp/rdp-keys/rdp.key\"
fi

# Start Weston with RDP backend for 4K@60Hz
weston --backend=rdp-backend.so --rdp4-key=\$KEY_FILE &
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