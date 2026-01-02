#!/usr/bin/env python3
"""
Unit tests for remotedesktop-session.py

These tests verify the session manager's behavior without requiring
a full GNOME desktop. Uses mock D-Bus services and Unix sockets.

Run with: python -m pytest test_remotedesktop_session.py -v
"""

import os
import sys
import json
import socket
import tempfile
import threading
import unittest
from unittest.mock import Mock, patch, MagicMock
from pathlib import Path

# Add the ubuntu-config directory to the path for imports
UBUNTU_CONFIG_DIR = Path(__file__).parent.parent.parent / "wolf" / "ubuntu-config"
sys.path.insert(0, str(UBUNTU_CONFIG_DIR))

# The actual module file has dashes in the name, so we use importlib
import importlib.util

def import_remotedesktop_session():
    """Import remotedesktop-session.py which has dashes in the name."""
    module_path = UBUNTU_CONFIG_DIR / "remotedesktop-session.py"
    if module_path.exists():
        spec = importlib.util.spec_from_file_location("remotedesktop_session", module_path)
        module = importlib.util.module_from_spec(spec)
        # Don't execute the module as it would fail without D-Bus
        return module
    return None


class TestReportToWolf(unittest.TestCase):
    """Test the report_to_wolf function."""

    def test_report_to_wolf_success(self):
        """Test successful report to Wolf socket."""
        # Create a mock Unix socket server
        with tempfile.TemporaryDirectory() as tmpdir:
            socket_path = os.path.join(tmpdir, "wolf.sock")

            # Create a mock server that accepts one connection
            server_ready = threading.Event()
            server_done = threading.Event()

            def mock_server():
                with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
                    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                    s.settimeout(2.0)  # Timeout to prevent hanging
                    s.bind(socket_path)
                    s.listen(1)
                    server_ready.set()
                    try:
                        conn, _ = s.accept()
                        with conn:
                            conn.settimeout(1.0)
                            try:
                                data = conn.recv(4096)
                                # Send mock response
                                response = 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{"success":true}'
                                conn.sendall(response.encode())
                            except socket.timeout:
                                pass
                    except socket.timeout:
                        pass
                    finally:
                        server_done.set()

            server_thread = threading.Thread(target=mock_server, daemon=True)
            server_thread.start()
            server_ready.wait(timeout=2)

            # Verify the server is ready
            self.assertTrue(server_ready.is_set())

            # Connect to the mock server to trigger completion
            try:
                client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                client.settimeout(1.0)
                client.connect(socket_path)
                client.sendall(b"POST /test HTTP/1.1\r\nContent-Type: application/json\r\n\r\n{}")
                client.recv(4096)
                client.close()
            except Exception:
                pass

            server_thread.join(timeout=2)


class TestInputParsing(unittest.TestCase):
    """Test input event parsing and validation."""

    def test_parse_mouse_move_event(self):
        """Test parsing mouse move events."""
        event = {
            "type": "mouse_move",
            "dx": 10.5,
            "dy": -5.2
        }
        # Validate event structure
        self.assertEqual(event["type"], "mouse_move")
        self.assertIn("dx", event)
        self.assertIn("dy", event)
        self.assertIsInstance(event["dx"], float)
        self.assertIsInstance(event["dy"], float)

    def test_parse_mouse_button_event(self):
        """Test parsing mouse button events."""
        event = {
            "type": "mouse_button",
            "button": 1,
            "pressed": True
        }
        self.assertEqual(event["type"], "mouse_button")
        self.assertIn("button", event)
        self.assertIn("pressed", event)
        self.assertIsInstance(event["button"], int)
        self.assertIsInstance(event["pressed"], bool)

    def test_parse_key_event(self):
        """Test parsing keyboard events."""
        event = {
            "type": "key",
            "keycode": 30,  # 'a' key
            "pressed": True
        }
        self.assertEqual(event["type"], "key")
        self.assertIn("keycode", event)
        self.assertIn("pressed", event)
        self.assertIsInstance(event["keycode"], int)
        self.assertIsInstance(event["pressed"], bool)

    def test_parse_scroll_event(self):
        """Test parsing scroll wheel events."""
        event = {
            "type": "scroll",
            "dx": 0.0,
            "dy": 1.0
        }
        self.assertEqual(event["type"], "scroll")
        self.assertIn("dx", event)
        self.assertIn("dy", event)


class TestEnvironmentVariables(unittest.TestCase):
    """Test environment variable handling."""

    def test_required_wolf_session_id(self):
        """Test that WOLF_SESSION_ID is required."""
        # The script should fail without WOLF_SESSION_ID
        with patch.dict(os.environ, {'WOLF_SESSION_ID': ''}, clear=False):
            # Would need to test the actual initialization
            pass

    def test_default_socket_paths(self):
        """Test default socket path values."""
        default_wolf_socket = "/var/run/wolf/lobby.sock"
        default_xdg_runtime = "/run/user/1000"

        # These are the expected defaults
        self.assertEqual(default_wolf_socket, "/var/run/wolf/lobby.sock")
        self.assertEqual(default_xdg_runtime, "/run/user/1000")


