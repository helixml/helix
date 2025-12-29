/*
 * wayland-client.c - Wolf Wayland connection and surface management
 *
 * Connects to Wolf's Wayland compositor and creates a fullscreen surface
 * for displaying GNOME's screen-cast frames.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/mman.h>
#include <fcntl.h>
#include <errno.h>

#include <wayland-client.h>
#include <drm_fourcc.h>

#include "gnome-wolf-bridge.h"
#include "xdg-shell-client-protocol.h"
#include "linux-dmabuf-unstable-v1-client-protocol.h"

struct gwb_wayland {
    struct gwb_context *ctx;

    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_shm *shm;
    struct xdg_wm_base *xdg_wm_base;
    struct zwp_linux_dmabuf_v1 *dmabuf;

    struct wl_surface *surface;
    struct xdg_surface *xdg_surface;
    struct xdg_toplevel *toplevel;

    struct wl_buffer *current_buffer;
    bool configured;
    bool frame_pending;

    /* SHM buffer pool for fallback */
    struct wl_shm_pool *shm_pool;
    int shm_fd;
    void *shm_data;
    size_t shm_size;

    /* DMA-BUF format support */
    uint32_t *dmabuf_formats;
    size_t dmabuf_formats_count;
};

/* Forward declarations */
static void registry_handle_global(void *data, struct wl_registry *registry,
                                    uint32_t name, const char *interface,
                                    uint32_t version);
static void registry_handle_global_remove(void *data, struct wl_registry *registry,
                                           uint32_t name);

static const struct wl_registry_listener registry_listener = {
    .global = registry_handle_global,
    .global_remove = registry_handle_global_remove,
};

static void xdg_wm_base_ping(void *data, struct xdg_wm_base *xdg_wm_base,
                              uint32_t serial) {
    (void)data;
    xdg_wm_base_pong(xdg_wm_base, serial);
}

static const struct xdg_wm_base_listener xdg_wm_base_listener = {
    .ping = xdg_wm_base_ping,
};

static void xdg_surface_configure(void *data, struct xdg_surface *xdg_surface,
                                   uint32_t serial) {
    struct gwb_wayland *wl = data;
    xdg_surface_ack_configure(xdg_surface, serial);
    wl->configured = true;
}

static const struct xdg_surface_listener xdg_surface_listener = {
    .configure = xdg_surface_configure,
};

static void xdg_toplevel_configure(void *data, struct xdg_toplevel *toplevel,
                                    int32_t width, int32_t height,
                                    struct wl_array *states) {
    struct gwb_wayland *wl = data;
    (void)toplevel;
    (void)states;

    if (width > 0 && height > 0) {
        wl->ctx->width = width;
        wl->ctx->height = height;
        fprintf(stderr, "[wayland] Configured size: %dx%d\n", width, height);
    }
}

static void xdg_toplevel_close(void *data, struct xdg_toplevel *toplevel) {
    struct gwb_wayland *wl = data;
    (void)toplevel;
    wl->ctx->running = false;
}

static void xdg_toplevel_configure_bounds(void *data,
                                           struct xdg_toplevel *toplevel,
                                           int32_t width, int32_t height) {
    (void)data;
    (void)toplevel;
    (void)width;
    (void)height;
}

static void xdg_toplevel_wm_capabilities(void *data,
                                          struct xdg_toplevel *toplevel,
                                          struct wl_array *capabilities) {
    (void)data;
    (void)toplevel;
    (void)capabilities;
}

static const struct xdg_toplevel_listener xdg_toplevel_listener = {
    .configure = xdg_toplevel_configure,
    .close = xdg_toplevel_close,
    .configure_bounds = xdg_toplevel_configure_bounds,
    .wm_capabilities = xdg_toplevel_wm_capabilities,
};

static void dmabuf_format(void *data, struct zwp_linux_dmabuf_v1 *dmabuf,
                           uint32_t format) {
    struct gwb_wayland *wl = data;
    (void)dmabuf;

    wl->dmabuf_formats = realloc(wl->dmabuf_formats,
                                  (wl->dmabuf_formats_count + 1) * sizeof(uint32_t));
    if (wl->dmabuf_formats) {
        wl->dmabuf_formats[wl->dmabuf_formats_count++] = format;
    }
}

