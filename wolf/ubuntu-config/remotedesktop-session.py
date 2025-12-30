#!/usr/bin/env python3
"""
Wolf RemoteDesktop Session Manager for GNOME

This script creates and maintains a RemoteDesktop session that provides:
1. ScreenCast for video (PipeWire stream)
2. Input injection via D-Bus Notify* methods

The script uses a PERSISTENT D-Bus connection to keep the session alive.
Shell scripts using gdbus call create new connections per call, which
causes GNOME to clean up sessions when the connection closes.

Usage:
  remotedesktop-session.py

Environment:
  WOLF_SESSION_ID - The lobby ID to report the node ID to
  WOLF_LOBBY_SOCKET_PATH - Path to Wolf's lobby Unix socket
  XDG_RUNTIME_DIR - Where sockets are located
"""

import os
import sys
import json
import socket
import signal
import threading
import time
import subprocess
from gi.repository import GLib, Gio

# D-Bus interfaces
REMOTE_DESKTOP_BUS = "org.gnome.Mutter.RemoteDesktop"
REMOTE_DESKTOP_PATH = "/org/gnome/Mutter/RemoteDesktop"
REMOTE_DESKTOP_IFACE = "org.gnome.Mutter.RemoteDesktop"
REMOTE_DESKTOP_SESSION_IFACE = "org.gnome.Mutter.RemoteDesktop.Session"

SCREEN_CAST_BUS = "org.gnome.Mutter.ScreenCast"
SCREEN_CAST_PATH = "/org/gnome/Mutter/ScreenCast"
SCREEN_CAST_IFACE = "org.gnome.Mutter.ScreenCast"
SCREEN_CAST_SESSION_IFACE = "org.gnome.Mutter.ScreenCast.Session"
SCREEN_CAST_STREAM_IFACE = "org.gnome.Mutter.ScreenCast.Stream"


def log(msg):
    print(f"[remotedesktop-py] {msg}", file=sys.stderr, flush=True)


def report_to_wolf(socket_path: str, endpoint: str, data: dict) -> bool:
    """Report data to Wolf via its lobby socket using curl."""
    try:
        json_data = json.dumps(data)
        result = subprocess.run(
            ["curl", "-s", "--unix-socket", socket_path,
             "-X", "POST",
             "-H", "Content-Type: application/json",
             "-d", json_data,
             f"http://localhost{endpoint}"],
            capture_output=True,
            text=True,
            timeout=5
        )
        log(f"Wolf {endpoint} response: {result.stdout}")
        return '"success":true' in result.stdout
    except Exception as e:
        log(f"Failed to report to Wolf: {e}")
        return False


