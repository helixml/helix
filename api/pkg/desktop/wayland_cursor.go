//go:build cgo && linux

// Package desktop provides Wayland cursor metadata reading.
// This reads cursor information from Wayland's ext-image-copy-capture-cursor-session-v1 protocol,
// which is used by Sway and other wlroots-based compositors.
package desktop

/*
#cgo pkg-config: wayland-client
#cgo CFLAGS: -I/usr/share/wayland-protocols/staging -D_GNU_SOURCE

#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <unistd.h>
#include <poll.h>
#include <sys/mman.h>
#include <linux/memfd.h>
#include <wayland-client.h>

// Forward declarations for Wayland protocol interfaces
struct ext_output_image_capture_source_manager_v1;
struct ext_image_capture_source_v1;
struct ext_image_copy_capture_manager_v1;
struct ext_image_copy_capture_session_v1;
struct ext_image_copy_capture_frame_v1;
struct ext_image_copy_capture_cursor_session_v1;

// ==============================================================================
// Protocol interface definitions for ext-image-copy-capture-v1
// Generated from ext-image-copy-capture-v1.xml
// ==============================================================================

// Forward declare all interfaces
static const struct wl_interface ext_output_image_capture_source_manager_v1_interface;
static const struct wl_interface ext_image_capture_source_v1_interface;
static const struct wl_interface ext_image_copy_capture_manager_v1_interface;
static const struct wl_interface ext_image_copy_capture_session_v1_interface;
static const struct wl_interface ext_image_copy_capture_frame_v1_interface;
static const struct wl_interface ext_image_copy_capture_cursor_session_v1_interface;

// Type arrays for message signatures
static const struct wl_interface *source_manager_create_source_types[] = {
    &ext_image_capture_source_v1_interface,  // new_id
    &wl_output_interface,                     // output
};

static const struct wl_interface *capture_manager_create_session_types[] = {
    &ext_image_copy_capture_session_v1_interface,  // new_id
    &ext_image_capture_source_v1_interface,        // source
    NULL,  // options (uint)
};

static const struct wl_interface *capture_manager_create_cursor_types[] = {
    &ext_image_copy_capture_cursor_session_v1_interface,  // new_id
    &ext_image_capture_source_v1_interface,               // source
    &wl_pointer_interface,                                // pointer
};

static const struct wl_interface *cursor_session_get_session_types[] = {
    &ext_image_copy_capture_session_v1_interface,  // new_id
};

static const struct wl_interface *session_create_frame_types[] = {
    &ext_image_copy_capture_frame_v1_interface,  // new_id
};

static const struct wl_interface *frame_attach_buffer_types[] = {
    &wl_buffer_interface,  // buffer
};

// ext_output_image_capture_source_manager_v1 requests
static const struct wl_message ext_output_image_capture_source_manager_v1_requests[] = {
    { "create_source", "no", source_manager_create_source_types },
    { "destroy", "", NULL },
};

// ext_image_capture_source_v1 requests
static const struct wl_message ext_image_capture_source_v1_requests[] = {
    { "destroy", "", NULL },
};

// ext_image_copy_capture_manager_v1 requests
static const struct wl_message ext_image_copy_capture_manager_v1_requests[] = {
    { "create_session", "nou", capture_manager_create_session_types },
    { "create_pointer_cursor_session", "noo", capture_manager_create_cursor_types },
    { "destroy", "", NULL },
};

// ext_image_copy_capture_session_v1 requests
static const struct wl_message ext_image_copy_capture_session_v1_requests[] = {
    { "create_frame", "n", session_create_frame_types },
    { "destroy", "", NULL },
};

// ext_image_copy_capture_session_v1 events
static const struct wl_message ext_image_copy_capture_session_v1_events[] = {
    { "buffer_size", "uu", NULL },
    { "shm_format", "u", NULL },
    { "dmabuf_device", "a", NULL },
    { "dmabuf_format", "ua", NULL },
    { "done", "", NULL },
    { "stopped", "", NULL },
};

// ext_image_copy_capture_frame_v1 requests
static const struct wl_message ext_image_copy_capture_frame_v1_requests[] = {
    { "destroy", "", NULL },
    { "attach_buffer", "o", frame_attach_buffer_types },
    { "damage_buffer", "iiii", NULL },
    { "capture", "", NULL },
};

// ext_image_copy_capture_frame_v1 events
static const struct wl_message ext_image_copy_capture_frame_v1_events[] = {
    { "transform", "u", NULL },
    { "damage", "iiii", NULL },
    { "presentation_time", "uuu", NULL },
    { "ready", "", NULL },
    { "failed", "u", NULL },
};

// ext_image_copy_capture_cursor_session_v1 requests
static const struct wl_message ext_image_copy_capture_cursor_session_v1_requests[] = {
    { "destroy", "", NULL },
    { "get_capture_session", "n", cursor_session_get_session_types },
};

// ext_image_copy_capture_cursor_session_v1 events
static const struct wl_message ext_image_copy_capture_cursor_session_v1_events[] = {
    { "enter", "", NULL },
    { "leave", "", NULL },
    { "position", "ii", NULL },
    { "hotspot", "ii", NULL },
};

// Interface definitions
static const struct wl_interface ext_output_image_capture_source_manager_v1_interface = {
    "ext_output_image_capture_source_manager_v1", 1,
    2, ext_output_image_capture_source_manager_v1_requests,
    0, NULL,
};

static const struct wl_interface ext_image_capture_source_v1_interface = {
    "ext_image_capture_source_v1", 1,
    1, ext_image_capture_source_v1_requests,
    0, NULL,
};

static const struct wl_interface ext_image_copy_capture_manager_v1_interface = {
    "ext_image_copy_capture_manager_v1", 1,
    3, ext_image_copy_capture_manager_v1_requests,
    0, NULL,
};

static const struct wl_interface ext_image_copy_capture_session_v1_interface = {
    "ext_image_copy_capture_session_v1", 1,
    2, ext_image_copy_capture_session_v1_requests,
    6, ext_image_copy_capture_session_v1_events,
};

static const struct wl_interface ext_image_copy_capture_frame_v1_interface = {
    "ext_image_copy_capture_frame_v1", 1,
    4, ext_image_copy_capture_frame_v1_requests,
    5, ext_image_copy_capture_frame_v1_events,
};

static const struct wl_interface ext_image_copy_capture_cursor_session_v1_interface = {
    "ext_image_copy_capture_cursor_session_v1", 1,
    2, ext_image_copy_capture_cursor_session_v1_requests,
    4, ext_image_copy_capture_cursor_session_v1_events,
};

// Cursor state
typedef struct {
    int in_area;
    int32_t pos_x;
    int32_t pos_y;
    int32_t hotspot_x;
    int32_t hotspot_y;
    uint32_t bitmap_width;
    uint32_t bitmap_height;
    int32_t bitmap_stride;
    uint32_t bitmap_format;
    uint32_t bitmap_size;
    uint8_t *bitmap_data;  // Caller must free if not NULL
} WlCursorInfo;

// Client state
typedef struct {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_seat *seat;
    struct wl_pointer *pointer;
    struct wl_output *output;
    struct wl_shm *shm;

    struct ext_output_image_capture_source_manager_v1 *source_manager;
    struct ext_image_capture_source_v1 *source;
    struct ext_image_copy_capture_manager_v1 *capture_manager;
    struct ext_image_copy_capture_cursor_session_v1 *cursor_session;
    struct ext_image_copy_capture_session_v1 *cursor_capture_session;
    struct ext_image_copy_capture_frame_v1 *cursor_frame;

    // Current cursor state
    WlCursorInfo cursor;

    // Cursor bitmap capture state
    int shm_fd;
    void *shm_ptr;
    size_t shm_size;
    struct wl_shm_pool *shm_pool;
    struct wl_buffer *cursor_buffer;
    uint32_t cursor_buffer_width;
    uint32_t cursor_buffer_height;
    int32_t cursor_buffer_stride;
    uint32_t cursor_buffer_format;

    // Frame state
    int frame_ready;
    int frame_failed;
    int session_ready;
    int stop_requested;

    // Callback
    void (*callback)(WlCursorInfo *info, void *userdata);
    void *userdata;
} WlCursorClient;

// Go callback declaration
extern void goWaylandCursorCallback(WlCursorInfo *info, void *userdata);

// Protocol binding helpers using wl_proxy (libwayland internal API)
// These are needed because we don't have generated protocol code inline

// ext_output_image_capture_source_manager_v1 opcodes
#define EXT_OUTPUT_SOURCE_MANAGER_CREATE_SOURCE 0
#define EXT_OUTPUT_SOURCE_MANAGER_DESTROY 1

// ext_image_copy_capture_manager_v1 opcodes
#define EXT_CAPTURE_MANAGER_CREATE_SESSION 0
#define EXT_CAPTURE_MANAGER_CREATE_POINTER_CURSOR_SESSION 1
#define EXT_CAPTURE_MANAGER_DESTROY 2

// ext_image_copy_capture_session_v1 opcodes
#define EXT_CAPTURE_SESSION_CREATE_FRAME 0
#define EXT_CAPTURE_SESSION_DESTROY 1

// ext_image_copy_capture_cursor_session_v1 opcodes
#define EXT_CURSOR_SESSION_DESTROY 0
#define EXT_CURSOR_SESSION_GET_CAPTURE_SESSION 1

// ext_image_copy_capture_frame_v1 opcodes
#define EXT_FRAME_DESTROY 0
#define EXT_FRAME_ATTACH_BUFFER 1
#define EXT_FRAME_DAMAGE_BUFFER 2
#define EXT_FRAME_CAPTURE 3

// Cursor session events
static int cursor_callback_count = 0;

static void cursor_session_enter(void *data, struct ext_image_copy_capture_cursor_session_v1 *session) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->cursor.in_area = 1;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor entered capture area\n");
}

static void cursor_session_leave(void *data, struct ext_image_copy_capture_cursor_session_v1 *session) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->cursor.in_area = 0;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor left capture area\n");

    // Notify callback with empty cursor
    if (client->callback) {
        WlCursorInfo info = {0};
        client->callback(&info, client->userdata);
    }
}

static void cursor_session_position(void *data, struct ext_image_copy_capture_cursor_session_v1 *session, int32_t x, int32_t y) {
    WlCursorClient *client = (WlCursorClient *)data;
    int32_t old_x = client->cursor.pos_x;
    int32_t old_y = client->cursor.pos_y;
    client->cursor.pos_x = x;
    client->cursor.pos_y = y;

    // Trigger callback on position change (not just hotspot)
    if (client->callback && client->cursor.in_area && (x != old_x || y != old_y)) {
        cursor_callback_count++;
        if (cursor_callback_count <= 5 || cursor_callback_count % 100 == 0) {
            fprintf(stderr, "[WAYLAND_CURSOR] cursor position: (%d,%d) callback #%d\n", x, y, cursor_callback_count);
        }
        client->callback(&client->cursor, client->userdata);
    }
}

static void cursor_session_hotspot(void *data, struct ext_image_copy_capture_cursor_session_v1 *session, int32_t x, int32_t y) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->cursor.hotspot_x = x;
    client->cursor.hotspot_y = y;

    // When hotspot changes, trigger a callback with current state
    if (client->callback && client->cursor.in_area) {
        client->callback(&client->cursor, client->userdata);
    }
}

// Listener struct for cursor session
struct ext_image_copy_capture_cursor_session_v1_listener {
    void (*enter)(void *data, struct ext_image_copy_capture_cursor_session_v1 *session);
    void (*leave)(void *data, struct ext_image_copy_capture_cursor_session_v1 *session);
    void (*position)(void *data, struct ext_image_copy_capture_cursor_session_v1 *session, int32_t x, int32_t y);
    void (*hotspot)(void *data, struct ext_image_copy_capture_cursor_session_v1 *session, int32_t x, int32_t y);
};

static const struct ext_image_copy_capture_cursor_session_v1_listener cursor_session_listener = {
    .enter = cursor_session_enter,
    .leave = cursor_session_leave,
    .position = cursor_session_position,
    .hotspot = cursor_session_hotspot,
};

// Capture session events (for cursor bitmap capture)
static void cursor_capture_session_buffer_size(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t width, uint32_t height) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->cursor_buffer_width = width;
    client->cursor_buffer_height = height;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor buffer size: %ux%u\n", width, height);
}

static void cursor_capture_session_shm_format(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t format) {
    WlCursorClient *client = (WlCursorClient *)data;
    // Accept any ARGB/XRGB/ABGR/XBGR format (32-bit with alpha or opaque)
    // Sway may use DRM fourcc codes (like 0x34324241 for ABGR8888)
    // We accept the first usable format
    if (client->cursor_buffer_format == 0) {
        client->cursor_buffer_format = format;
    }
}

static void cursor_capture_session_done(void *data, struct ext_image_copy_capture_session_v1 *session) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->session_ready = 1;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor capture session ready\n");
}

static void cursor_capture_session_stopped(void *data, struct ext_image_copy_capture_session_v1 *session) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->stop_requested = 1;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor capture session stopped\n");
}

// Listener struct for capture session
struct ext_image_copy_capture_session_v1_listener {
    void (*buffer_size)(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t width, uint32_t height);
    void (*shm_format)(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t format);
    void (*dmabuf_device)(void *data, struct ext_image_copy_capture_session_v1 *session, struct wl_array *device);
    void (*dmabuf_format)(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t format, struct wl_array *modifiers);
    void (*done)(void *data, struct ext_image_copy_capture_session_v1 *session);
    void (*stopped)(void *data, struct ext_image_copy_capture_session_v1 *session);
};

static void cursor_capture_session_dmabuf_device(void *data, struct ext_image_copy_capture_session_v1 *session, struct wl_array *device) {}
static void cursor_capture_session_dmabuf_format(void *data, struct ext_image_copy_capture_session_v1 *session, uint32_t format, struct wl_array *modifiers) {}

static const struct ext_image_copy_capture_session_v1_listener cursor_capture_session_listener = {
    .buffer_size = cursor_capture_session_buffer_size,
    .shm_format = cursor_capture_session_shm_format,
    .dmabuf_device = cursor_capture_session_dmabuf_device,
    .dmabuf_format = cursor_capture_session_dmabuf_format,
    .done = cursor_capture_session_done,
    .stopped = cursor_capture_session_stopped,
};

// Frame events (for cursor bitmap)
static void cursor_frame_transform(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t transform) {}
static void cursor_frame_damage(void *data, struct ext_image_copy_capture_frame_v1 *frame, int32_t x, int32_t y, int32_t w, int32_t h) {}
static void cursor_frame_presentation_time(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t tv_sec_hi, uint32_t tv_sec_lo, uint32_t tv_nsec) {}

static void cursor_frame_ready(void *data, struct ext_image_copy_capture_frame_v1 *frame) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->frame_ready = 1;

    // Copy bitmap data
    if (client->shm_ptr && client->cursor_buffer_width > 0 && client->cursor_buffer_height > 0) {
        client->cursor.bitmap_width = client->cursor_buffer_width;
        client->cursor.bitmap_height = client->cursor_buffer_height;
        client->cursor.bitmap_stride = client->cursor_buffer_stride;
        client->cursor.bitmap_format = client->cursor_buffer_format;

        uint32_t size = client->cursor_buffer_stride * client->cursor_buffer_height;
        if (client->cursor.bitmap_data) {
            free(client->cursor.bitmap_data);
        }
        client->cursor.bitmap_data = (uint8_t *)malloc(size);
        if (client->cursor.bitmap_data) {
            memcpy(client->cursor.bitmap_data, client->shm_ptr, size);
            client->cursor.bitmap_size = size;
        }
    }

    // Notify callback
    if (client->callback && client->cursor.in_area) {
        client->callback(&client->cursor, client->userdata);
    }
}

static void cursor_frame_failed(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t reason) {
    WlCursorClient *client = (WlCursorClient *)data;
    client->frame_failed = 1;
    fprintf(stderr, "[WAYLAND_CURSOR] Cursor frame failed: %u\n", reason);
}

// Listener struct for frame
struct ext_image_copy_capture_frame_v1_listener {
    void (*transform)(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t transform);
    void (*damage)(void *data, struct ext_image_copy_capture_frame_v1 *frame, int32_t x, int32_t y, int32_t w, int32_t h);
    void (*presentation_time)(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t tv_sec_hi, uint32_t tv_sec_lo, uint32_t tv_nsec);
    void (*ready)(void *data, struct ext_image_copy_capture_frame_v1 *frame);
    void (*failed)(void *data, struct ext_image_copy_capture_frame_v1 *frame, uint32_t reason);
};

static const struct ext_image_copy_capture_frame_v1_listener cursor_frame_listener = {
    .transform = cursor_frame_transform,
    .damage = cursor_frame_damage,
    .presentation_time = cursor_frame_presentation_time,
    .ready = cursor_frame_ready,
    .failed = cursor_frame_failed,
};

// Seat listener for pointer
static void seat_capabilities(void *data, struct wl_seat *seat, uint32_t caps) {
    WlCursorClient *client = (WlCursorClient *)data;
    if ((caps & WL_SEAT_CAPABILITY_POINTER) && !client->pointer) {
        client->pointer = wl_seat_get_pointer(seat);
        fprintf(stderr, "[WAYLAND_CURSOR] Got pointer from seat\n");
    }
}

static void seat_name(void *data, struct wl_seat *seat, const char *name) {
    fprintf(stderr, "[WAYLAND_CURSOR] Seat name: %s\n", name);
}

static const struct wl_seat_listener seat_listener = {
    .capabilities = seat_capabilities,
    .name = seat_name,
};

// Registry listener
static void registry_global(void *data, struct wl_registry *registry, uint32_t name, const char *interface, uint32_t version) {
    WlCursorClient *client = (WlCursorClient *)data;

    if (strcmp(interface, "wl_seat") == 0) {
        client->seat = wl_registry_bind(registry, name, &wl_seat_interface, 1);
        wl_seat_add_listener(client->seat, &seat_listener, client);
        fprintf(stderr, "[WAYLAND_CURSOR] Bound wl_seat\n");
    } else if (strcmp(interface, "wl_output") == 0 && !client->output) {
        client->output = wl_registry_bind(registry, name, &wl_output_interface, 1);
        fprintf(stderr, "[WAYLAND_CURSOR] Bound wl_output\n");
    } else if (strcmp(interface, "wl_shm") == 0) {
        client->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
        fprintf(stderr, "[WAYLAND_CURSOR] Bound wl_shm\n");
    } else if (strcmp(interface, "ext_output_image_capture_source_manager_v1") == 0) {
        client->source_manager = wl_registry_bind(registry, name, &ext_output_image_capture_source_manager_v1_interface, 1);
        fprintf(stderr, "[WAYLAND_CURSOR] Bound ext_output_image_capture_source_manager_v1\n");
    } else if (strcmp(interface, "ext_image_copy_capture_manager_v1") == 0) {
        client->capture_manager = wl_registry_bind(registry, name, &ext_image_copy_capture_manager_v1_interface, 1);
        fprintf(stderr, "[WAYLAND_CURSOR] Bound ext_image_copy_capture_manager_v1\n");
    }
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t name) {}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

// Create SHM buffer for cursor capture
static int create_cursor_buffer(WlCursorClient *client) {
    if (!client->shm || client->cursor_buffer_width == 0 || client->cursor_buffer_height == 0) {
        return -1;
    }

    // Clean up old buffer
    if (client->cursor_buffer) {
        wl_buffer_destroy(client->cursor_buffer);
        client->cursor_buffer = NULL;
    }
    if (client->shm_pool) {
        wl_shm_pool_destroy(client->shm_pool);
        client->shm_pool = NULL;
    }
    if (client->shm_ptr) {
        munmap(client->shm_ptr, client->shm_size);
        client->shm_ptr = NULL;
    }
    if (client->shm_fd >= 0) {
        close(client->shm_fd);
        client->shm_fd = -1;
    }

    // Calculate stride (4 bytes per pixel for ARGB)
    client->cursor_buffer_stride = client->cursor_buffer_width * 4;
    client->shm_size = client->cursor_buffer_stride * client->cursor_buffer_height;

    // Create memfd
    client->shm_fd = memfd_create("wayland-cursor", MFD_CLOEXEC);
    if (client->shm_fd < 0) {
        fprintf(stderr, "[WAYLAND_CURSOR] memfd_create failed\n");
        return -1;
    }

    if (ftruncate(client->shm_fd, client->shm_size) < 0) {
        fprintf(stderr, "[WAYLAND_CURSOR] ftruncate failed\n");
        close(client->shm_fd);
        client->shm_fd = -1;
        return -1;
    }

    client->shm_ptr = mmap(NULL, client->shm_size, PROT_READ | PROT_WRITE, MAP_SHARED, client->shm_fd, 0);
    if (client->shm_ptr == MAP_FAILED) {
        fprintf(stderr, "[WAYLAND_CURSOR] mmap failed\n");
        close(client->shm_fd);
        client->shm_fd = -1;
        client->shm_ptr = NULL;
        return -1;
    }

    // Create wl_shm_pool and buffer
    client->shm_pool = wl_shm_create_pool(client->shm, client->shm_fd, client->shm_size);
    if (!client->shm_pool) {
        fprintf(stderr, "[WAYLAND_CURSOR] wl_shm_create_pool failed\n");
        return -1;
    }

    // For wl_shm buffer creation, we need a wl_shm format (small integer)
    // Some compositors report DRM fourcc codes which are large numbers
    // Map DRM formats to wl_shm formats for buffer creation
    uint32_t buffer_format = client->cursor_buffer_format;
    if (buffer_format > 100) {
        // Likely a DRM fourcc code, use ARGB8888 for the buffer
        // Common DRM formats: 0x34324241 (ABGR8888), 0x34324258 (XBGR8888)
        fprintf(stderr, "[WAYLAND_CURSOR] Format 0x%08x looks like DRM fourcc, using ARGB8888 for buffer\n", buffer_format);
        buffer_format = WL_SHM_FORMAT_ARGB8888;
    }
    client->cursor_buffer = wl_shm_pool_create_buffer(client->shm_pool, 0,
        client->cursor_buffer_width, client->cursor_buffer_height,
        client->cursor_buffer_stride, buffer_format);
    if (!client->cursor_buffer) {
        fprintf(stderr, "[WAYLAND_CURSOR] wl_shm_pool_create_buffer failed (format=0x%08x)\n", buffer_format);
        return -1;
    }

    fprintf(stderr, "[WAYLAND_CURSOR] Created cursor buffer %ux%u stride=%d\n",
        client->cursor_buffer_width, client->cursor_buffer_height, client->cursor_buffer_stride);
    return 0;
}

// Create client
static WlCursorClient* wl_cursor_client_new(void) {
    WlCursorClient *client = calloc(1, sizeof(WlCursorClient));
    if (!client) return NULL;

    client->shm_fd = -1;

    // Connect to Wayland display
    // Try common socket names
    const char *socket_names[] = {"wayland-1", "wayland-0", NULL};
    const char *xdg_runtime_dir = getenv("XDG_RUNTIME_DIR");
    if (!xdg_runtime_dir) xdg_runtime_dir = "/run/user/1000";

    for (int i = 0; socket_names[i]; i++) {
        char socket_path[256];
        snprintf(socket_path, sizeof(socket_path), "%s/%s", xdg_runtime_dir, socket_names[i]);
        if (access(socket_path, F_OK) == 0) {
            setenv("WAYLAND_DISPLAY", socket_names[i], 1);
            client->display = wl_display_connect(NULL);
            if (client->display) {
                fprintf(stderr, "[WAYLAND_CURSOR] Connected to %s\n", socket_names[i]);
                break;
            }
        }
    }

    if (!client->display) {
        fprintf(stderr, "[WAYLAND_CURSOR] Failed to connect to Wayland display\n");
        free(client);
        return NULL;
    }

    client->registry = wl_display_get_registry(client->display);
    wl_registry_add_listener(client->registry, &registry_listener, client);

    // First roundtrip to get globals
    wl_display_roundtrip(client->display);

    // Second roundtrip to get seat capabilities
    wl_display_roundtrip(client->display);

    // Check required globals
    if (!client->source_manager || !client->capture_manager) {
        fprintf(stderr, "[WAYLAND_CURSOR] ext-image-copy-capture not supported by compositor\n");
        if (client->registry) wl_registry_destroy(client->registry);
        wl_display_disconnect(client->display);
        free(client);
        return NULL;
    }

    if (!client->seat || !client->pointer) {
        fprintf(stderr, "[WAYLAND_CURSOR] No pointer available\n");
        if (client->registry) wl_registry_destroy(client->registry);
        wl_display_disconnect(client->display);
        free(client);
        return NULL;
    }

    if (!client->output) {
        fprintf(stderr, "[WAYLAND_CURSOR] No output available\n");
        if (client->registry) wl_registry_destroy(client->registry);
        wl_display_disconnect(client->display);
        free(client);
        return NULL;
    }

    fprintf(stderr, "[WAYLAND_CURSOR] Client initialized successfully\n");
    return client;
}

// Start cursor tracking
static int wl_cursor_client_start(WlCursorClient *client, void *userdata) {
    if (!client) return -1;

    client->callback = goWaylandCursorCallback;
    client->userdata = userdata;

    // Create output capture source
    // ext_output_image_capture_source_manager_v1.create_source(output) -> source
    static const struct wl_interface *create_source_types[] = { &ext_image_capture_source_v1_interface, &wl_output_interface };
    static const struct wl_message create_source_msg = { "create_source", "no", create_source_types };
    struct wl_proxy *source_proxy = wl_proxy_marshal_flags(
        (struct wl_proxy *)client->source_manager,
        EXT_OUTPUT_SOURCE_MANAGER_CREATE_SOURCE,
        &ext_image_capture_source_v1_interface,
        wl_proxy_get_version((struct wl_proxy *)client->source_manager),
        0,
        NULL, client->output);
    client->source = (struct ext_image_capture_source_v1 *)source_proxy;

    if (!client->source) {
        fprintf(stderr, "[WAYLAND_CURSOR] Failed to create capture source\n");
        return -1;
    }

    // Create cursor session
    // ext_image_copy_capture_manager_v1.create_pointer_cursor_session(source, pointer) -> cursor_session
    static const struct wl_interface *cursor_session_types[] = {
        &ext_image_copy_capture_cursor_session_v1_interface,
        &ext_image_capture_source_v1_interface,
        &wl_pointer_interface
    };
    struct wl_proxy *cursor_proxy = wl_proxy_marshal_flags(
        (struct wl_proxy *)client->capture_manager,
        EXT_CAPTURE_MANAGER_CREATE_POINTER_CURSOR_SESSION,
        &ext_image_copy_capture_cursor_session_v1_interface,
        wl_proxy_get_version((struct wl_proxy *)client->capture_manager),
        0,
        NULL, client->source, client->pointer);
    client->cursor_session = (struct ext_image_copy_capture_cursor_session_v1 *)cursor_proxy;

    if (!client->cursor_session) {
        fprintf(stderr, "[WAYLAND_CURSOR] Failed to create cursor session\n");
        return -1;
    }

    // Add listener for cursor events
    wl_proxy_add_listener((struct wl_proxy *)client->cursor_session,
        (void (**)(void))&cursor_session_listener, client);

    // Get capture session for cursor bitmap
    // ext_image_copy_capture_cursor_session_v1.get_capture_session() -> session
    static const struct wl_interface *get_session_types[] = { &ext_image_copy_capture_session_v1_interface };
    struct wl_proxy *session_proxy = wl_proxy_marshal_flags(
        (struct wl_proxy *)client->cursor_session,
        EXT_CURSOR_SESSION_GET_CAPTURE_SESSION,
        &ext_image_copy_capture_session_v1_interface,
        wl_proxy_get_version((struct wl_proxy *)client->cursor_session),
        0,
        NULL);
    client->cursor_capture_session = (struct ext_image_copy_capture_session_v1 *)session_proxy;

    if (!client->cursor_capture_session) {
        fprintf(stderr, "[WAYLAND_CURSOR] Failed to get cursor capture session\n");
        return -1;
    }

    // Add listener for session events
    wl_proxy_add_listener((struct wl_proxy *)client->cursor_capture_session,
        (void (**)(void))&cursor_capture_session_listener, client);

    // Roundtrip to get session constraints
    wl_display_roundtrip(client->display);

    fprintf(stderr, "[WAYLAND_CURSOR] Cursor tracking started, session_ready=%d\n", client->session_ready);
    return 0;
}

// Request a cursor frame capture
static int wl_cursor_client_capture_frame(WlCursorClient *client) {
    if (!client || !client->cursor_capture_session) return -1;

    // Create buffer if needed
    if (!client->cursor_buffer && client->cursor_buffer_width > 0) {
        if (create_cursor_buffer(client) < 0) {
            return -1;
        }
    }

    if (!client->cursor_buffer) {
        return -1;  // No buffer available yet
    }

    // Clean up old frame
    if (client->cursor_frame) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_frame,
            EXT_FRAME_DESTROY, NULL, wl_proxy_get_version((struct wl_proxy *)client->cursor_frame),
            WL_MARSHAL_FLAG_DESTROY);
        client->cursor_frame = NULL;
    }

    // Create new frame
    // ext_image_copy_capture_session_v1.create_frame() -> frame
    static const struct wl_interface *create_frame_types[] = { &ext_image_copy_capture_frame_v1_interface };
    struct wl_proxy *frame_proxy = wl_proxy_marshal_flags(
        (struct wl_proxy *)client->cursor_capture_session,
        EXT_CAPTURE_SESSION_CREATE_FRAME,
        &ext_image_copy_capture_frame_v1_interface,
        wl_proxy_get_version((struct wl_proxy *)client->cursor_capture_session),
        0,
        NULL);
    client->cursor_frame = (struct ext_image_copy_capture_frame_v1 *)frame_proxy;

    if (!client->cursor_frame) {
        fprintf(stderr, "[WAYLAND_CURSOR] Failed to create frame\n");
        return -1;
    }

    // Add listener
    wl_proxy_add_listener((struct wl_proxy *)client->cursor_frame,
        (void (**)(void))&cursor_frame_listener, client);

    client->frame_ready = 0;
    client->frame_failed = 0;

    // Attach buffer
    // ext_image_copy_capture_frame_v1.attach_buffer(buffer)
    wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_frame,
        EXT_FRAME_ATTACH_BUFFER, NULL,
        wl_proxy_get_version((struct wl_proxy *)client->cursor_frame),
        0, client->cursor_buffer);

    // Damage full buffer
    // ext_image_copy_capture_frame_v1.damage_buffer(x, y, width, height)
    wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_frame,
        EXT_FRAME_DAMAGE_BUFFER, NULL,
        wl_proxy_get_version((struct wl_proxy *)client->cursor_frame),
        0, 0, 0, (int32_t)client->cursor_buffer_width, (int32_t)client->cursor_buffer_height);

    // Capture
    // ext_image_copy_capture_frame_v1.capture()
    wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_frame,
        EXT_FRAME_CAPTURE, NULL,
        wl_proxy_get_version((struct wl_proxy *)client->cursor_frame),
        0);

    return 0;
}

// Iterate the event loop (non-blocking)
static int wl_cursor_client_iterate(WlCursorClient *client) {
    if (!client || !client->display || client->stop_requested) return -1;

    // Dispatch pending events
    wl_display_dispatch_pending(client->display);

    // Flush requests
    if (wl_display_flush(client->display) < 0) {
        return -1;
    }

    // Read events with timeout
    struct pollfd pfd = { .fd = wl_display_get_fd(client->display), .events = POLLIN };
    int ret = poll(&pfd, 1, 16);  // ~60fps polling
    if (ret > 0 && (pfd.revents & POLLIN)) {
        wl_display_dispatch(client->display);
    }

    return client->stop_requested ? -1 : 0;
}

// Stop and destroy client
static void wl_cursor_client_destroy(WlCursorClient *client) {
    if (!client) return;

    client->stop_requested = 1;

    // Free cursor bitmap data
    if (client->cursor.bitmap_data) {
        free(client->cursor.bitmap_data);
    }

    // Clean up Wayland objects
    if (client->cursor_frame) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_frame,
            EXT_FRAME_DESTROY, NULL, wl_proxy_get_version((struct wl_proxy *)client->cursor_frame),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->cursor_buffer) wl_buffer_destroy(client->cursor_buffer);
    if (client->shm_pool) wl_shm_pool_destroy(client->shm_pool);
    if (client->shm_ptr) munmap(client->shm_ptr, client->shm_size);
    if (client->shm_fd >= 0) close(client->shm_fd);

    if (client->cursor_capture_session) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_capture_session,
            EXT_CAPTURE_SESSION_DESTROY, NULL,
            wl_proxy_get_version((struct wl_proxy *)client->cursor_capture_session),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->cursor_session) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->cursor_session,
            EXT_CURSOR_SESSION_DESTROY, NULL,
            wl_proxy_get_version((struct wl_proxy *)client->cursor_session),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->source) {
        // ext_image_capture_source_v1 has destroy request at opcode 0
        wl_proxy_marshal_flags((struct wl_proxy *)client->source, 0, NULL,
            wl_proxy_get_version((struct wl_proxy *)client->source),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->pointer) wl_pointer_destroy(client->pointer);
    if (client->seat) wl_seat_destroy(client->seat);
    if (client->output) wl_output_destroy(client->output);
    if (client->shm) wl_shm_destroy(client->shm);
    if (client->source_manager) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->source_manager,
            EXT_OUTPUT_SOURCE_MANAGER_DESTROY, NULL,
            wl_proxy_get_version((struct wl_proxy *)client->source_manager),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->capture_manager) {
        wl_proxy_marshal_flags((struct wl_proxy *)client->capture_manager,
            EXT_CAPTURE_MANAGER_DESTROY, NULL,
            wl_proxy_get_version((struct wl_proxy *)client->capture_manager),
            WL_MARSHAL_FLAG_DESTROY);
    }
    if (client->registry) wl_registry_destroy(client->registry);
    if (client->display) wl_display_disconnect(client->display);

    free(client);
}

*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// WaylandCursorData represents cursor information from Wayland
type WaylandCursorData struct {
	// InArea indicates whether cursor is in the captured area
	InArea bool
	// Position relative to captured output
	PositionX int32
	PositionY int32
	// Hotspot offset within cursor image
	HotspotX int32
	HotspotY int32
	// Bitmap data (when available)
	BitmapWidth  uint32
	BitmapHeight uint32
	BitmapStride int32
	BitmapFormat uint32
	BitmapData   []byte
}

// WaylandCursorCallback is called when cursor changes
type WaylandCursorCallback func(cursor *WaylandCursorData)

// WaylandCursorClient reads cursor metadata from Wayland's ext-image-copy-capture-cursor-session-v1
type WaylandCursorClient struct {
	client   *C.WlCursorClient
	callback WaylandCursorCallback
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
}

// Global registry for callbacks (CGO can't call Go closures directly)
var (
	waylandCallbacksMu sync.RWMutex
	waylandCallbacks   = make(map[uintptr]*WaylandCursorClient)
	nextWaylandID      uintptr
)

//export goWaylandCursorCallback
func goWaylandCursorCallback(info *C.WlCursorInfo, userdata unsafe.Pointer) {
	id := uintptr(userdata)

	waylandCallbacksMu.RLock()
	client := waylandCallbacks[id]
	waylandCallbacksMu.RUnlock()

	if client == nil || client.callback == nil {
		return
	}

	// Convert C struct to Go struct
	cursor := &WaylandCursorData{
		InArea:       info.in_area != 0,
		PositionX:    int32(info.pos_x),
		PositionY:    int32(info.pos_y),
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

// NewWaylandCursorClient creates a new Wayland cursor client
func NewWaylandCursorClient() (*WaylandCursorClient, error) {
	client := C.wl_cursor_client_new()
	if client == nil {
		return nil, fmt.Errorf("failed to create Wayland cursor client (ext-image-copy-capture not supported?)")
	}

	return &WaylandCursorClient{
		client: client,
		stopCh: make(chan struct{}),
	}, nil
}

// SetCallback sets the cursor update callback
func (w *WaylandCursorClient) SetCallback(callback WaylandCursorCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callback = callback
}

// Run starts the cursor client (blocking, call from goroutine)
func (w *WaylandCursorClient) Run(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("cursor client already running")
	}
	w.running = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	// Register callback
	waylandCallbacksMu.Lock()
	nextWaylandID++
	id := nextWaylandID
	waylandCallbacks[id] = w
	waylandCallbacksMu.Unlock()

	defer func() {
		waylandCallbacksMu.Lock()
		delete(waylandCallbacks, id)
		waylandCallbacksMu.Unlock()
	}()

	// Start cursor tracking
	result := C.wl_cursor_client_start(w.client, unsafe.Pointer(id))
	if result != 0 {
		return fmt.Errorf("failed to start cursor tracking")
	}

	// Main loop
	ticker := time.NewTicker(100 * time.Millisecond) // Capture cursor bitmap periodically
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.stopCh:
			return nil
		case <-ticker.C:
			// Request cursor frame capture
			C.wl_cursor_client_capture_frame(w.client)
		default:
			// Process Wayland events
			result := C.wl_cursor_client_iterate(w.client)
			if result != 0 {
				return fmt.Errorf("wayland event loop error")
			}
		}
	}
}

// Stop stops the cursor client
func (w *WaylandCursorClient) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	select {
	case <-w.stopCh:
		// Already closed
	default:
		close(w.stopCh)
	}
}

// Close cleans up resources
func (w *WaylandCursorClient) Close() {
	w.Stop()

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client != nil {
		C.wl_cursor_client_destroy(w.client)
		w.client = nil
	}
}
