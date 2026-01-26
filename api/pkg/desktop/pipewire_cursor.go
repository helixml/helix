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

// SPA_PARAM_META_type and SPA_PARAM_META_size from spa/param/param.h
#ifndef SPA_PARAM_META_type
#define SPA_PARAM_META_type 1
#endif
#ifndef SPA_PARAM_META_size
#define SPA_PARAM_META_size 2
#endif

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

    // Flag to track if we've already called update_params
    int params_set;
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

    // Debug: log what meta types are in the buffer
    static int debug_count = 0;
    if (debug_count < 5) {
        debug_count++;
        fprintf(stderr, "[CURSOR_CLIENT] buffer has %d metas: ", buffer->n_metas);
        for (uint32_t i = 0; i < buffer->n_metas; i++) {
            fprintf(stderr, "%d ", buffer->metas[i].type);
        }
        fprintf(stderr, "(looking for SPA_META_Cursor=%d)\n", SPA_META_Cursor);
    }

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

    static int process_count = 0;
    process_count++;
    if (process_count <= 5 || process_count % 100 == 0) {
        fprintf(stderr, "[CURSOR_CLIENT] process callback #%d\n", process_count);
    }

    if ((b = pw_stream_dequeue_buffer(client->stream)) == NULL) {
        if (process_count <= 5) {
            fprintf(stderr, "[CURSOR_CLIENT] dequeue_buffer returned NULL\n");
        }
        return;
    }

    CursorInfo info;
    int result = extract_cursor_from_buffer(b->buffer, &info);

    if (process_count <= 5) {
        fprintf(stderr, "[CURSOR_CLIENT] extract_cursor result=%d id=%u pos=(%d,%d)\n",
            result, info.id, info.position_x, info.position_y);
    }

    if (result == 0) {
        // Check if cursor changed
        uint64_t hash = cursor_hash(&info);
        if (hash != client->last_cursor_hash) {
            client->last_cursor_hash = hash;

            fprintf(stderr, "[CURSOR_CLIENT] cursor changed: id=%u pos=(%d,%d) hotspot=(%d,%d) bitmap=%ux%u\n",
                info.id, info.position_x, info.position_y,
                info.hotspot_x, info.hotspot_y,
                info.bitmap_width, info.bitmap_height);

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

// Helper to get state name - PW_STREAM_STATE_ERROR = -1, UNCONNECTED = 0, CONNECTING = 1, PAUSED = 2, STREAMING = 3
static const char* pw_state_name(enum pw_stream_state s) {
    switch (s) {
        case PW_STREAM_STATE_ERROR: return "error";
        case PW_STREAM_STATE_UNCONNECTED: return "unconnected";
        case PW_STREAM_STATE_CONNECTING: return "connecting";
        case PW_STREAM_STATE_PAUSED: return "paused";
        case PW_STREAM_STATE_STREAMING: return "streaming";
        default: return "unknown";
    }
}

static void on_stream_state_changed(void *data, enum pw_stream_state old,
                                     enum pw_stream_state state, const char *error) {
    PwCursorClient *client = (PwCursorClient *)data;

    fprintf(stderr, "[CURSOR_CLIENT] state: %s -> %s\n", pw_state_name(old), pw_state_name(state));

    if (state == PW_STREAM_STATE_ERROR) {
        fprintf(stderr, "[CURSOR_CLIENT] stream error: %s\n", error ? error : "unknown");
        client->running = 0;
    } else if (state == PW_STREAM_STATE_UNCONNECTED) {
        client->running = 0;
    } else if (state == PW_STREAM_STATE_PAUSED) {
        // Stream is paused and ready - AUTOCONNECT flag should handle activation
        fprintf(stderr, "[CURSOR_CLIENT] stream paused, ready for buffers\n");
    } else if (state == PW_STREAM_STATE_STREAMING) {
        fprintf(stderr, "[CURSOR_CLIENT] stream now streaming\n");
    }
}

static void on_stream_param_changed(void *data, uint32_t id, const struct spa_pod *param) {
    PwCursorClient *client = (PwCursorClient *)data;

    fprintf(stderr, "[CURSOR_CLIENT] param_changed: id=%u has_param=%d\n", id, param != NULL);

    if (param == NULL || id != SPA_PARAM_Format) {
        return;
    }

    if (client->params_set) {
        fprintf(stderr, "[CURSOR_CLIENT] params already set, skipping\n");
        return;
    }

    fprintf(stderr, "[CURSOR_CLIENT] received Format param, requesting cursor metadata in buffers\n");

    // Request cursor metadata in buffer params
    // This tells PipeWire/Mutter we want SPA_META_Cursor in each buffer
    uint8_t buffer[1024];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));

    // Calculate cursor meta sizes like OBS does:
    // CURSOR_META_SIZE(w,h) = sizeof(spa_meta_cursor) + sizeof(spa_meta_bitmap) + w*h*4
    // = 28 + 20 + w*h*4 (spa_meta_cursor=28, spa_meta_bitmap=20)
    const int32_t cursor_meta_size_64 = 28 + 20 + 64 * 64 * 4;     // 16432 (default)
    const int32_t cursor_meta_size_1 = 28 + 20 + 1 * 1 * 4;        // 52 (min)
    const int32_t cursor_meta_size_256 = 28 + 20 + 256 * 256 * 4;  // 262192 (max)

    struct spa_pod *params[2];

    // Request cursor metadata with CHOICE_RANGE like OBS does
    // This allows GNOME to choose an appropriate size within our range
    params[0] = spa_pod_builder_add_object(&b,
        SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
        SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Cursor),
        SPA_PARAM_META_size, SPA_POD_CHOICE_RANGE_Int(cursor_meta_size_64, cursor_meta_size_1, cursor_meta_size_256));

    // Request header metadata (for timing info)
    params[1] = spa_pod_builder_add_object(&b,
        SPA_TYPE_OBJECT_ParamMeta, SPA_PARAM_Meta,
        SPA_PARAM_META_type, SPA_POD_Id(SPA_META_Header),
        SPA_PARAM_META_size, SPA_POD_Int(sizeof(struct spa_meta_header)));

    pw_stream_update_params(client->stream, (const struct spa_pod **)params, 2);

    client->params_set = 1;
    fprintf(stderr, "[CURSOR_CLIENT] requested cursor meta RANGE size: default=%d, min=%d, max=%d\n",
        cursor_meta_size_64, cursor_meta_size_1, cursor_meta_size_256);
}

