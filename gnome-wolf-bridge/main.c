/*
 * gnome-wolf-bridge - Main entry point
 *
 * Bridges GNOME Shell's PipeWire screen-cast to Wolf's Wayland compositor.
 * Uses DMA-BUF for zero-copy GPU frame transfer when available.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <poll.h>
#include <unistd.h>
#include <getopt.h>

#include "gnome-wolf-bridge.h"

static struct gwb_context ctx = {0};

static void signal_handler(int sig) {
    (void)sig;
    ctx.running = false;
}

static void print_usage(const char *prog) {
    fprintf(stderr, "Usage: %s [OPTIONS]\n", prog);
    fprintf(stderr, "\n");
    fprintf(stderr, "Bridge GNOME headless screen-cast to Wolf's Wayland compositor.\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Options:\n");
    fprintf(stderr, "  -d, --display DISPLAY  Wayland display to connect to (default: wayland-1)\n");
    fprintf(stderr, "  -w, --width WIDTH      Display width (default: 1920)\n");
    fprintf(stderr, "  -h, --height HEIGHT    Display height (default: 1080)\n");
    fprintf(stderr, "  --help                 Show this help message\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "Environment:\n");
    fprintf(stderr, "  WAYLAND_DISPLAY        Wayland display (alternative to -d)\n");
    fprintf(stderr, "\n");
    fprintf(stderr, "The bridge:\n");
    fprintf(stderr, "  1. Connects to Wolf's Wayland compositor\n");
    fprintf(stderr, "  2. Calls GNOME's org.gnome.Mutter.ScreenCast D-Bus API\n");
    fprintf(stderr, "  3. Receives PipeWire stream (DMA-BUF or SHM)\n");
    fprintf(stderr, "  4. Submits frames to Wolf's Wayland surface\n");
}

int main(int argc, char *argv[]) {
    const char *display_name = NULL;
    int opt;

    static struct option long_options[] = {
        {"display", required_argument, 0, 'd'},
        {"width",   required_argument, 0, 'w'},
        {"height",  required_argument, 0, 'h'},
        {"help",    no_argument,       0, 'H'},
        {0, 0, 0, 0}
    };

    /* Default values */
    ctx.width = 1920;
    ctx.height = 1080;

    while ((opt = getopt_long(argc, argv, "d:w:h:", long_options, NULL)) != -1) {
        switch (opt) {
        case 'd':
            display_name = optarg;
            break;
        case 'w':
            ctx.width = atoi(optarg);
            break;
        case 'h':
            ctx.height = atoi(optarg);
            break;
        case 'H':
            print_usage(argv[0]);
            return 0;
        default:
            print_usage(argv[0]);
            return 1;
        }
    }

    /* Use WAYLAND_DISPLAY if not specified */
    if (!display_name) {
        display_name = getenv("WAYLAND_DISPLAY");
    }
    if (!display_name) {
        display_name = "wayland-1";
    }

    fprintf(stderr, "[gnome-wolf-bridge] Starting...\n");
    fprintf(stderr, "[gnome-wolf-bridge] Display: %s, Resolution: %dx%d\n",
            display_name, ctx.width, ctx.height);

    /* Setup signal handlers */
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);

    /* Initialize Wayland connection to Wolf */
    ctx.wayland = gwb_wayland_create(&ctx, display_name);
    if (!ctx.wayland) {
        fprintf(stderr, "[gnome-wolf-bridge] Failed to connect to Wayland display: %s\n",
                display_name);
        return 1;
    }
    fprintf(stderr, "[gnome-wolf-bridge] Connected to Wayland\n");

    /*
     * Initialize screen-cast session
     * Try XDG Portal first (works on GNOME, KDE, Sway)
     * Fall back to GNOME-specific API if portal unavailable
     */
    fprintf(stderr, "[gnome-wolf-bridge] Checking for XDG Desktop Portal...\n");

    if (gwb_portal_screencast_available()) {
        fprintf(stderr, "[gnome-wolf-bridge] Using XDG Desktop Portal (universal)\n");
        ctx.use_portal = true;

        ctx.portal = gwb_portal_screencast_create(&ctx);
        if (!ctx.portal) {
            fprintf(stderr, "[gnome-wolf-bridge] Failed to create portal session\n");
            gwb_wayland_destroy(ctx.wayland);
            return 1;
        }

        if (!gwb_portal_screencast_start(ctx.portal)) {
            fprintf(stderr, "[gnome-wolf-bridge] Portal start failed, trying GNOME direct...\n");
            gwb_portal_screencast_destroy(ctx.portal);
            ctx.portal = NULL;
            ctx.use_portal = false;
        }
    }

    /* Fall back to GNOME-specific API */
    if (!ctx.use_portal) {
        fprintf(stderr, "[gnome-wolf-bridge] Using GNOME Mutter ScreenCast API\n");

        ctx.screencast = gwb_screencast_create(&ctx);
        if (!ctx.screencast) {
            fprintf(stderr, "[gnome-wolf-bridge] Failed to create screen-cast session\n");
            gwb_wayland_destroy(ctx.wayland);
            return 1;
        }

        if (!gwb_screencast_start(ctx.screencast)) {
            fprintf(stderr, "[gnome-wolf-bridge] Failed to start screen-cast\n");
            gwb_screencast_destroy(ctx.screencast);
            gwb_wayland_destroy(ctx.wayland);
            return 1;
        }
    }

    fprintf(stderr, "[gnome-wolf-bridge] Screen-cast started, PipeWire node: %u\n",
            ctx.pipewire_node_id);

    /* Initialize PipeWire and connect to stream */
    ctx.pipewire = gwb_pipewire_create(&ctx);
    if (!ctx.pipewire) {
        fprintf(stderr, "[gnome-wolf-bridge] Failed to initialize PipeWire\n");
        gwb_screencast_stop(ctx.screencast);
        gwb_screencast_destroy(ctx.screencast);
        gwb_wayland_destroy(ctx.wayland);
        return 1;
    }
    fprintf(stderr, "[gnome-wolf-bridge] PipeWire initialized\n");

    if (!gwb_pipewire_connect(ctx.pipewire, ctx.pipewire_node_id)) {
        fprintf(stderr, "[gnome-wolf-bridge] Failed to connect to PipeWire node\n");
        gwb_pipewire_destroy(ctx.pipewire);
        gwb_screencast_stop(ctx.screencast);
        gwb_screencast_destroy(ctx.screencast);
        gwb_wayland_destroy(ctx.wayland);
        return 1;
    }
    fprintf(stderr, "[gnome-wolf-bridge] Connected to PipeWire stream\n");

    /* Main event loop */
    ctx.running = true;
    fprintf(stderr, "[gnome-wolf-bridge] Entering main loop\n");

    while (ctx.running) {
        struct pollfd fds[2];
        int nfds = 0;

        /* Wayland display fd */
        int wl_fd = gwb_wayland_get_fd(ctx.wayland);
        if (wl_fd >= 0) {
            fds[nfds].fd = wl_fd;
            fds[nfds].events = POLLIN;
            nfds++;
        }

        /* PipeWire fd */
        int pw_fd = gwb_pipewire_get_fd(ctx.pipewire);
        if (pw_fd >= 0) {
            fds[nfds].fd = pw_fd;
            fds[nfds].events = POLLIN;
            nfds++;
        }

        /* Flush Wayland before polling */
        gwb_wayland_flush(ctx.wayland);

        int ret = poll(fds, nfds, 100); /* 100ms timeout */
        if (ret < 0) {
            if (ctx.running) {
                perror("[gnome-wolf-bridge] poll");
            }
            break;
        }

        /* Dispatch Wayland events */
        if (gwb_wayland_dispatch(ctx.wayland) < 0) {
            fprintf(stderr, "[gnome-wolf-bridge] Wayland dispatch error\n");
            break;
        }

        /* Dispatch PipeWire events */
        if (gwb_pipewire_dispatch(ctx.pipewire) < 0) {
            fprintf(stderr, "[gnome-wolf-bridge] PipeWire dispatch error\n");
            break;
        }
    }

    fprintf(stderr, "[gnome-wolf-bridge] Shutting down...\n");

    /* Cleanup */
    gwb_pipewire_destroy(ctx.pipewire);

    if (ctx.use_portal) {
        gwb_portal_screencast_stop(ctx.portal);
        gwb_portal_screencast_destroy(ctx.portal);
    } else {
        gwb_screencast_stop(ctx.screencast);
        gwb_screencast_destroy(ctx.screencast);
    }

    gwb_wayland_destroy(ctx.wayland);

    fprintf(stderr, "[gnome-wolf-bridge] Done\n");
    return 0;
}
