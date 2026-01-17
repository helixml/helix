//go:build cgo

// Package desktop provides PipeWire cursor metadata reading.
// This reads cursor information directly from PipeWire buffer metadata,
// eliminating the need for file-based IPC with the Rust GStreamer plugin.
package desktop

/*
#cgo pkg-config: libpipewire-0.3 libspa-0.2
#include <pipewire/pipewire.h>
#include <spa/param/video/format-utils.h>
#include <spa/param/param.h>
#include <spa/param/buffers.h>
#include <spa/buffer/buffer.h>
#include <spa/buffer/meta.h>
#include <spa/utils/result.h>
#include <stdlib.h>
#include <string.h>

// Cursor data extracted from spa_meta_cursor
typedef struct {
    uint32_t id;
    int32_t position_x;
    int32_t position_y;
    int32_t hotspot_x;
    int32_t hotspot_y;
    uint32_t bitmap_width;
    uint32_t bitmap_height;
    int32_t bitmap_stride;
    uint32_t bitmap_format;
    uint32_t bitmap_size;
    uint8_t *bitmap_data;  // Caller must free if not NULL
} CursorInfo;

// Stream state
typedef struct {
    struct pw_main_loop *loop;
    struct pw_context *context;
    struct pw_core *core;
    struct pw_stream *stream;
    struct spa_hook stream_listener;
    int running;

    // Callback for cursor updates
    void (*cursor_callback)(CursorInfo *info, void *userdata);
    void *userdata;

    // Last cursor hash to avoid duplicate callbacks
    uint64_t last_cursor_hash;
} PwCursorClient;

// Forward declarations
static void on_stream_process(void *data);
static void on_stream_param_changed(void *data, uint32_t id, const struct spa_pod *param);
static void on_stream_state_changed(void *data, enum pw_stream_state old,
                                     enum pw_stream_state state, const char *error);

// Go callback declaration (implemented in Go with //export)
extern void goCursorCallback(CursorInfo *info, void *userdata);

static const struct pw_stream_events stream_events = {
    PW_VERSION_STREAM_EVENTS,
    .state_changed = on_stream_state_changed,
    .param_changed = on_stream_param_changed,
    .process = on_stream_process,
};

// Extract cursor metadata from buffer
static int extract_cursor_from_buffer(struct spa_buffer *buffer, CursorInfo *info) {
    struct spa_meta_cursor *cursor = NULL;

    // Find cursor metadata using spa_buffer_find_meta
    struct spa_meta *meta = spa_buffer_find_meta(buffer, SPA_META_Cursor);

    if (meta && meta->size >= sizeof(struct spa_meta_cursor)) {
        cursor = (struct spa_meta_cursor *)meta->data;
    }

    if (!cursor) {
        return -1;  // No cursor metadata
    }

    // Fill basic info
    info->id = cursor->id;
    info->position_x = cursor->position.x;
    info->position_y = cursor->position.y;
    info->hotspot_x = cursor->hotspot.x;
    info->hotspot_y = cursor->hotspot.y;
    info->bitmap_data = NULL;
    info->bitmap_size = 0;
    info->bitmap_width = 0;
    info->bitmap_height = 0;
    info->bitmap_stride = 0;
    info->bitmap_format = 0;

    // Check for bitmap data
    // bitmap_offset >= sizeof(spa_meta_cursor) means there's bitmap data
    if (cursor->bitmap_offset >= sizeof(struct spa_meta_cursor)) {
        struct spa_meta_bitmap *bitmap = SPA_PTROFF(cursor, cursor->bitmap_offset, struct spa_meta_bitmap);

        // Validate bitmap
        if (bitmap->format != 0 && bitmap->size.width > 0 && bitmap->size.height > 0) {
            info->bitmap_format = bitmap->format;
            info->bitmap_width = bitmap->size.width;
            info->bitmap_height = bitmap->size.height;
            info->bitmap_stride = bitmap->stride;

            // Calculate pixel data size and copy
            uint32_t pixel_size = abs(bitmap->stride) * bitmap->size.height;
            if (pixel_size > 0 && pixel_size < 4 * 1024 * 1024) {  // Sanity check: max 4MB
                uint8_t *pixel_data = SPA_PTROFF(bitmap, bitmap->offset, uint8_t);
                info->bitmap_data = (uint8_t *)malloc(pixel_size);
                if (info->bitmap_data) {
                    memcpy(info->bitmap_data, pixel_data, pixel_size);
                    info->bitmap_size = pixel_size;
                }
            }
        }
    }

    return 0;
}

// Simple hash for cursor deduplication
static uint64_t cursor_hash(CursorInfo *info) {
    uint64_t h = info->id;
    h = h * 31 + info->hotspot_x;
    h = h * 31 + info->hotspot_y;
    h = h * 31 + info->bitmap_width;
    h = h * 31 + info->bitmap_height;
    if (info->bitmap_data && info->bitmap_size >= 8) {
        h = h * 31 + *(uint64_t*)info->bitmap_data;
    }
    return h;
}

static void on_stream_process(void *data) {
    PwCursorClient *client = (PwCursorClient *)data;
    struct pw_buffer *b;

    if ((b = pw_stream_dequeue_buffer(client->stream)) == NULL) {
        return;
    }

    CursorInfo info;
    int result = extract_cursor_from_buffer(b->buffer, &info);

    if (result == 0) {
        // Check if cursor changed
        uint64_t hash = cursor_hash(&info);
        if (hash != client->last_cursor_hash) {
            client->last_cursor_hash = hash;

            // Call callback
            if (client->cursor_callback) {
                client->cursor_callback(&info, client->userdata);
            }
        }

        // Free bitmap data if allocated
        if (info.bitmap_data) {
            free(info.bitmap_data);
        }
    }

    pw_stream_queue_buffer(client->stream, b);
}

static void on_stream_state_changed(void *data, enum pw_stream_state old,
                                     enum pw_stream_state state, const char *error) {
    PwCursorClient *client = (PwCursorClient *)data;

    if (state == PW_STREAM_STATE_ERROR) {
        fprintf(stderr, "[CURSOR_CLIENT] stream error: %s\n", error ? error : "unknown");
        client->running = 0;
    } else if (state == PW_STREAM_STATE_UNCONNECTED) {
        client->running = 0;
    }
}

static void on_stream_param_changed(void *data, uint32_t id, const struct spa_pod *param) {
    PwCursorClient *client = (PwCursorClient *)data;

    if (param == NULL || id != SPA_PARAM_Format) {
        return;
    }

    // Handle Format (id=4) - final negotiated format
    if (id == SPA_PARAM_Format) {

        // Request cursor metadata and buffer types
        uint8_t buffer[2048];
        struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));

        // Request both SHM and DmaBuf buffer types
        // This matches what gnome-remote-desktop and OBS do
        uint32_t buffer_types = (1 << SPA_DATA_MemFd) | (1 << SPA_DATA_DmaBuf);

        // Cursor metadata size calculation (must match Mutter's CURSOR_META_SIZE macro)
        // sizeof(spa_meta_cursor) = 28, sizeof(spa_meta_bitmap) = 20
        #define CURSOR_META_SIZE(w, h) (28 + 20 + (w) * (h) * 4)

        const struct spa_pod *params[3];

        // 1. Buffer types param (with 0 terminator for varargs)
        params[0] = spa_pod_builder_add_object(&b,
            SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
            SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int(8, 2, 8),
            SPA_PARAM_BUFFERS_dataType, SPA_POD_Int(buffer_types),
            0);

        // 2. Header metadata (grd also requests this)
        params[1] = spa_pod_builder_add_object(&b,
            SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
            SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Header),
            SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_header)),
            0);

        // 3. Cursor metadata with enough space for cursor bitmap (384x384 to match Mutter)
        // gnome-remote-desktop uses CURSOR_META_SIZE(384, 384) as default
        params[2] = spa_pod_builder_add_object(&b,
            SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
            SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Cursor),
            SPA_PARAM_META_size, SPA_POD_CHOICE_RANGE_Int(
                CURSOR_META_SIZE(384, 384),  // Default: 384x384 cursor (matches Mutter)
                CURSOR_META_SIZE(1, 1),      // Min: 1x1 cursor
                CURSOR_META_SIZE(384, 384)   // Max: 384x384 cursor
            ),
            0);

        pw_stream_update_params(client->stream, params, 3);
    }
}

// Create a new PipeWire cursor client
static PwCursorClient* pw_cursor_client_new(void) {
    pw_init(NULL, NULL);

    PwCursorClient *client = calloc(1, sizeof(PwCursorClient));
    if (!client) return NULL;

    client->loop = pw_main_loop_new(NULL);
    if (!client->loop) {
        free(client);
        return NULL;
    }

    client->context = pw_context_new(pw_main_loop_get_loop(client->loop), NULL, 0);
    if (!client->context) {
        pw_main_loop_destroy(client->loop);
        free(client);
        return NULL;
    }

    client->core = pw_context_connect(client->context, NULL, 0);
    if (!client->core) {
        pw_context_destroy(client->context);
        pw_main_loop_destroy(client->loop);
        free(client);
        return NULL;
    }

    return client;
}

// Connect to a PipeWire stream by node ID (uses goCursorCallback)
static int pw_cursor_client_connect(PwCursorClient *client, uint32_t node_id, void *userdata) {
    if (!client) return -1;

    client->cursor_callback = goCursorCallback;
    client->userdata = userdata;
    client->running = 1;

    struct pw_properties *props = pw_properties_new(
        PW_KEY_MEDIA_TYPE, "Video",
        PW_KEY_MEDIA_CATEGORY, "Capture",
        PW_KEY_MEDIA_ROLE, "Screen",
        NULL);

    client->stream = pw_stream_new(client->core, "cursor-monitor", props);
    if (!client->stream) {
        return -1;
    }

    pw_stream_add_listener(client->stream, &client->stream_listener, &stream_events, client);

    // Don't specify format in connect - accept whatever GNOME offers
    // Format negotiation happens in on_stream_param_changed
    // Only pass NULL params to accept server defaults
    uint8_t buffer[4096];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
    (void)buffer;
    (void)b;

    // Connect to the specific node
    int connect_result = pw_stream_connect(client->stream,
                          PW_DIRECTION_INPUT,
                          node_id,
                          PW_STREAM_FLAG_AUTOCONNECT |
                          PW_STREAM_FLAG_MAP_BUFFERS,
                          NULL, 0);

    if (connect_result < 0) {
        pw_stream_destroy(client->stream);
        client->stream = NULL;
        return -1;
    }

    return 0;
}

// Run the main loop (blocking)
static void pw_cursor_client_run(PwCursorClient *client) {
    if (client && client->loop && client->running) {
        pw_main_loop_run(client->loop);
    }
}

// Stop the client
static void pw_cursor_client_stop(PwCursorClient *client) {
    if (client) {
        client->running = 0;
        if (client->loop) {
            pw_main_loop_quit(client->loop);
        }
    }
}

// Destroy the client
static void pw_cursor_client_destroy(PwCursorClient *client) {
    if (!client) return;

    if (client->stream) {
        pw_stream_destroy(client->stream);
    }
    if (client->core) {
        pw_core_disconnect(client->core);
    }
    if (client->context) {
        pw_context_destroy(client->context);
    }
    if (client->loop) {
        pw_main_loop_destroy(client->loop);
    }
    free(client);
}

// Iterate the loop once (non-blocking) - returns 0 if should continue, -1 if stopped
static int pw_cursor_client_iterate(PwCursorClient *client) {
    if (!client || !client->running) return -1;

    struct pw_loop *loop = pw_main_loop_get_loop(client->loop);
    pw_loop_iterate(loop, 0);  // 0 = non-blocking

    return client->running ? 0 : -1;
}
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"unsafe"
)

// CursorData represents cursor information from PipeWire
type CursorData struct {
	ID          uint32
	PositionX   int32
	PositionY   int32
	HotspotX    int32
	HotspotY    int32
	BitmapWidth  uint32
	BitmapHeight uint32
	BitmapStride int32
	BitmapFormat uint32
	BitmapData   []byte
}

// CursorCallback is called when cursor changes
type CursorCallback func(cursor *CursorData)

// PipeWireCursorClient reads cursor metadata directly from PipeWire
type PipeWireCursorClient struct {
	client   *C.PwCursorClient
	callback CursorCallback
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
}

// Global registry for callbacks (CGO can't call Go closures directly)
var (
	cursorCallbacksMu sync.RWMutex
	cursorCallbacks   = make(map[uintptr]*PipeWireCursorClient)
	nextCallbackID    uintptr
)

//export goCursorCallback
func goCursorCallback(info *C.CursorInfo, userdata unsafe.Pointer) {
	id := uintptr(userdata)

	cursorCallbacksMu.RLock()
	client := cursorCallbacks[id]
	cursorCallbacksMu.RUnlock()

	if client == nil || client.callback == nil {
		return
	}

	// Convert C struct to Go struct
	cursor := &CursorData{
		ID:           uint32(info.id),
		PositionX:    int32(info.position_x),
		PositionY:    int32(info.position_y),
		HotspotX:     int32(info.hotspot_x),
		HotspotY:     int32(info.hotspot_y),
		BitmapWidth:  uint32(info.bitmap_width),
		BitmapHeight: uint32(info.bitmap_height),
		BitmapStride: int32(info.bitmap_stride),
		BitmapFormat: uint32(info.bitmap_format),
	}

	// Copy bitmap data if present
	if info.bitmap_data != nil && info.bitmap_size > 0 {
		cursor.BitmapData = C.GoBytes(unsafe.Pointer(info.bitmap_data), C.int(info.bitmap_size))
	}

	client.callback(cursor)
}

// NewPipeWireCursorClient creates a new PipeWire cursor client
func NewPipeWireCursorClient() (*PipeWireCursorClient, error) {
	client := C.pw_cursor_client_new()
	if client == nil {
		return nil, fmt.Errorf("failed to create PipeWire cursor client")
	}

	return &PipeWireCursorClient{
		client: client,
		stopCh: make(chan struct{}),
	}, nil
}

// Connect to a PipeWire stream by node ID
func (p *PipeWireCursorClient) Connect(nodeID uint32, callback CursorCallback) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.callback = callback

	// Register callback
	cursorCallbacksMu.Lock()
	nextCallbackID++
	id := nextCallbackID
	cursorCallbacks[id] = p
	cursorCallbacksMu.Unlock()

	// Connect with C callback wrapper
	// Note: We pass the ID as userdata, the C code will call back to Go
	result := C.pw_cursor_client_connect(
		p.client,
		C.uint32_t(nodeID),
		unsafe.Pointer(id),
	)

	if result != 0 {
		cursorCallbacksMu.Lock()
		delete(cursorCallbacks, id)
		cursorCallbacksMu.Unlock()
		return fmt.Errorf("failed to connect to PipeWire stream %d", nodeID)
	}

	p.running = true
	return nil
}

// Run starts the cursor client in a goroutine
func (p *PipeWireCursorClient) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				p.Stop()
				return
			case <-p.stopCh:
				return
			default:
				p.mu.Lock()
				if !p.running {
					p.mu.Unlock()
					return
				}
				result := C.pw_cursor_client_iterate(p.client)
				p.mu.Unlock()

				if result != 0 {
					return
				}
			}
		}
	}()
}

// Stop stops the cursor client
func (p *PipeWireCursorClient) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	p.running = false
	C.pw_cursor_client_stop(p.client)

	close(p.stopCh)
}

// Close destroys the cursor client
func (p *PipeWireCursorClient) Close() {
	p.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		C.pw_cursor_client_destroy(p.client)
		p.client = nil
	}
}
