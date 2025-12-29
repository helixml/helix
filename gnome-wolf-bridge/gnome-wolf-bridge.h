/*
 * gnome-wolf-bridge - Bridge GNOME headless screen-cast to Wolf's Wayland
 *
 * This bridges GNOME Shell's PipeWire screen-cast to Wolf's Wayland compositor,
 * enabling zero-copy GPU frame transfer via DMA-BUF.
 */

#ifndef GNOME_WOLF_BRIDGE_H
#define GNOME_WOLF_BRIDGE_H

#include <stdbool.h>
#include <stdint.h>

/* Forward declarations */
struct gwb_context;
struct gwb_wayland;
struct gwb_screencast;
struct gwb_pipewire;

/*
 * Main context structure
 */
struct gwb_context {
    struct gwb_wayland *wayland;
    struct gwb_screencast *screencast;           /* GNOME-specific backend */
    struct gwb_portal_screencast *portal;        /* Universal portal backend */
    struct gwb_pipewire *pipewire;

    bool running;
    bool use_portal;  /* true = portal, false = GNOME direct */
    int width;
    int height;

    /* PipeWire node ID from screen-cast session */
    uint32_t pipewire_node_id;
};

/*
 * Wayland client interface
 */
struct gwb_wayland *gwb_wayland_create(struct gwb_context *ctx,
                                        const char *display_name);
void gwb_wayland_destroy(struct gwb_wayland *wl);
int gwb_wayland_get_fd(struct gwb_wayland *wl);
int gwb_wayland_dispatch(struct gwb_wayland *wl);
int gwb_wayland_flush(struct gwb_wayland *wl);

/* Submit a DMA-BUF frame to Wolf */
bool gwb_wayland_submit_dmabuf(struct gwb_wayland *wl,
                                int dmabuf_fd,
                                uint32_t width,
                                uint32_t height,
                                uint32_t stride,
                                uint32_t format,
                                uint64_t modifier);

/* Submit a SHM frame to Wolf (fallback) */
bool gwb_wayland_submit_shm(struct gwb_wayland *wl,
                             void *data,
                             uint32_t width,
                             uint32_t height,
                             uint32_t stride,
                             uint32_t format);

/*
 * Screen-cast D-Bus interface (GNOME-specific)
 */
struct gwb_screencast *gwb_screencast_create(struct gwb_context *ctx);
void gwb_screencast_destroy(struct gwb_screencast *sc);
bool gwb_screencast_start(struct gwb_screencast *sc);
void gwb_screencast_stop(struct gwb_screencast *sc);

/*
 * XDG Desktop Portal screen-cast interface (universal: GNOME, KDE, Sway)
 */
struct gwb_portal_screencast;
struct gwb_portal_screencast *gwb_portal_screencast_create(struct gwb_context *ctx);
void gwb_portal_screencast_destroy(struct gwb_portal_screencast *sc);
bool gwb_portal_screencast_start(struct gwb_portal_screencast *sc);
void gwb_portal_screencast_stop(struct gwb_portal_screencast *sc);
bool gwb_portal_screencast_available(void);

/*
 * PipeWire stream interface
 */
struct gwb_pipewire *gwb_pipewire_create(struct gwb_context *ctx);
void gwb_pipewire_destroy(struct gwb_pipewire *pw);
bool gwb_pipewire_connect(struct gwb_pipewire *pw, uint32_t node_id);
int gwb_pipewire_get_fd(struct gwb_pipewire *pw);
int gwb_pipewire_dispatch(struct gwb_pipewire *pw);

#endif /* GNOME_WOLF_BRIDGE_H */
