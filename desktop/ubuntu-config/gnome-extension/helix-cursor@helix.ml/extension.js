/**
 * Helix Cursor Tracker Extension
 *
 * Monitors cursor shape changes via Meta.CursorTracker and sends
 * cursor sprite data to the Helix desktop-bridge via Unix socket.
 *
 * Architecture:
 *   Meta.CursorTracker -> This Extension -> Unix Socket -> Go Process -> WebSocket -> Frontend
 *
 * Socket Protocol:
 *   JSON messages with cursor bitmap data (base64 encoded RGBA pixels)
 *   {
 *     "hotspot_x": number,
 *     "hotspot_y": number,
 *     "width": number,
 *     "height": number,
 *     "pixels": string (base64 encoded RGBA data)
 *   }
 */

import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import Cogl from 'gi://Cogl';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const SOCKET_PATH = '/run/user/1000/helix-cursor.sock';

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

            console.log('[HelixCursor] Cursor changed: ' + width + 'x' + height + ' hotspot=(' + hotspotX + ',' + hotspotY + ')');

            // Read pixel data from texture using pre-allocated buffer
            let pixelData = null;
            const rowstride = width * 4; // RGBA = 4 bytes per pixel
            const bufferSize = rowstride * height;

            try {
                if (texture.get_data) {
                    const buffer = new Uint8Array(bufferSize);
                    const result = texture.get_data(Cogl.PixelFormat.RGBA_8888, rowstride, buffer);
                    if (result > 0) {
                        pixelData = this._uint8ArrayToBase64(buffer);
                    }
                }
            } catch (e) {
                console.log('[HelixCursor] get_data() failed: ' + e);
            }

            // Build message
            const message = {
                hotspot_x: hotspotX,
                hotspot_y: hotspotY,
                width: width,
                height: height,
                pixels: pixelData || ''
            };

            const success = this._sendToSocket(JSON.stringify(message));
            if (success) {
                this._lastCursorHash = hash;
                this._sentSuccessfully = true;
                console.log('[HelixCursor] Cursor data sent successfully');
            }
            return success;

        } catch (e) {
            console.log('[HelixCursor] Error in _onCursorChanged: ' + e);
            return false;
        }
    }

    _uint8ArrayToBase64(uint8Array) {
        // Convert Uint8Array to base64 string
        let binary = '';
        const len = uint8Array.byteLength;
        for (let i = 0; i < len; i++) {
            binary += String.fromCharCode(uint8Array[i]);
        }
        return GLib.base64_encode(binary);
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
