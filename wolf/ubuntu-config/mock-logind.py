#!/usr/bin/env python3
"""Mock logind D-Bus service for running GNOME Shell without real systemd-logind.

GNOME Shell in nested Wayland mode requires logind for session tracking.
This mock service provides the minimal D-Bus interface needed to satisfy
GNOME Shell's requirements when running in a container without systemd.

See: design/2025-12-29-gnome-nested-logind-mock.md
"""

import dbus
import dbus.service
import dbus.mainloop.glib
from gi.repository import GLib
import os
import sys

SESSION_ID = "c1"
SESSION_PATH = "/org/freedesktop/login1/session/c1"
USER_PATH = "/org/freedesktop/login1/user/_1000"  # logind escapes numeric IDs with underscore
SEAT_PATH = "/org/freedesktop/login1/seat/seat0"


class MockSeat(dbus.service.Object):
    """Mock logind seat object."""

    def __init__(self, bus, path):
        super().__init__(bus, path)

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='ss', out_signature='v')
    def Get(self, interface, prop):
        props = {
            'Id': 'seat0',
            'ActiveSession': dbus.Struct((SESSION_ID, dbus.ObjectPath(SESSION_PATH)), signature='so'),
            'CanGraphical': dbus.Boolean(True),
            'CanTTY': dbus.Boolean(True),
            'CanMultiSession': dbus.Boolean(True),
            'Sessions': dbus.Array([(SESSION_ID, dbus.ObjectPath(SESSION_PATH))], signature='(so)'),
        }
        if prop in props:
            return props[prop]
        raise dbus.exceptions.DBusException(f'Unknown property: {prop}')

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='s', out_signature='a{sv}')
    def GetAll(self, interface):
        return {
            'Id': 'seat0',
            'ActiveSession': dbus.Struct((SESSION_ID, dbus.ObjectPath(SESSION_PATH)), signature='so'),
            'CanGraphical': dbus.Boolean(True),
            'CanTTY': dbus.Boolean(True),
            'CanMultiSession': dbus.Boolean(True),
            'Sessions': dbus.Array([(SESSION_ID, dbus.ObjectPath(SESSION_PATH))], signature='(so)'),
        }


class MockUser(dbus.service.Object):
    """Mock logind user object."""

    def __init__(self, bus, path):
        super().__init__(bus, path)

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='ss', out_signature='v')
    def Get(self, interface, prop):
        props = {
            'UID': dbus.UInt32(os.getuid()),
            'GID': dbus.UInt32(os.getgid()),
            'Name': 'retro',
            'State': 'active',
            'RuntimePath': f'/run/user/{os.getuid()}',
            # Display is (so) - session id and object path for the graphical session
            'Display': dbus.Struct((SESSION_ID, dbus.ObjectPath(SESSION_PATH)), signature='so'),
            # Sessions is array of (so) - all sessions for this user
            'Sessions': dbus.Array([(SESSION_ID, dbus.ObjectPath(SESSION_PATH))], signature='(so)'),
            'IdleHint': dbus.Boolean(False),
            'Linger': dbus.Boolean(False),
        }
        if prop in props:
            return props[prop]
        raise dbus.exceptions.DBusException(f'Unknown property: {prop}')

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='s', out_signature='a{sv}')
    def GetAll(self, interface):
        return {
            'UID': dbus.UInt32(os.getuid()),
            'GID': dbus.UInt32(os.getgid()),
            'Name': 'retro',
            'State': 'active',
            'RuntimePath': f'/run/user/{os.getuid()}',
            'Display': dbus.Struct((SESSION_ID, dbus.ObjectPath(SESSION_PATH)), signature='so'),
            'Sessions': dbus.Array([(SESSION_ID, dbus.ObjectPath(SESSION_PATH))], signature='(so)'),
            'IdleHint': dbus.Boolean(False),
            'Linger': dbus.Boolean(False),
        }