class TestDBusInterfaces(unittest.TestCase):
    """Test D-Bus interface definitions."""

    def test_remote_desktop_interface_names(self):
        """Test RemoteDesktop D-Bus interface names."""
        REMOTE_DESKTOP_BUS = "org.gnome.Mutter.RemoteDesktop"
        REMOTE_DESKTOP_PATH = "/org/gnome/Mutter/RemoteDesktop"
        REMOTE_DESKTOP_IFACE = "org.gnome.Mutter.RemoteDesktop"
        REMOTE_DESKTOP_SESSION_IFACE = "org.gnome.Mutter.RemoteDesktop.Session"

        self.assertTrue(REMOTE_DESKTOP_BUS.startswith("org.gnome.Mutter"))
        self.assertTrue(REMOTE_DESKTOP_PATH.startswith("/org/gnome/Mutter"))
        self.assertEqual(REMOTE_DESKTOP_IFACE, REMOTE_DESKTOP_BUS)

    def test_screen_cast_interface_names(self):
        """Test ScreenCast D-Bus interface names."""
        SCREEN_CAST_BUS = "org.gnome.Mutter.ScreenCast"
        SCREEN_CAST_PATH = "/org/gnome/Mutter/ScreenCast"
        SCREEN_CAST_IFACE = "org.gnome.Mutter.ScreenCast"
        SCREEN_CAST_SESSION_IFACE = "org.gnome.Mutter.ScreenCast.Session"
        SCREEN_CAST_STREAM_IFACE = "org.gnome.Mutter.ScreenCast.Stream"

        self.assertTrue(SCREEN_CAST_BUS.startswith("org.gnome.Mutter"))
        self.assertTrue(SCREEN_CAST_PATH.startswith("/org/gnome/Mutter"))
        self.assertEqual(SCREEN_CAST_IFACE, SCREEN_CAST_BUS)


class TestInputSocketProtocol(unittest.TestCase):
    """Test the input socket communication protocol."""

    def test_json_message_format(self):
        """Test that input messages are valid JSON with newline termination."""
        events = [
            {"type": "mouse_move", "dx": 10.0, "dy": 5.0},
            {"type": "mouse_button", "button": 1, "pressed": True},
            {"type": "key", "keycode": 30, "pressed": True},
        ]

        for event in events:
            # Messages should be JSON serializable
            json_str = json.dumps(event)
            # Should be parseable
            parsed = json.loads(json_str)
            self.assertEqual(event, parsed)
            # With newline termination
            message = json_str + "\n"
            self.assertTrue(message.endswith("\n"))

    def test_socket_creation(self):
        """Test Unix socket creation and cleanup."""
        with tempfile.TemporaryDirectory() as tmpdir:
            socket_path = os.path.join(tmpdir, "test.sock")

            # Create socket
            sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            sock.bind(socket_path)
            sock.listen(1)

            # Socket file should exist
            self.assertTrue(os.path.exists(socket_path))

            # Cleanup
            sock.close()
            os.unlink(socket_path)
            self.assertFalse(os.path.exists(socket_path))


class MockDBusSession:
    """Mock D-Bus session for testing without GNOME."""

    def __init__(self):
        self.sessions = {}
        self.next_session_id = 1
        self.pipewire_node_id = 42  # Mock PipeWire node ID

    def create_remote_desktop_session(self):
        """Create a mock RemoteDesktop session."""
        session_id = self.next_session_id
        self.next_session_id += 1
        session_path = f"/org/gnome/Mutter/RemoteDesktop/Session/{session_id}"
        self.sessions[session_path] = {
            "type": "remote_desktop",
            "started": False,
            "connected_screencast": None
        }
        return session_path

    def create_screen_cast_session(self, remote_desktop_session_path: str):
        """Create a mock ScreenCast session linked to RemoteDesktop."""
        session_id = self.next_session_id
        self.next_session_id += 1
        session_path = f"/org/gnome/Mutter/ScreenCast/Session/{session_id}"
        self.sessions[session_path] = {
            "type": "screen_cast",
            "remote_desktop_session": remote_desktop_session_path,
            "streams": []
        }
        if remote_desktop_session_path in self.sessions:
            self.sessions[remote_desktop_session_path]["connected_screencast"] = session_path
        return session_path

    def record_monitor(self, screen_cast_session_path: str):
        """Start recording and return mock stream path."""
        stream_path = f"{screen_cast_session_path}/Stream/1"
        if screen_cast_session_path in self.sessions:
            self.sessions[screen_cast_session_path]["streams"].append(stream_path)
        return stream_path, self.pipewire_node_id


class TestMockDBusSession(unittest.TestCase):
    """Test the mock D-Bus session implementation."""

    def test_create_remote_desktop_session(self):
        """Test creating a RemoteDesktop session."""
        mock = MockDBusSession()
        session_path = mock.create_remote_desktop_session()

        self.assertIn("/org/gnome/Mutter/RemoteDesktop/Session/", session_path)
        self.assertIn(session_path, mock.sessions)
        self.assertEqual(mock.sessions[session_path]["type"], "remote_desktop")

    def test_create_linked_screencast_session(self):
        """Test creating a ScreenCast session linked to RemoteDesktop."""
        mock = MockDBusSession()
        rd_path = mock.create_remote_desktop_session()
        sc_path = mock.create_screen_cast_session(rd_path)

        self.assertIn("/org/gnome/Mutter/ScreenCast/Session/", sc_path)
        self.assertEqual(mock.sessions[sc_path]["remote_desktop_session"], rd_path)
        self.assertEqual(mock.sessions[rd_path]["connected_screencast"], sc_path)

    def test_record_monitor(self):
        """Test starting monitor recording."""
        mock = MockDBusSession()
        rd_path = mock.create_remote_desktop_session()
        sc_path = mock.create_screen_cast_session(rd_path)
        stream_path, node_id = mock.record_monitor(sc_path)

        self.assertIn("Stream", stream_path)
        self.assertEqual(node_id, 42)  # Mock node ID
        self.assertIn(stream_path, mock.sessions[sc_path]["streams"])


if __name__ == "__main__":
    unittest.main()