// Create a new PipeWire cursor client
// If pipewire_fd > 0, uses pw_context_connect_fd to connect via portal FD
// Otherwise uses pw_context_connect to connect to default PipeWire socket
static PwCursorClient* pw_cursor_client_new(int pipewire_fd) {
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

    // Connect to PipeWire - use FD if provided (for portal ScreenCast access)
    if (pipewire_fd > 0) {
        fprintf(stderr, "[CURSOR_CLIENT] connecting via portal FD %d\n", pipewire_fd);
        client->core = pw_context_connect_fd(client->context, pipewire_fd, NULL, 0);
    } else {
        fprintf(stderr, "[CURSOR_CLIENT] connecting to default PipeWire socket\n");
        client->core = pw_context_connect(client->context, NULL, 0);
    }

    if (!client->core) {
        fprintf(stderr, "[CURSOR_CLIENT] failed to connect to PipeWire\n");
        pw_context_destroy(client->context);
        pw_main_loop_destroy(client->loop);
        free(client);
        return NULL;
    }

    fprintf(stderr, "[CURSOR_CLIENT] connected to PipeWire\n");
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

    // Build initial format params like gnome-remote-desktop does
    // We accept any raw video format - we only care about cursor metadata
    uint8_t buffer[4096];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));

    // Build a format pod that accepts any raw video
    // spa_format_video_raw_build() from spa/param/video/format-utils.h
    struct spa_video_info_raw info;
    memset(&info, 0, sizeof(info));
    info.format = SPA_VIDEO_FORMAT_UNKNOWN; // Accept any format
    info.size.width = 0;
    info.size.height = 0;
    info.framerate.num = 0;
    info.framerate.denom = 1;

    // Use spa_format_video_raw_build to create the format pod
    const struct spa_pod *format_param = spa_format_video_raw_build(&b,
        SPA_PARAM_EnumFormat,
        &info);

    const struct spa_pod *params[1];
    params[0] = format_param;

    fprintf(stderr, "[CURSOR_CLIENT] connecting with EnumFormat param (accept any video format)\\n");

    // Connect to the specific node with format params
    int connect_result = pw_stream_connect(client->stream,
                          PW_DIRECTION_INPUT,
                          node_id,
                          PW_STREAM_FLAG_AUTOCONNECT |
                          PW_STREAM_FLAG_MAP_BUFFERS,
                          params, 1);

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
// If pipeWireFd > 0, uses the portal FD for ScreenCast node access
// Otherwise connects to the default PipeWire socket
func NewPipeWireCursorClient(pipeWireFd int) (*PipeWireCursorClient, error) {
	client := C.pw_cursor_client_new(C.int(pipeWireFd))
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