class MockSession(dbus.service.Object):
    """Mock logind session object."""

    def __init__(self, bus, path):
        super().__init__(bus, path)

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='ss', out_signature='v')
    def Get(self, interface, prop):
        props = {
            'Id': SESSION_ID,
            'State': 'active',
            'Active': dbus.Boolean(True),
            'Type': 'wayland',
            'Class': 'user',
            'VTNr': dbus.UInt32(1),
            # User is (uo) - uid and object path
            'User': dbus.Struct((dbus.UInt32(os.getuid()), dbus.ObjectPath(USER_PATH)), signature='uo'),
            # Seat is (so) - seat name and object path
            'Seat': dbus.Struct(('seat0', dbus.ObjectPath(SEAT_PATH)), signature='so'),
        }
        if prop in props:
            return props[prop]
        raise dbus.exceptions.DBusException(f'Unknown property: {prop}')

    @dbus.service.method(dbus_interface='org.freedesktop.DBus.Properties',
                         in_signature='s', out_signature='a{sv}')
    def GetAll(self, interface):
        return {
            'Id': SESSION_ID,
            'State': 'active',
            'Active': dbus.Boolean(True),
            'Type': 'wayland',
            'Class': 'user',
            'VTNr': dbus.UInt32(1),
            'User': dbus.Struct((dbus.UInt32(os.getuid()), dbus.ObjectPath(USER_PATH)), signature='uo'),
            'Seat': dbus.Struct(('seat0', dbus.ObjectPath(SEAT_PATH)), signature='so'),
        }

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session")
    def Activate(self):
        pass

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session",
                         in_signature='uu', out_signature='hb')
    def TakeDevice(self, major, minor):
        """Return file descriptor for device (used for GPU access)."""
        try:
            dev_path = None
            if major == 226:  # DRM
                if minor < 128:
                    dev_path = f"/dev/dri/card{minor}"
                else:
                    dev_path = f"/dev/dri/renderD{minor}"
                if dev_path and os.path.exists(dev_path):
                    fd = os.open(dev_path, os.O_RDWR)
                    print(f"[mock-logind] TakeDevice({major}, {minor}) -> {dev_path}")
                    return (dbus.types.UnixFd(fd), dbus.Boolean(True))
        except Exception as e:
            print(f"[mock-logind] TakeDevice failed: {e}")
        fd = os.open("/dev/null", os.O_RDONLY)
        return (dbus.types.UnixFd(fd), dbus.Boolean(False))

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session",
                         in_signature='uu')
    def ReleaseDevice(self, major, minor):
        pass

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session",
                         in_signature='b')
    def TakeControl(self, force):
        print(f"[mock-logind] TakeControl({force})")
        pass

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session")
    def ReleaseControl(self):
        pass

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session",
                         in_signature='b')
    def SetIdleHint(self, idle):
        """Set idle hint for power management."""
        print(f"[mock-logind] SetIdleHint({idle})")
        pass

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Session",
                         in_signature='b')
    def SetLockedHint(self, locked):
        """Set locked hint."""
        print(f"[mock-logind] SetLockedHint({locked})")
        pass

    # Signals for session Lock/Unlock (GNOME Shell listens for these)
    @dbus.service.signal(dbus_interface="org.freedesktop.login1.Session")
    def Lock(self):
        """Emitted when session should be locked."""
        pass

    @dbus.service.signal(dbus_interface="org.freedesktop.login1.Session")
    def Unlock(self):
        """Emitted when session should be unlocked."""
        pass


class MockManager(dbus.service.Object):
    """Mock logind manager object."""

    def __init__(self, bus, path):
        super().__init__(bus, path)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         in_signature='u', out_signature='o')
    def GetSessionByPID(self, pid):
        print(f"[mock-logind] GetSessionByPID({pid}) -> {SESSION_PATH}")
        return dbus.ObjectPath(SESSION_PATH)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         in_signature='u', out_signature='o')
    def GetUser(self, uid):
        # logind escapes numeric IDs with underscore prefix
        print(f"[mock-logind] GetUser({uid}) -> {USER_PATH}")
        return dbus.ObjectPath(USER_PATH)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         in_signature='s', out_signature='o')
    def GetSession(self, session_id):
        print(f"[mock-logind] GetSession({session_id}) -> {SESSION_PATH}")
        return dbus.ObjectPath(SESSION_PATH)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         in_signature='s', out_signature='o')
    def GetSeat(self, seat_id):
        print(f"[mock-logind] GetSeat({seat_id}) -> {SEAT_PATH}")
        return dbus.ObjectPath(SEAT_PATH)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         in_signature='ssss', out_signature='h')
    def Inhibit(self, what, who, why, mode):
        """Return inhibit lock file descriptor."""
        fd = os.open("/dev/null", os.O_RDONLY)
        return dbus.types.UnixFd(fd)

    @dbus.service.method(dbus_interface="org.freedesktop.login1.Manager",
                         out_signature='a(susso)')
    def ListSessions(self):
        return [(SESSION_ID, dbus.UInt32(os.getuid()), "retro", "seat0", dbus.ObjectPath(SESSION_PATH))]


def main():
    dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)

    try:
        bus = dbus.SystemBus()
    except dbus.exceptions.DBusException as e:
        print(f"[mock-logind] Failed to connect to system bus: {e}")
        sys.exit(1)

    try:
        name = dbus.service.BusName("org.freedesktop.login1", bus)
    except dbus.exceptions.DBusException as e:
        print(f"[mock-logind] Failed to own org.freedesktop.login1: {e}")
        sys.exit(1)

    manager = MockManager(bus, "/org/freedesktop/login1")
    session = MockSession(bus, SESSION_PATH)
    user = MockUser(bus, USER_PATH)
    seat = MockSeat(bus, SEAT_PATH)

    print(f"[mock-logind] Mock logind started on system bus")
    print(f"[mock-logind]   Session: {SESSION_PATH}")
    print(f"[mock-logind]   User: {USER_PATH}")
    print(f"[mock-logind]   Seat: {SEAT_PATH}")

    loop = GLib.MainLoop()
    loop.run()


if __name__ == '__main__':
    main()