static void dmabuf_modifier(void *data, struct zwp_linux_dmabuf_v1 *dmabuf,
                             uint32_t format, uint32_t modifier_hi,
                             uint32_t modifier_lo) {
    (void)data;
    (void)dmabuf;
    (void)format;
    (void)modifier_hi;
    (void)modifier_lo;
    /* We track formats in the format callback */
}

static const struct zwp_linux_dmabuf_v1_listener dmabuf_listener = {
    .format = dmabuf_format,
    .modifier = dmabuf_modifier,
};

static void registry_handle_global(void *data, struct wl_registry *registry,
                                    uint32_t name, const char *interface,
                                    uint32_t version) {
    struct gwb_wayland *wl = data;

    if (strcmp(interface, wl_compositor_interface.name) == 0) {
        wl->compositor = wl_registry_bind(registry, name,
                                           &wl_compositor_interface, 4);
    } else if (strcmp(interface, wl_shm_interface.name) == 0) {
        wl->shm = wl_registry_bind(registry, name, &wl_shm_interface, 1);
    } else if (strcmp(interface, xdg_wm_base_interface.name) == 0) {
        wl->xdg_wm_base = wl_registry_bind(registry, name,
                                            &xdg_wm_base_interface, 1);
        xdg_wm_base_add_listener(wl->xdg_wm_base, &xdg_wm_base_listener, wl);
    } else if (strcmp(interface, zwp_linux_dmabuf_v1_interface.name) == 0) {
        uint32_t bind_version = version < 3 ? version : 3;
        wl->dmabuf = wl_registry_bind(registry, name,
                                       &zwp_linux_dmabuf_v1_interface,
                                       bind_version);
        zwp_linux_dmabuf_v1_add_listener(wl->dmabuf, &dmabuf_listener, wl);
    }
}

static void registry_handle_global_remove(void *data, struct wl_registry *registry,
                                           uint32_t name) {
    (void)data;
    (void)registry;
    (void)name;
}

static void frame_callback_done(void *data, struct wl_callback *callback,
                                 uint32_t time) {
    struct gwb_wayland *wl = data;
    (void)time;

    wl_callback_destroy(callback);
    wl->frame_pending = false;
}

static const struct wl_callback_listener frame_listener = {
    .done = frame_callback_done,
};

struct gwb_wayland *gwb_wayland_create(struct gwb_context *ctx,
                                        const char *display_name) {
    struct gwb_wayland *wl = calloc(1, sizeof(*wl));
    if (!wl) {
        return NULL;
    }

    wl->ctx = ctx;

    /* Connect to Wayland display */
    wl->display = wl_display_connect(display_name);
    if (!wl->display) {
        fprintf(stderr, "[wayland] Failed to connect to display: %s\n",
                display_name);
        free(wl);
        return NULL;
    }

    /* Get registry and bind globals */
    wl->registry = wl_display_get_registry(wl->display);
    wl_registry_add_listener(wl->registry, &registry_listener, wl);
    wl_display_roundtrip(wl->display);

    /* Check required globals */
    if (!wl->compositor) {
        fprintf(stderr, "[wayland] No wl_compositor found\n");
        goto error;
    }
    if (!wl->xdg_wm_base) {
        fprintf(stderr, "[wayland] No xdg_wm_base found\n");
        goto error;
    }
    if (!wl->shm) {
        fprintf(stderr, "[wayland] No wl_shm found\n");
        goto error;
    }

    /* DMA-BUF is optional but preferred */
    if (wl->dmabuf) {
        wl_display_roundtrip(wl->display);
        fprintf(stderr, "[wayland] DMA-BUF supported (%zu formats)\n",
                wl->dmabuf_formats_count);
    } else {
        fprintf(stderr, "[wayland] DMA-BUF not available, using SHM fallback\n");
    }

    /* Create surface */
    wl->surface = wl_compositor_create_surface(wl->compositor);
    if (!wl->surface) {
        fprintf(stderr, "[wayland] Failed to create surface\n");
        goto error;
    }

    /* Create xdg_surface */
    wl->xdg_surface = xdg_wm_base_get_xdg_surface(wl->xdg_wm_base, wl->surface);
    if (!wl->xdg_surface) {
        fprintf(stderr, "[wayland] Failed to create xdg_surface\n");
        goto error;
    }
    xdg_surface_add_listener(wl->xdg_surface, &xdg_surface_listener, wl);

    /* Create toplevel */
    wl->toplevel = xdg_surface_get_toplevel(wl->xdg_surface);
    if (!wl->toplevel) {
        fprintf(stderr, "[wayland] Failed to create toplevel\n");
        goto error;
    }
    xdg_toplevel_add_listener(wl->toplevel, &xdg_toplevel_listener, wl);

    /* Set window properties */
    xdg_toplevel_set_title(wl->toplevel, "GNOME Desktop");
    xdg_toplevel_set_app_id(wl->toplevel, "gnome-wolf-bridge");
    xdg_toplevel_set_fullscreen(wl->toplevel, NULL);

    /* Commit initial state */
    wl_surface_commit(wl->surface);
    wl_display_roundtrip(wl->display);

    /* Wait for configure */
    while (!wl->configured) {
        wl_display_dispatch(wl->display);
    }

    fprintf(stderr, "[wayland] Surface created and configured\n");
    return wl;

error:
    gwb_wayland_destroy(wl);
    return NULL;
}