class RemoteDesktopSession:
    """Manages a GNOME RemoteDesktop session with persistent D-Bus connection."""

    def __init__(self):
        self.running = True
        self.bus = None
        self.rd_proxy = None
        self.rd_session_proxy = None
        self.sc_proxy = None
        self.sc_session_proxy = None
        self.sc_stream_proxy = None

        self.rd_session_path = None
        self.sc_session_path = None
        self.sc_stream_path = None
        self.node_id = None

        self.wolf_socket = os.environ.get("WOLF_LOBBY_SOCKET_PATH", "/var/run/wolf/lobby.sock")
        self.wolf_session_id = os.environ.get("WOLF_SESSION_ID", "")
        self.xdg_runtime_dir = os.environ.get("XDG_RUNTIME_DIR", "/run/user/1000")
        self.input_socket = f"{self.xdg_runtime_dir}/wolf-input.sock"

        if not self.wolf_session_id:
            log("ERROR: WOLF_SESSION_ID not set")
            sys.exit(1)

        log(f"Wolf session ID: {self.wolf_session_id}")
        log(f"Wolf socket: {self.wolf_socket}")

    def connect(self):
        """Connect to D-Bus and create proxies."""
        log("Connecting to session D-Bus...")
        self.bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)

        # Create proxy for RemoteDesktop service
        self.rd_proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,
            REMOTE_DESKTOP_BUS,
            REMOTE_DESKTOP_PATH,
            REMOTE_DESKTOP_IFACE,
            None
        )

        # Create proxy for ScreenCast service
        self.sc_proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,
            SCREEN_CAST_BUS,
            SCREEN_CAST_PATH,
            SCREEN_CAST_IFACE,
            None
        )

        log("D-Bus connected")

    def create_session(self):
        """Create RemoteDesktop and linked ScreenCast sessions."""
        # Create RemoteDesktop session
        log("Creating RemoteDesktop session...")
        result = self.rd_proxy.call_sync(
            "CreateSession",
            None,  # No parameters
            Gio.DBusCallFlags.NONE,
            -1, None
        )

        self.rd_session_path = result.unpack()[0]
        log(f"RemoteDesktop session: {self.rd_session_path}")

        # Create proxy for the session
        self.rd_session_proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,
            REMOTE_DESKTOP_BUS,
            self.rd_session_path,
            REMOTE_DESKTOP_SESSION_IFACE,
            None
        )

        # Get the session ID property (needed for linking ScreenCast)
        session_id = self.rd_session_proxy.get_cached_property("SessionId")
        if session_id:
            session_id_str = session_id.unpack()
            log(f"Session ID: {session_id_str}")
        else:
            # Fall back to extracting from path
            session_id_str = self.rd_session_path.split("/")[-1]
            log(f"Session ID (from path): {session_id_str}")

        # Create linked ScreenCast session
        log("Creating linked ScreenCast session...")
        # Build the options dict with proper GLib.Variant values
        options_dict = {
            "remote-desktop-session-id": GLib.Variant("s", session_id_str)
        }

        result = self.sc_proxy.call_sync(
            "CreateSession",
            GLib.Variant("(a{sv})", (options_dict,)),
            Gio.DBusCallFlags.NONE,
            -1, None
        )

        self.sc_session_path = result.unpack()[0]
        log(f"ScreenCast session: {self.sc_session_path}")

        # Create proxy for ScreenCast session
        self.sc_session_proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,
            SCREEN_CAST_BUS,
            self.sc_session_path,
            SCREEN_CAST_SESSION_IFACE,
            None
        )

        # Record virtual display
        log("Recording virtual display...")
        # Build options dict with proper GLib.Variant values
        record_options = {
            "cursor-mode": GLib.Variant("u", 1)  # Embedded cursor
        }

        try:
            result = self.sc_session_proxy.call_sync(
                "RecordVirtual",
                GLib.Variant("(a{sv})", (record_options,)),
                Gio.DBusCallFlags.NONE,
                -1, None
            )
        except Exception as e:
            log(f"RecordVirtual failed, trying RecordMonitor: {e}")
            result = self.sc_session_proxy.call_sync(
                "RecordMonitor",
                GLib.Variant("(sa{sv})", ("", record_options)),
                Gio.DBusCallFlags.NONE,
                -1, None
            )

        self.sc_stream_path = result.unpack()[0]
        log(f"Stream: {self.sc_stream_path}")

        # Create proxy for stream
        self.sc_stream_proxy = Gio.DBusProxy.new_sync(
            self.bus,
            Gio.DBusProxyFlags.NONE,
            None,
            SCREEN_CAST_BUS,
            self.sc_stream_path,
            SCREEN_CAST_STREAM_IFACE,
            None
        )

        # NOTE: PipeWire node ID is not available until after Start() is called
        # It will be retrieved in start() method

    def start(self):
        """Start the RemoteDesktop session."""
        log("Starting RemoteDesktop session...")
        self.rd_session_proxy.call_sync(
            "Start",
            None,
            Gio.DBusCallFlags.NONE,
            -1, None
        )
        log("Session started")

        # Get the PipeWire node ID with retry
        # The node ID may take a moment to become available after Start()
        log("Getting PipeWire node ID...")
        for attempt in range(10):
            time.sleep(0.5)

            # Re-create the stream proxy to refresh cached properties
            self.sc_stream_proxy = Gio.DBusProxy.new_sync(
                self.bus,
                Gio.DBusProxyFlags.NONE,
                None,
                SCREEN_CAST_BUS,
                self.sc_stream_path,
                SCREEN_CAST_STREAM_IFACE,
                None
            )

            node_id_variant = self.sc_stream_proxy.get_cached_property("PipeWireNodeId")
            if node_id_variant:
                self.node_id = node_id_variant.unpack()
                log(f"PipeWire node ID: {self.node_id} (attempt {attempt + 1})")
                return

            # Try fetching directly
            try:
                props = self.sc_stream_proxy.call_sync(
                    "org.freedesktop.DBus.Properties.Get",
                    GLib.Variant("(ss)", (SCREEN_CAST_STREAM_IFACE, "PipeWireNodeId")),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )
                self.node_id = props.unpack()[0]
                log(f"PipeWire node ID: {self.node_id} (attempt {attempt + 1}, via GetProperty)")
                return
            except Exception as e:
                log(f"Attempt {attempt + 1}: PipeWireNodeId not available yet: {e}")

        raise Exception("Failed to get PipeWire node ID after 10 attempts")

    def report_to_wolf(self):
        """Report node ID and input socket to Wolf."""
        if os.path.exists(self.wolf_socket):
            # Report node ID
            log("Reporting node ID to Wolf...")
            report_to_wolf(
                self.wolf_socket,
                "/set-pipewire-node-id",
                {"node_id": self.node_id, "session_path": self.rd_session_path}
            )

            # Report input socket
            log("Reporting input socket to Wolf...")
            report_to_wolf(
                self.wolf_socket,
                "/set-input-socket",
                {"input_socket": self.input_socket}
            )
        else:
            log(f"WARNING: Wolf socket not found at {self.wolf_socket}")

    def run_input_bridge(self):
        """Run the input bridge in the main thread."""
        log(f"Starting input bridge on {self.input_socket}...")

        # Remove existing socket
        try:
            os.unlink(self.input_socket)
        except FileNotFoundError:
            pass

        # Create Unix socket
        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(self.input_socket)
        os.chmod(self.input_socket, 0o777)
        server.listen(5)
        server.settimeout(1.0)

        log(f"Input bridge listening")

        while self.running:
            try:
                conn, addr = server.accept()
                thread = threading.Thread(target=self._handle_client, args=(conn,))
                thread.daemon = True
                thread.start()
            except socket.timeout:
                continue
            except Exception as e:
                if self.running:
                    log(f"Accept error: {e}")

        server.close()
        try:
            os.unlink(self.input_socket)
        except:
            pass

    def _handle_client(self, conn: socket.socket):
        """Handle input from Wolf."""
        log("Input client connected")
        buffer = ""

        try:
            while self.running:
                data = conn.recv(4096)
                if not data:
                    break

                buffer += data.decode("utf-8", errors="replace")

                while "\n" in buffer:
                    line, buffer = buffer.split("\n", 1)
                    line = line.strip()
                    if not line:
                        continue

                    try:
                        event = json.loads(line)
                        self._handle_input(event)
                    except json.JSONDecodeError:
                        pass

        except Exception as e:
            log(f"Client error: {e}")
        finally:
            conn.close()
            log("Input client disconnected")

    def _handle_input(self, data: dict):
        """Handle a single input event."""
        event_type = data.get("type")

        try:
            if event_type == "mouse_move_abs":
                stream = data.get("stream", self.sc_stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.rd_session_proxy.call_sync(
                    "NotifyPointerMotionAbsolute",
                    GLib.Variant("(sdd)", (stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "mouse_move_rel":
                dx = float(data.get("dx", 0))
                dy = float(data.get("dy", 0))
                self.rd_session_proxy.call_sync(
                    "NotifyPointerMotion",
                    GLib.Variant("(dd)", (dx, dy)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "button":
                button = int(data.get("button", 1))
                state = bool(data.get("state", False))
                self.rd_session_proxy.call_sync(
                    "NotifyPointerButton",
                    GLib.Variant("(ib)", (button, state)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "scroll":
                dy = int(data.get("dy", 0))
                dx = int(data.get("dx", 0))
                if dy != 0:
                    self.rd_session_proxy.call_sync(
                        "NotifyPointerAxisDiscrete",
                        GLib.Variant("(ui)", (0, dy)),
                        Gio.DBusCallFlags.NONE,
                        -1, None
                    )
                if dx != 0:
                    self.rd_session_proxy.call_sync(
                        "NotifyPointerAxisDiscrete",
                        GLib.Variant("(ui)", (1, dx)),
                        Gio.DBusCallFlags.NONE,
                        -1, None
                    )

            elif event_type == "scroll_smooth":
                dx = float(data.get("dx", 0.0))
                dy = float(data.get("dy", 0.0))
                flags = 4  # SOURCE_FINGER
                self.rd_session_proxy.call_sync(
                    "NotifyPointerAxis",
                    GLib.Variant("(udd)", (flags, dx, dy)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "key":
                keycode = int(data.get("keycode", 0))
                state = bool(data.get("state", False))
                self.rd_session_proxy.call_sync(
                    "NotifyKeyboardKeycode",
                    GLib.Variant("(ub)", (keycode, state)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_down":
                slot = int(data.get("slot", 0))
                stream = data.get("stream", self.sc_stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.rd_session_proxy.call_sync(
                    "NotifyTouchDown",
                    GLib.Variant("(usdd)", (slot, stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_motion":
                slot = int(data.get("slot", 0))
                stream = data.get("stream", self.sc_stream_path)
                x = float(data.get("x", 0))
                y = float(data.get("y", 0))
                self.rd_session_proxy.call_sync(
                    "NotifyTouchMotion",
                    GLib.Variant("(usdd)", (slot, stream, x, y)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

            elif event_type == "touch_up":
                slot = int(data.get("slot", 0))
                self.rd_session_proxy.call_sync(
                    "NotifyTouchUp",
                    GLib.Variant("(u,)", (slot,)),
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )

        except Exception as e:
            pass  # Silently ignore input errors

    def stop(self):
        """Stop the session."""
        log("Stopping...")
        self.running = False

        if self.rd_session_proxy:
            try:
                self.rd_session_proxy.call_sync(
                    "Stop",
                    None,
                    Gio.DBusCallFlags.NONE,
                    -1, None
                )
            except:
                pass

    def run(self):
        """Main entry point."""
        try:
            self.connect()
            self.create_session()
            self.start()
            self.report_to_wolf()
            self.run_input_bridge()
        except Exception as e:
            log(f"ERROR: {e}")
            import traceback
            traceback.print_exc()
            sys.exit(1)
        finally:
            self.stop()


def main():
    session = RemoteDesktopSession()

    def signal_handler(sig, frame):
        log(f"Received signal {sig}")
        session.stop()

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    session.run()


if __name__ == "__main__":
    main()
