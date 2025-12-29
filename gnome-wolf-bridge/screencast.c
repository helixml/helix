/*
 * screencast.c - GNOME Screen-cast D-Bus session
 *
 * Uses org.gnome.Mutter.ScreenCast to create a screen-cast session
 * and obtain the PipeWire node ID for the stream.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <gio/gio.h>

#include "gnome-wolf-bridge.h"

#define SCREENCAST_BUS_NAME "org.gnome.Mutter.ScreenCast"
#define SCREENCAST_OBJECT_PATH "/org/gnome/Mutter/ScreenCast"
#define SCREENCAST_INTERFACE "org.gnome.Mutter.ScreenCast"
#define SESSION_INTERFACE "org.gnome.Mutter.ScreenCast.Session"
#define STREAM_INTERFACE "org.gnome.Mutter.ScreenCast.Stream"

struct gwb_screencast {
    struct gwb_context *ctx;

    GDBusConnection *connection;
    char *session_path;
    char *stream_path;
};

struct gwb_screencast *gwb_screencast_create(struct gwb_context *ctx) {
    struct gwb_screencast *sc = calloc(1, sizeof(*sc));
    if (!sc) {
        return NULL;
    }

    sc->ctx = ctx;

    GError *error = NULL;
    sc->connection = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &error);
    if (!sc->connection) {
        fprintf(stderr, "[screencast] Failed to connect to session bus: %s\n",
                error->message);
        g_error_free(error);
        free(sc);
        return NULL;
    }

    return sc;
}

void gwb_screencast_destroy(struct gwb_screencast *sc) {
    if (!sc) {
        return;
    }

    g_free(sc->session_path);
    g_free(sc->stream_path);

    if (sc->connection) {
        g_object_unref(sc->connection);
    }

    free(sc);
}

bool gwb_screencast_start(struct gwb_screencast *sc) {
    GError *error = NULL;
    GVariant *result;

    /*
     * Step 1: Create a screen-cast session
     * Call: org.gnome.Mutter.ScreenCast.CreateSession(properties)
     * Returns: session object path
     */
    GVariantBuilder props_builder;
    g_variant_builder_init(&props_builder, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&props_builder, "{sv}",
                          "remote-desktop-session-id",
                          g_variant_new_string(""));

    result = g_dbus_connection_call_sync(
        sc->connection,
        SCREENCAST_BUS_NAME,
        SCREENCAST_OBJECT_PATH,
        SCREENCAST_INTERFACE,
        "CreateSession",
        g_variant_new("(a{sv})", &props_builder),
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[screencast] CreateSession failed: %s\n",
                error->message);
        g_error_free(error);
        return false;
    }

    g_variant_get(result, "(o)", &sc->session_path);
    g_variant_unref(result);
    fprintf(stderr, "[screencast] Session created: %s\n", sc->session_path);

    /*
     * Step 2: Record the virtual display
     * Call: Session.RecordVirtual(properties)
     * Returns: stream object path
     */
    GVariantBuilder stream_props;
    g_variant_builder_init(&stream_props, G_VARIANT_TYPE("a{sv}"));

    /* Request specific cursor mode */
    g_variant_builder_add(&stream_props, "{sv}",
                          "cursor-mode",
                          g_variant_new_uint32(2)); /* 2 = embedded */

    /* For headless GNOME, we need to record the primary monitor */
    /* If GNOME isn't headless, RecordMonitor would be used instead */

    result = g_dbus_connection_call_sync(
        sc->connection,
        SCREENCAST_BUS_NAME,
        sc->session_path,
        SESSION_INTERFACE,
        "RecordVirtual",
        g_variant_new("(a{sv})", &stream_props),
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        /* Try RecordMonitor as fallback (for non-headless GNOME) */
        fprintf(stderr, "[screencast] RecordVirtual failed, trying RecordMonitor...\n");
        g_error_free(error);
        error = NULL;

        g_variant_builder_init(&stream_props, G_VARIANT_TYPE("a{sv}"));
        g_variant_builder_add(&stream_props, "{sv}",
                              "cursor-mode",
                              g_variant_new_uint32(2));

        result = g_dbus_connection_call_sync(
            sc->connection,
            SCREENCAST_BUS_NAME,
            sc->session_path,
            SESSION_INTERFACE,
            "RecordMonitor",
            g_variant_new("(sa{sv})", "", &stream_props),
            G_VARIANT_TYPE("(o)"),
            G_DBUS_CALL_FLAGS_NONE,
            -1,
            NULL,
            &error);

        if (!result) {
            fprintf(stderr, "[screencast] RecordMonitor failed: %s\n",
                    error->message);
            g_error_free(error);
            return false;
        }
    }

    g_variant_get(result, "(o)", &sc->stream_path);
    g_variant_unref(result);
    fprintf(stderr, "[screencast] Stream created: %s\n", sc->stream_path);

    /*
     * Step 3: Start the session
     * Call: Session.Start()
     */
    result = g_dbus_connection_call_sync(
        sc->connection,
        SCREENCAST_BUS_NAME,
        sc->session_path,
        SESSION_INTERFACE,
        "Start",
        NULL,
        NULL,
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[screencast] Start failed: %s\n", error->message);
        g_error_free(error);
        return false;
    }
    g_variant_unref(result);
    fprintf(stderr, "[screencast] Session started\n");

    /*
     * Step 4: Get PipeWire node ID from stream properties
     * Read: Stream.PipeWireStreamNodeId property
     */
    result = g_dbus_connection_call_sync(
        sc->connection,
        SCREENCAST_BUS_NAME,
        sc->stream_path,
        "org.freedesktop.DBus.Properties",
        "Get",
        g_variant_new("(ss)", STREAM_INTERFACE, "PipeWireStreamNodeId"),
        G_VARIANT_TYPE("(v)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[screencast] Failed to get PipeWireStreamNodeId: %s\n",
                error->message);
        g_error_free(error);
        return false;
    }

    GVariant *inner;
    g_variant_get(result, "(v)", &inner);
    sc->ctx->pipewire_node_id = g_variant_get_uint32(inner);
    g_variant_unref(inner);
    g_variant_unref(result);

    fprintf(stderr, "[screencast] PipeWire node ID: %u\n",
            sc->ctx->pipewire_node_id);

    return true;
}

void gwb_screencast_stop(struct gwb_screencast *sc) {
    if (!sc || !sc->session_path) {
        return;
    }

    GError *error = NULL;
    GVariant *result = g_dbus_connection_call_sync(
        sc->connection,
        SCREENCAST_BUS_NAME,
        sc->session_path,
        SESSION_INTERFACE,
        "Stop",
        NULL,
        NULL,
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[screencast] Stop failed: %s\n", error->message);
        g_error_free(error);
        return;
    }
    g_variant_unref(result);

    fprintf(stderr, "[screencast] Session stopped\n");
}