void gwb_wayland_destroy(struct gwb_wayland *wl) {
    if (!wl) {
        return;
    }

    if (wl->shm_data) {
        munmap(wl->shm_data, wl->shm_size);
    }
    if (wl->shm_fd >= 0) {
        close(wl->shm_fd);
    }
    if (wl->shm_pool) {
        wl_shm_pool_destroy(wl->shm_pool);
    }
    if (wl->current_buffer) {
        wl_buffer_destroy(wl->current_buffer);
    }
    if (wl->toplevel) {
        xdg_toplevel_destroy(wl->toplevel);
    }
    if (wl->xdg_surface) {
        xdg_surface_destroy(wl->xdg_surface);
    }
    if (wl->surface) {
        wl_surface_destroy(wl->surface);
    }
    if (wl->dmabuf) {
        zwp_linux_dmabuf_v1_destroy(wl->dmabuf);
    }
    if (wl->xdg_wm_base) {
        xdg_wm_base_destroy(wl->xdg_wm_base);
    }
    if (wl->shm) {
        wl_shm_destroy(wl->shm);
    }
    if (wl->compositor) {
        wl_compositor_destroy(wl->compositor);
    }
    if (wl->registry) {
        wl_registry_destroy(wl->registry);
    }
    if (wl->display) {
        wl_display_disconnect(wl->display);
    }

    free(wl->dmabuf_formats);
    free(wl);
}

int gwb_wayland_get_fd(struct gwb_wayland *wl) {
    return wl_display_get_fd(wl->display);
}

int gwb_wayland_dispatch(struct gwb_wayland *wl) {
    return wl_display_dispatch_pending(wl->display);
}

int gwb_wayland_flush(struct gwb_wayland *wl) {
    return wl_display_flush(wl->display);
}

/* DMA-BUF buffer creation callback */
struct dmabuf_buffer_data {
    struct gwb_wayland *wl;
    struct wl_buffer *buffer;
    bool created;
    bool failed;
};

static void dmabuf_created(void *data,
                            struct zwp_linux_buffer_params_v1 *params,
                            struct wl_buffer *buffer) {
    struct dmabuf_buffer_data *bd = data;
    bd->buffer = buffer;
    bd->created = true;
    zwp_linux_buffer_params_v1_destroy(params);
}

static void dmabuf_failed(void *data,
                           struct zwp_linux_buffer_params_v1 *params) {
    struct dmabuf_buffer_data *bd = data;
    bd->failed = true;
    zwp_linux_buffer_params_v1_destroy(params);
}

static const struct zwp_linux_buffer_params_v1_listener dmabuf_params_listener = {
    .created = dmabuf_created,
    .failed = dmabuf_failed,
};

