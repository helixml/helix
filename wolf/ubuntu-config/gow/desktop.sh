#!/bin/bash
# GOW Desktop startup script for Ubuntu GNOME
# Adapted from Zorin GOW for pure Ubuntu 22.04

function launch_desktop() {
    # Launching X11 desktop, xorg server has already been launched.

    # For flatpak apps to appear in menus
    export XDG_DATA_DIRS=/var/lib/flatpak/exports/share:/home/retro/.local/share/flatpak/exports/share:/usr/local/share/:/usr/share/

    # Environment variables to ensure apps integrate well with GNOME
    # https://wiki.archlinux.org/title/Xdg-utils#Environment_variables
    export XDG_CURRENT_DESKTOP=ubuntu:GNOME
    export DE=gnome
    export DESKTOP_SESSION=ubuntu
    export GNOME_SHELL_SESSION_MODE="ubuntu"
    export XDG_SESSION_TYPE=x11

    # Various envs to help with apps compatibility
    export XDG_SESSION_CLASS="user"
    export _JAVA_AWT_WM_NONREPARENTING=1
    export GDK_BACKEND=x11
    export MOZ_ENABLE_WAYLAND=0
    export QT_QPA_PLATFORM="xcb"
    export QT_AUTO_SCREEN_SCALE_FACTOR=1
    export QT_ENABLE_HIGHDPI_SCALING=1
    export LC_ALL="en_US.UTF-8"

    # Set display, unset wayland display and get dbus envs
    export DISPLAY=:9
    unset WAYLAND_DISPLAY
    export $(dbus-launch)

    # Load dconf settings for Ubuntu theming BEFORE gnome-session starts
    # This ensures wallpaper and theme are set from the very beginning
    if [ -f /opt/gow/dconf-settings.ini ]; then
        echo "Loading dconf settings from /opt/gow/dconf-settings.ini"
        dconf load / < /opt/gow/dconf-settings.ini || echo "Warning: dconf load failed"
    fi

    # Set scaling factor (1 = no scaling)
    gsettings set org.gnome.desktop.interface scaling-factor 1 2>/dev/null || true

    # Start GNOME session
    /usr/bin/dbus-launch /usr/bin/gnome-session
}

launch_desktop
