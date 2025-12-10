#!/bin/bash
# GOW Xwayland setup script for Helix Ubuntu container

export DISPLAY=:9

function wait_for_x_display() {
    local display=":9"
    local max_attempts=100
    local attempt=0
    while ! xdpyinfo -display "$display" >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "Xwayland failed to start on display $display"
            exit 1
        fi
        sleep 0.1
    done
}

function wait_for_dbus() {
    local max_attempts=100
    local attempt=0
    while ! dbus-send --system --dest=org.freedesktop.DBus --type=method_call --print-reply \
        /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; do
        if (( attempt++ >= max_attempts )); then
            echo "DBus failed to start"
            exit 1
        fi
        sleep 0.1
    done
}

function launch_xorg() {
    # Start Xwayland at :9
    Xwayland :9 &
    wait_for_x_display
    XWAYLAND_OUTPUT=$(xrandr --display :9 | grep " connected" | awk '{print $1}')

    xrandr --output "$XWAYLAND_OUTPUT" --mode "${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}"
}

launch_xorg
sudo /opt/gow/dbus.sh  # Start dbus as root
wait_for_dbus
/opt/gow/desktop.sh    # Start desktop
