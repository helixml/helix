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

    # ========================================================================
    # Initialize GNOME Keyring with empty password (BEFORE gnome-session)
    # ========================================================================
    # This prevents the "Choose password for new keyring" prompt.
    # D-Bus is already running, so we can start the keyring daemon now.
    echo "Initializing GNOME Keyring with empty password..."
    mkdir -p ~/.local/share/keyrings
    echo "login" > ~/.local/share/keyrings/default

    # Start keyring daemon and unlock with empty password
    # The daemon will create the login keyring if it doesn't exist
    echo -n '' | gnome-keyring-daemon --start --unlock --components=secrets,pkcs11 > /tmp/keyring-init.log 2>&1

    # Export the keyring control socket for other apps
    export $(gnome-keyring-daemon --start --components=secrets,pkcs11 2>/dev/null)
    echo "GNOME Keyring initialized"

    # Load dconf settings for Ubuntu theming BEFORE gnome-session starts
    # This ensures wallpaper and theme are set from the very beginning
    if [ -f /opt/gow/dconf-settings.ini ]; then
        echo "Loading dconf settings from /opt/gow/dconf-settings.ini"
        dconf load / < /opt/gow/dconf-settings.ini || echo "Warning: dconf load failed"
    fi

    # Set scaling factor from HELIX_ZOOM_LEVEL (before gnome-session starts)
    # This ensures Zed and other apps launch with correct scaling
    ZOOM_LEVEL=${HELIX_ZOOM_LEVEL:-100}
    GNOME_SCALE=$((ZOOM_LEVEL / 100))
    if [ "$GNOME_SCALE" -lt 1 ]; then
        GNOME_SCALE=1
    fi
    echo "Setting GNOME scaling factor to $GNOME_SCALE (from HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)"
    gsettings set org.gnome.desktop.interface scaling-factor $GNOME_SCALE 2>/dev/null || true

    # Start GNOME session
    /usr/bin/dbus-launch /usr/bin/gnome-session
}

launch_desktop
