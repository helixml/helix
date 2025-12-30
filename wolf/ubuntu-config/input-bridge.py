#!/usr/bin/env python3
"""
Wolf Input Bridge for GNOME RemoteDesktop

This daemon receives input events from Wolf over a Unix socket and forwards
them to GNOME via the Mutter RemoteDesktop D-Bus API.

Input protocol (JSON lines over Unix socket):
  {"type": "mouse_move_abs", "x": 100, "y": 200, "stream": "/path/to/stream"}
  {"type": "mouse_move_rel", "dx": 10, "dy": -5}
  {"type": "button", "button": 1, "state": true}
  {"type": "scroll", "dx": 0, "dy": -1}
  {"type": "key", "keycode": 36, "state": true}
  {"type": "keysym", "keysym": 65293, "state": true}

The stream path is optional for mouse_move_abs; defaults to the session's stream.

Usage:
  input-bridge.py <session_path> <stream_path> <socket_path>
"""

import os
import sys
import json
import socket
import threading
import signal
from gi.repository import GLib, Gio

# D-Bus interfaces
REMOTE_DESKTOP_BUS = "org.gnome.Mutter.RemoteDesktop"
REMOTE_DESKTOP_IFACE = "org.gnome.Mutter.RemoteDesktop.Session"


class InputBridge:
    def __init__(self, session_path: str, stream_path: str, socket_path: str):
        self.session_path = session_path
        self.stream_path = stream_path
        self.socket_path = socket_path
        self.running = True

        # Connect to session D-Bus
        self.bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)

        # Create proxy for the RemoteDesktop session
        self.proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,  # interface info
            REMOTE_DESKTOP_BUS,
            session_path,
            REMOTE_DESKTOP_IFACE,
            None  # cancellable
        )

        print(f"[input-bridge] Connected to session: {session_path}", file=sys.stderr)
        print(f"[input-bridge] Stream path: {stream_path}", file=sys.stderr)

    def handle_input(self, data: dict):
        """Handle a single input event."""
        event_type = data.get("type")

        try:
            if event_type == "mouse_move_abs":
                stream = data.get("stream", self.stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.proxy.call_sync(
                    "NotifyPointerMotionAbsolute",
                    GLib.Variant("(sdd)", (stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "mouse_move_rel":
                dx = float(data.get("dx", 0))
                dy = float(data.get("dy", 0))
                self.proxy.call_sync(
                    "NotifyPointerMotion",
                    GLib.Variant("(dd)", (dx, dy)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "button":
                button = int(data.get("button", 1))
                state = bool(data.get("state", False))
                self.proxy.call_sync(
                    "NotifyPointerButton",
                    GLib.Variant("(ib)", (button, state)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "scroll":
                # Discrete scroll (legacy) - axis 0 for vertical, 1 for horizontal
                dy = int(data.get("dy", 0))
                dx = int(data.get("dx", 0))
                if dy != 0:
                    self.proxy.call_sync(
                        "NotifyPointerAxisDiscrete",
                        GLib.Variant("(ui)", (0, dy)),  # vertical
                        Gio.DBusCallFlags.NONE,
                        -1, None
                    )
                if dx != 0:
                    self.proxy.call_sync(
                        "NotifyPointerAxisDiscrete",
                        GLib.Variant("(ui)", (1, dx)),  # horizontal
                        Gio.DBusCallFlags.NONE,
                        -1, None
                    )

            elif event_type == "scroll_smooth":
                # Smooth/pixel-perfect scrolling via NotifyPointerAxis
                # This supports MacBook trackpads and high-resolution scroll wheels
                # Mutter expects (flags, dx, dy) where 10.0 = one discrete scroll step
                # Flags: 0 = none, 1 = finish, 2 = wheel, 4 = finger, 8 = continuous
                dx = float(data.get("dx", 0.0))
                dy = float(data.get("dy", 0.0))
                # Use SOURCE_FINGER flag (4) for smooth scrolling feel
                flags = 4  # META_REMOTE_DESKTOP_NOTIFY_AXIS_FLAGS_SOURCE_FINGER
                self.proxy.call_sync(
                    "NotifyPointerAxis",
                    GLib.Variant("(udd)", (flags, dx, dy)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "key":
                keycode = int(data.get("keycode", 0))
                state = bool(data.get("state", False))
                self.proxy.call_sync(
                    "NotifyKeyboardKeycode",
                    GLib.Variant("(ub)", (keycode, state)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "keysym":
                keysym = int(data.get("keysym", 0))
                state = bool(data.get("state", False))
                self.proxy.call_sync(
                    "NotifyKeyboardKeysym",
                    GLib.Variant("(ub)", (keysym, state)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_down":
                slot = int(data.get("slot", 0))
                stream = data.get("stream", self.stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.proxy.call_sync(
                    "NotifyTouchDown",
                    GLib.Variant("(usdd)", (slot, stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_motion":
                slot = int(data.get("slot", 0))
                stream = data.get("stream", self.stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.proxy.call_sync(
                    "NotifyTouchMotion",
                    GLib.Variant("(usdd)", (slot, stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_up":
                slot = int(data.get("slot", 0))
                self.proxy.call_sync(
                    "NotifyTouchUp",
                    GLib.Variant("(u,)", (slot,)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

        except Exception as e:
            # Log but don't crash on individual event errors
            print(f"[input-bridge] Error handling {event_type}: {e}", file=sys.stderr)

    def handle_client(self, conn: socket.socket, addr):
        """Handle a connected client."""
        print(f"[input-bridge] Client connected", file=sys.stderr)
        buffer = ""

        try:
            while self.running:
                data = conn.recv(4096)
                if not data:
                    break

                buffer += data.decode("utf-8", errors="replace")

                # Process complete JSON lines
                while "\n" in buffer:
                    line, buffer = buffer.split("\n", 1)
                    line = line.strip()
                    if not line:
                        continue

                    try:
                        event = json.loads(line)
                        self.handle_input(event)
                    except json.JSONDecodeError as e:
                        print(f"[input-bridge] Invalid JSON: {line[:100]}", file=sys.stderr)

        except Exception as e:
            print(f"[input-bridge] Client error: {e}", file=sys.stderr)
        finally:
            conn.close()
            print(f"[input-bridge] Client disconnected", file=sys.stderr)

    def run(self):
        """Main loop - listen for connections."""
        # Remove existing socket
        try:
            os.unlink(self.socket_path)
        except FileNotFoundError:
            pass

        # Create Unix socket
        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(self.socket_path)
        os.chmod(self.socket_path, 0o777)  # Allow container access
        server.listen(5)
        server.settimeout(1.0)  # Allow checking self.running

        print(f"[input-bridge] Listening on {self.socket_path}", file=sys.stderr)

        while self.running:
            try:
                conn, addr = server.accept()
                # Handle each client in a thread
                thread = threading.Thread(target=self.handle_client, args=(conn, addr))
                thread.daemon = True
                thread.start()
            except socket.timeout:
                continue
            except Exception as e:
                if self.running:
                    print(f"[input-bridge] Accept error: {e}", file=sys.stderr)

        server.close()
        try:
            os.unlink(self.socket_path)
        except:
            pass

        print(f"[input-bridge] Stopped", file=sys.stderr)

    def stop(self):
        """Stop the bridge."""
        self.running = False


def main():
    if len(sys.argv) != 4:
        print(f"Usage: {sys.argv[0]} <session_path> <stream_path> <socket_path>")
        sys.exit(1)

    session_path = sys.argv[1]
    stream_path = sys.argv[2]
    socket_path = sys.argv[3]

    bridge = InputBridge(session_path, stream_path, socket_path)

    # Handle signals
    def signal_handler(sig, frame):
        print(f"[input-bridge] Received signal {sig}, stopping...", file=sys.stderr)
        bridge.stop()

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    bridge.run()


if __name__ == "__main__":
    main()