bool gwb_wayland_submit_dmabuf(struct gwb_wayland *wl,
                                int dmabuf_fd,
                                uint32_t width,
                                uint32_t height,
                                uint32_t stride,
                                uint32_t format,
                                uint64_t modifier) {
    if (!wl->dmabuf) {
        return false;
    }

    if (wl->frame_pending) {
        return true; /* Skip frame, previous one still pending */
    }

    /* Create DMA-BUF params */
    struct zwp_linux_buffer_params_v1 *params =
        zwp_linux_dmabuf_v1_create_params(wl->dmabuf);

    zwp_linux_buffer_params_v1_add(params, dmabuf_fd, 0, 0, stride,
                                    modifier >> 32, modifier & 0xffffffff);

    struct dmabuf_buffer_data bd = {.wl = wl};
    zwp_linux_buffer_params_v1_add_listener(params, &dmabuf_params_listener, &bd);

    zwp_linux_buffer_params_v1_create(params, width, height, format, 0);
    wl_display_roundtrip(wl->display);

    if (bd.failed || !bd.created) {
        fprintf(stderr, "[wayland] DMA-BUF buffer creation failed\n");
        return false;
    }

    /* Destroy old buffer */
    if (wl->current_buffer) {
        wl_buffer_destroy(wl->current_buffer);
    }
    wl->current_buffer = bd.buffer;

    /* Request frame callback */
    struct wl_callback *callback = wl_surface_frame(wl->surface);
    wl_callback_add_listener(callback, &frame_listener, wl);
    wl->frame_pending = true;

    /* Attach and commit */
    wl_surface_attach(wl->surface, wl->current_buffer, 0, 0);
    wl_surface_damage_buffer(wl->surface, 0, 0, width, height);
    wl_surface_commit(wl->surface);

    return true;
}

static int create_shm_file(size_t size) {
    char name[] = "/gnome-wolf-bridge-XXXXXX";
    int fd = shm_open(name, O_RDWR | O_CREAT | O_EXCL, 0600);
    if (fd < 0) {
        return -1;
    }
    shm_unlink(name);

    if (ftruncate(fd, size) < 0) {
        close(fd);
        return -1;
    }

    return fd;
}

bool gwb_wayland_submit_shm(struct gwb_wayland *wl,
                             void *data,
                             uint32_t width,
                             uint32_t height,
                             uint32_t stride,
                             uint32_t format) {
    if (wl->frame_pending) {
        return true; /* Skip frame */
    }

    size_t size = stride * height;

    /* Reallocate SHM pool if needed */
    if (!wl->shm_pool || wl->shm_size < size) {
        if (wl->shm_data) {
            munmap(wl->shm_data, wl->shm_size);
        }
        if (wl->shm_fd >= 0) {
            close(wl->shm_fd);
        }
        if (wl->shm_pool) {
            wl_shm_pool_destroy(wl->shm_pool);
        }

        wl->shm_size = size;
        wl->shm_fd = create_shm_file(size);
        if (wl->shm_fd < 0) {
            fprintf(stderr, "[wayland] Failed to create SHM file\n");
            return false;
        }

        wl->shm_data = mmap(NULL, size, PROT_READ | PROT_WRITE,
                             MAP_SHARED, wl->shm_fd, 0);
        if (wl->shm_data == MAP_FAILED) {
            close(wl->shm_fd);
            wl->shm_fd = -1;
            return false;
        }

        wl->shm_pool = wl_shm_create_pool(wl->shm, wl->shm_fd, size);
    }

    /* Copy frame data */
    memcpy(wl->shm_data, data, size);

    /* Create buffer */
    uint32_t wl_format = WL_SHM_FORMAT_ARGB8888;
    if (format == DRM_FORMAT_XRGB8888) {
        wl_format = WL_SHM_FORMAT_XRGB8888;
    } else if (format == DRM_FORMAT_ARGB8888) {
        wl_format = WL_SHM_FORMAT_ARGB8888;
    }

    if (wl->current_buffer) {
        wl_buffer_destroy(wl->current_buffer);
    }
    wl->current_buffer = wl_shm_pool_create_buffer(wl->shm_pool, 0,
                                                     width, height,
                                                     stride, wl_format);

    /* Request frame callback */
    struct wl_callback *callback = wl_surface_frame(wl->surface);
    wl_callback_add_listener(callback, &frame_listener, wl);
    wl->frame_pending = true;

    /* Attach and commit */
    wl_surface_attach(wl->surface, wl->current_buffer, 0, 0);
    wl_surface_damage_buffer(wl->surface, 0, 0, width, height);
    wl_surface_commit(wl->surface);

    return true;
}
