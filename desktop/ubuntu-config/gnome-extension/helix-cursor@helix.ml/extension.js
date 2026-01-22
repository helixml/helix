/**
 * Helix Cursor Tracker Extension
 *
 * Monitors cursor shape changes via Meta.CursorTracker and sends
 * cursor shape names to the Helix desktop-bridge via Unix socket.
 *
 * Architecture:
 *   Meta.CursorTracker -> This Extension -> Unix Socket -> Go Process -> WebSocket -> Frontend
 *
 * Socket Protocol:
 *   JSON messages with cursor shape information
 *   {
 *     "hotspot_x": number,
 *     "hotspot_y": number,
 *     "width": number,
 *     "height": number,
 *     "pixels": "",  // Always empty - we use fingerprinting instead
 *     "cursor_name": string  // CSS cursor name (e.g., "default", "pointer", "text")
 *   }
 *
 * Cursor Identification Strategy:
 *   Uses "Helix-Invisible" cursor theme where each cursor type has a unique
 *   hotspot position. By reading the hotspot, we can identify the cursor shape
 *   without needing to capture pixel data (which fails in headless mode anyway).
 */

import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const SOCKET_PATH = '/run/user/1000/helix-cursor.sock';

// Hotspot fingerprinting: map (hotspotX, hotspotY) to CSS cursor names
// These values match the Helix-Invisible cursor theme exactly
// Each cursor type has a unique hotspot position at 24x24 size
//
// Format: "hotspotX,hotspotY" -> "css-cursor-name"
// Note: We match on hotspot only (not size) for flexibility
const CURSOR_FINGERPRINTS = {
    // Arrow/pointer cursors (top-left area hotspots)
    '0,0': 'default',
    '6,0': 'pointer',
    '0,1': 'context-menu',
    '0,2': 'help',
    '0,3': 'progress',
    '0,4': 'copy',
    '0,5': 'alias',

    // Wait/busy (top-center)
    '12,0': 'wait',

    // Centered cursors with distinct Y values
    '12,1': 'crosshair',
    '12,2': 'move',
    '12,3': 'grab',
    '12,4': 'grabbing',
    '12,5': 'not-allowed',
    '12,6': 'cell',
    '12,7': 'zoom-in',
    '12,8': 'zoom-out',

    // Resize cursors (each direction has unique hotspot)
    '12,9': 'ns-resize',      // Vertical resize (north-south)
    '12,10': 'ew-resize',     // Horizontal resize (east-west)
    '12,11': 'nwse-resize',   // Diagonal resize (NW-SE / backslash) - top_left_corner, bottom_right_corner
    '12,12': 'text',          // Text cursor / I-beam
    '12,13': 'nesw-resize',   // Diagonal resize (NE-SW / forward slash) - top_right_corner, bottom_left_corner
};

export default class HelixCursorExtension extends Extension {
    constructor(metadata) {
        super(metadata);
        this._cursorTracker = null;
        this._cursorChangedId = 0;
        this._lastCursorHash = null;
    }

    enable() {
        console.log('[HelixCursor] Extension enabled');

        // Get cursor tracker for the display
        // In GNOME 45+, the cursor tracker is accessed via global.backend
        try {
            // Try global.backend.get_cursor_tracker() first (GNOME 45+)
            this._cursorTracker = global.backend.get_cursor_tracker();
            console.log('[HelixCursor] Got cursor tracker via global.backend');
        } catch (e) {
            console.log('[HelixCursor] backend.get_cursor_tracker() failed: ' + e);
            // Fallback attempts
            try {
                // Try direct import approach
                const Meta = imports.gi.Meta;
                this._cursorTracker = Meta.CursorTracker.get_for_display(global.display);
                console.log('[HelixCursor] Got cursor tracker via Meta.CursorTracker');
            } catch (e2) {
                console.log('[HelixCursor] Meta.CursorTracker failed: ' + e2);
            }
        }

        if (!this._cursorTracker) {
            console.log('[HelixCursor] ERROR: Could not get cursor tracker');
            return;
        }

        // Connect to cursor-changed signal
        this._cursorChangedId = this._cursorTracker.connect('cursor-changed', () => {
            this._onCursorChanged();
        });

        console.log('[HelixCursor] Connected to cursor-changed signal');

        // Send initial cursor after 1 second, then retry every 2 seconds until successful
        this._sentSuccessfully = false;
        this._retryCount = 0;
        this._initialTimeoutId = GLib.timeout_add(GLib.PRIORITY_DEFAULT, 1000, () => {
            this._sendInitialCursor();
            return GLib.SOURCE_REMOVE;
        });
    }

    _sendInitialCursor() {
        // Try to send cursor data, retry if socket doesn't exist yet
        const success = this._onCursorChanged(true);

        if (!success && this._retryCount < 30) {
            // Retry every 2 seconds for up to 60 seconds
            this._retryCount++;
            console.log('[HelixCursor] Socket not ready, will retry (' + this._retryCount + '/30)...');
            this._retryTimeoutId = GLib.timeout_add(GLib.PRIORITY_DEFAULT, 2000, () => {
                this._sendInitialCursor();
                return GLib.SOURCE_REMOVE;
            });
        }
    }

