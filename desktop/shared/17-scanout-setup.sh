#!/bin/bash
# Start system D-Bus daemon for scanout mode (macOS ARM).
# logind-stub needs the system bus to register org.freedesktop.login1.
# This script runs as root during container init (before gosu).
if [ "${GPU_VENDOR}" = "virtio" ] && [ ! -S /var/run/dbus/system_bus_socket ]; then
    echo "**** Starting system D-Bus for scanout mode ****"
    mkdir -p /var/run/dbus
    dbus-daemon --system --fork 2>/dev/null && echo "System D-Bus started" || { echo "FATAL: Failed to start system D-Bus"; exit 1; }
fi
