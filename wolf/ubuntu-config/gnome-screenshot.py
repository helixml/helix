#!/usr/bin/env python3
"""
GNOME 49+ Screenshot Capture using existing RemoteDesktop/ScreenCast session.

This script captures a screenshot from the PipeWire stream that was created by
remotedesktop-session.py. Instead of creating a new ScreenCast session (which
fails due to D-Bus connection cleanup), we use the existing stream.

The PipeWire node ID is saved to /tmp/pipewire-node-id by remotedesktop-session.py
when it starts. We read that and capture a frame using GStreamer.

Usage: gnome-screenshot.py /path/to/output.png
"""

import os
import sys
import subprocess
import time

def log(msg):
    print(f"[gnome-screenshot] {msg}", file=sys.stderr, flush=True)

def get_pipewire_node_id():
    """Get the PipeWire node ID from the saved file or D-Bus."""
    # First try the saved file (written by remotedesktop-session.py)
    node_file = "/tmp/pipewire-node-id"
    if os.path.exists(node_file):
        try:
            with open(node_file, 'r') as f:
                node_id = f.read().strip()
                if node_id:
                    log(f"Got PipeWire node ID from file: {node_id}")
                    return int(node_id)
        except Exception as e:
            log(f"Failed to read node ID file: {e}")

    # If no file, try to get it from pw-cli
    try:
        result = subprocess.run(
            ["pw-cli", "ls", "Node"],
            capture_output=True,
            text=True,
            timeout=5
        )
        # Look for ScreenCast node
        for line in result.stdout.split('\n'):
            if 'id' in line.lower() and 'screen' in result.stdout.lower():
                # Parse node ID from pw-cli output
                parts = line.split()
                for part in parts:
                    if part.isdigit():
                        node_id = int(part)
                        log(f"Found PipeWire node from pw-cli: {node_id}")
                        return node_id
    except Exception as e:
        log(f"pw-cli failed: {e}")

    return None

def capture_frame(node_id, output_file, timeout_secs=5):
    """Capture a single frame from PipeWire stream using GStreamer."""
    log(f"Capturing frame from PipeWire node {node_id}...")

    # Use GStreamer to capture one frame from the PipeWire stream
    # pipewiresrc: reads from PipeWire node
    # num-buffers=1: capture exactly one frame
    # videoconvert: convert to compatible format
    # pngenc: encode as PNG
    #
    # IMPORTANT: gst-launch-1.0 requires ! as separate arguments, not stripped
    cmd = [
        "gst-launch-1.0", "-q",
        "pipewiresrc", f"path={node_id}", "num-buffers=1", "do-timestamp=true",
        "!", "videoconvert",
        "!", "pngenc",
        "!", "filesink", f"location={output_file}"
    ]
    log(f"Running: {' '.join(cmd)}")

    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout_secs
        )

        # Check if output was created
        if os.path.exists(output_file) and os.path.getsize(output_file) > 0:
            size = os.path.getsize(output_file)
            log(f"Screenshot saved: {output_file} ({size} bytes)")
            return True
        else:
            log(f"GStreamer produced no output. stderr: {result.stderr}")
            return False

    except subprocess.TimeoutExpired:
        log(f"GStreamer capture timed out after {timeout_secs}s")
        return False
    except Exception as e:
        log(f"GStreamer capture failed: {e}")
        return False

def main():
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/screenshot.png"

    # Get the PipeWire node ID
    node_id = get_pipewire_node_id()

    if node_id is None:
        log("ERROR: Could not find PipeWire node ID")
        log("Make sure remotedesktop-session.py is running and has saved the node ID")
        sys.exit(1)

    # Capture the frame
    if capture_frame(node_id, output_file):
        print(output_file)  # Print path for caller to read
        sys.exit(0)
    else:
        sys.exit(1)

if __name__ == "__main__":
    main()