    disable() {
        console.log('[HelixCursor] Extension disabled');

        if (this._initialTimeoutId) {
            GLib.source_remove(this._initialTimeoutId);
            this._initialTimeoutId = null;
        }

        if (this._retryTimeoutId) {
            GLib.source_remove(this._retryTimeoutId);
            this._retryTimeoutId = null;
        }

        if (this._cursorChangedId && this._cursorTracker) {
            this._cursorTracker.disconnect(this._cursorChangedId);
            this._cursorChangedId = 0;
        }

        this._cursorTracker = null;
    }

    _onCursorChanged(forceResend = false) {
        try {
            // Get cursor sprite - returns CoglTexture in GNOME 49
            const sprite = this._cursorTracker.get_sprite();
            if (!sprite) {
                console.log('[HelixCursor] No cursor sprite available');
                return false;
            }

            // Get hotspot
            let hotspotX = 0, hotspotY = 0;
            try {
                [hotspotX, hotspotY] = this._cursorTracker.get_hot();
            } catch (e) {
                console.log('[HelixCursor] get_hot() not available: ' + e);
            }

            // In GNOME 49, get_sprite() returns CoglTexture directly
            let texture = sprite;
            if (sprite.get_texture) {
                texture = sprite.get_texture();
            }

            if (!texture) {
                console.log('[HelixCursor] No texture available');
                return false;
            }

            // Get dimensions
            let width = 0, height = 0;
            if (texture.get_width && texture.get_height) {
                width = texture.get_width();
                height = texture.get_height();
            } else if (sprite.get_preferred_size) {
                [width, height] = sprite.get_preferred_size();
            }

            if (width <= 0 || height <= 0) {
                console.log('[HelixCursor] Invalid cursor dimensions: ' + width + 'x' + height);
                return false;
            }

            // Simple hash to avoid duplicate sends (unless forceResend is true)
            const hash = `${width}x${height}@${hotspotX},${hotspotY}`;
            if (!forceResend && hash === this._lastCursorHash) {
                return true; // Already sent this cursor
            }

            // Use hotspot fingerprinting to identify cursor shape.
            // With Helix-Invisible cursor theme, each cursor type has a unique hotspot,
            // so we can reliably identify the shape without needing pixel data.
            // (The cursor pixels are transparent anyway, so capturing them is pointless.)
            const cursorName = this._getCursorNameFromFingerprint(hotspotX, hotspotY, width, height);
            console.log('[HelixCursor] Cursor changed: hotspot=(' + hotspotX + ',' + hotspotY + ') -> ' + cursorName);

            // Build message (pixels always empty - we use cursor_name for client-side rendering)
            const message = {
                hotspot_x: hotspotX,
                hotspot_y: hotspotY,
                width: width,
                height: height,
                pixels: '',
                cursor_name: cursorName
            };

            const success = this._sendToSocket(JSON.stringify(message));
            if (success) {
                this._lastCursorHash = hash;
                this._sentSuccessfully = true;
            }
            return success;

        } catch (e) {
            console.log('[HelixCursor] Error in _onCursorChanged: ' + e);
            return false;
        }
    }

    /**
     * Get CSS cursor name from hotspot fingerprint
     *
     * When using the Helix-Invisible cursor theme, each cursor type has a
     * unique (hotspotX, hotspotY) position, so we can reliably identify the
     * cursor shape from the hotspot alone.
     *
     * If no exact match is found (e.g., using a different theme), falls back
     * to 'default' which renders as an arrow cursor.
     */
    _getCursorNameFromFingerprint(hotspotX, hotspotY, width, height) {
        // Check exact match in fingerprint table (hotspot only)
        const key = `${hotspotX},${hotspotY}`;
        if (CURSOR_FINGERPRINTS[key]) {
            return CURSOR_FINGERPRINTS[key];
        }

        // No exact match - fall back to 'default' (arrow cursor)
        // This happens when using a theme other than Helix-Invisible
        console.log('[HelixCursor] No fingerprint match for hotspot (' + hotspotX + ',' + hotspotY + '), using default');
        return 'default';
    }

    _sendToSocket(message) {
        try {
            // Check if socket exists
            const socketFile = Gio.File.new_for_path(SOCKET_PATH);
            if (!socketFile.query_exists(null)) {
                // Socket doesn't exist yet, that's OK - Go process may not be running
                return false;
            }

            // Create socket address
            const socketAddress = Gio.UnixSocketAddress.new(SOCKET_PATH);

            // Create socket client
            const client = new Gio.SocketClient();

            // Connect (synchronous for simplicity)
            const connection = client.connect(socketAddress, null);
            if (!connection) {
                console.log('[HelixCursor] Could not connect to socket');
                return false;
            }

            // Send message with newline delimiter
            const outputStream = connection.get_output_stream();
            const data = message + '\n';
            outputStream.write_all(data, null);
            outputStream.flush(null);

            // Close connection
            connection.close(null);
            return true;

        } catch (e) {
            // Don't spam logs if socket isn't available
            if (!e.message.includes('No such file') && !e.message.includes('Connection refused')) {
                console.log('[HelixCursor] Socket error: ' + e);
            }
            return false;
        }
    }
}
