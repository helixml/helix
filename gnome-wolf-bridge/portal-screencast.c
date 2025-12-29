/*
 * portal-screencast.c - XDG Desktop Portal screen-cast session
 *
 * Uses org.freedesktop.portal.ScreenCast to create a screen-cast session.
 * This works across GNOME, KDE, and Sway (any desktop with portal support).
 *
 * Portal API flow:
 * 1. CreateSession() -> Request path -> Response signal -> session handle
 * 2. SelectSources() -> Request path -> Response signal
 * 3. Start() -> Request path -> Response signal -> PipeWire node ID
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <gio/gio.h>

#include "gnome-wolf-bridge.h"

#define PORTAL_BUS_NAME "org.freedesktop.portal.Desktop"
#define PORTAL_OBJECT_PATH "/org/freedesktop/portal/desktop"
#define PORTAL_SCREENCAST_INTERFACE "org.freedesktop.portal.ScreenCast"
#define PORTAL_REQUEST_INTERFACE "org.freedesktop.portal.Request"

struct gwb_portal_screencast {
    struct gwb_context *ctx;

    GDBusConnection *connection;
    char *session_handle;
    char *sender_name;  /* Our D-Bus sender name, munged for request path */

    /* For async response handling */
    guint response_signal_id;
    GMainLoop *wait_loop;
    GVariant *response_result;
    uint32_t response_code;
};

/* Munge sender name for request path: :1.234 -> 1_234 */
static char *munge_sender_name(const char *sender) {
    if (!sender || sender[0] != ':') {
        return g_strdup("unknown");
    }

    char *munged = g_strdup(sender + 1);  /* Skip the ':' */
    for (char *p = munged; *p; p++) {
        if (*p == '.') {
            *p = '_';
        }
    }
    return munged;
}

/* Generate unique request token */
static char *generate_token(void) {
    static int counter = 0;
    return g_strdup_printf("wolf_bridge_%d_%d", getpid(), counter++);
}

/* Signal handler for portal Response */
static void on_portal_response(GDBusConnection *connection,
                                const char *sender_name,
                                const char *object_path,
                                const char *interface_name,
                                const char *signal_name,
                                GVariant *parameters,
                                gpointer user_data) {
    struct gwb_portal_screencast *sc = user_data;
    (void)connection;
    (void)sender_name;
    (void)object_path;
    (void)interface_name;
    (void)signal_name;

    g_variant_get(parameters, "(u@a{sv})", &sc->response_code, &sc->response_result);

    if (sc->wait_loop) {
        g_main_loop_quit(sc->wait_loop);
    }
}

/* Subscribe to Response signal and wait for it */
static bool wait_for_response(struct gwb_portal_screencast *sc,
                               const char *request_path) {
    sc->response_code = UINT32_MAX;
    sc->response_result = NULL;

    /* Subscribe to Response signal */
    sc->response_signal_id = g_dbus_connection_signal_subscribe(
        sc->connection,
        PORTAL_BUS_NAME,
        PORTAL_REQUEST_INTERFACE,
        "Response",
        request_path,
        NULL,
        G_DBUS_SIGNAL_FLAGS_NO_MATCH_RULE,
        on_portal_response,
        sc,
        NULL);

    /* Run loop until we get the response */
    sc->wait_loop = g_main_loop_new(NULL, FALSE);

    /* Add timeout to prevent hanging forever */
    GSource *timeout_source = g_timeout_source_new_seconds(30);
    g_source_set_callback(timeout_source, (GSourceFunc)g_main_loop_quit,
                          sc->wait_loop, NULL);
    g_source_attach(timeout_source, NULL);

    g_main_loop_run(sc->wait_loop);

    g_source_destroy(timeout_source);
    g_source_unref(timeout_source);

    g_main_loop_unref(sc->wait_loop);
    sc->wait_loop = NULL;

    g_dbus_connection_signal_unsubscribe(sc->connection, sc->response_signal_id);

    if (sc->response_code == UINT32_MAX) {
        fprintf(stderr, "[portal] Response timeout\n");
        return false;
    }

    return sc->response_code == 0;  /* 0 = success */
}

struct gwb_portal_screencast *gwb_portal_screencast_create(struct gwb_context *ctx) {
    struct gwb_portal_screencast *sc = calloc(1, sizeof(*sc));
    if (!sc) {
        return NULL;
    }

    sc->ctx = ctx;

    GError *error = NULL;
    sc->connection = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &error);
    if (!sc->connection) {
        fprintf(stderr, "[portal] Failed to connect to session bus: %s\n",
                error->message);
        g_error_free(error);
        free(sc);
        return NULL;
    }

    /* Get our sender name for constructing request paths */
    const char *sender = g_dbus_connection_get_unique_name(sc->connection);
    sc->sender_name = munge_sender_name(sender);

    return sc;
}

void gwb_portal_screencast_destroy(struct gwb_portal_screencast *sc) {
    if (!sc) {
        return;
    }

    if (sc->response_result) {
        g_variant_unref(sc->response_result);
    }
    g_free(sc->session_handle);
    g_free(sc->sender_name);

    if (sc->connection) {
        g_object_unref(sc->connection);
    }

    free(sc);
}

bool gwb_portal_screencast_start(struct gwb_portal_screencast *sc) {
    GError *error = NULL;
    GVariant *result;
    char *token;
    char *request_path;

    /*
     * Step 1: CreateSession
     */
    token = generate_token();
    request_path = g_strdup_printf("/org/freedesktop/portal/desktop/request/%s/%s",
                                    sc->sender_name, token);

    GVariantBuilder options;
    g_variant_builder_init(&options, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&options, "{sv}", "handle_token", g_variant_new_string(token));
    g_variant_builder_add(&options, "{sv}", "session_handle_token",
                          g_variant_new_string("wolf_session"));

    result = g_dbus_connection_call_sync(
        sc->connection,
        PORTAL_BUS_NAME,
        PORTAL_OBJECT_PATH,
        PORTAL_SCREENCAST_INTERFACE,
        "CreateSession",
        g_variant_new("(a{sv})", &options),
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    g_free(token);

    if (!result) {
        fprintf(stderr, "[portal] CreateSession call failed: %s\n", error->message);
        g_error_free(error);
        g_free(request_path);
        return false;
    }
    g_variant_unref(result);

    fprintf(stderr, "[portal] Waiting for CreateSession response...\n");
    if (!wait_for_response(sc, request_path)) {
        fprintf(stderr, "[portal] CreateSession failed (response code: %u)\n",
                sc->response_code);
        g_free(request_path);
        return false;
    }
    g_free(request_path);

    /* Extract session handle from response */
    GVariant *session_handle_v = g_variant_lookup_value(sc->response_result,
                                                         "session_handle",
                                                         G_VARIANT_TYPE_STRING);
    if (!session_handle_v) {
        fprintf(stderr, "[portal] No session_handle in response\n");
        return false;
    }
    sc->session_handle = g_variant_dup_string(session_handle_v, NULL);
    g_variant_unref(session_handle_v);
    g_variant_unref(sc->response_result);
    sc->response_result = NULL;

    fprintf(stderr, "[portal] Session created: %s\n", sc->session_handle);

    /*
     * Step 2: SelectSources
     */
    token = generate_token();
    request_path = g_strdup_printf("/org/freedesktop/portal/desktop/request/%s/%s",
                                    sc->sender_name, token);

    g_variant_builder_init(&options, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&options, "{sv}", "handle_token", g_variant_new_string(token));
    /* types: 1=monitor, 2=window, 4=virtual */
    g_variant_builder_add(&options, "{sv}", "types", g_variant_new_uint32(1 | 4));
    /* cursor_mode: 1=hidden, 2=embedded, 4=metadata */
    g_variant_builder_add(&options, "{sv}", "cursor_mode", g_variant_new_uint32(2));
    /* multiple: allow selecting multiple sources */
    g_variant_builder_add(&options, "{sv}", "multiple", g_variant_new_boolean(FALSE));

    result = g_dbus_connection_call_sync(
        sc->connection,
        PORTAL_BUS_NAME,
        PORTAL_OBJECT_PATH,
        PORTAL_SCREENCAST_INTERFACE,
        "SelectSources",
        g_variant_new("(oa{sv})", sc->session_handle, &options),
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    g_free(token);

    if (!result) {
        fprintf(stderr, "[portal] SelectSources call failed: %s\n", error->message);
        g_error_free(error);
        g_free(request_path);
        return false;
    }
    g_variant_unref(result);

    fprintf(stderr, "[portal] Waiting for SelectSources response...\n");
    if (!wait_for_response(sc, request_path)) {
        fprintf(stderr, "[portal] SelectSources failed (response code: %u)\n",
                sc->response_code);
        g_free(request_path);
        return false;
    }
    g_free(request_path);

    if (sc->response_result) {
        g_variant_unref(sc->response_result);
        sc->response_result = NULL;
    }

    fprintf(stderr, "[portal] Sources selected\n");

    /*
     * Step 3: Start (returns PipeWire node ID)
     */
    token = generate_token();
    request_path = g_strdup_printf("/org/freedesktop/portal/desktop/request/%s/%s",
                                    sc->sender_name, token);

    g_variant_builder_init(&options, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&options, "{sv}", "handle_token", g_variant_new_string(token));

    result = g_dbus_connection_call_sync(
        sc->connection,
        PORTAL_BUS_NAME,
        PORTAL_OBJECT_PATH,
        PORTAL_SCREENCAST_INTERFACE,
        "Start",
        g_variant_new("(osa{sv})", sc->session_handle, "", &options),
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    g_free(token);

    if (!result) {
        fprintf(stderr, "[portal] Start call failed: %s\n", error->message);
        g_error_free(error);
        g_free(request_path);
        return false;
    }
    g_variant_unref(result);

    fprintf(stderr, "[portal] Waiting for Start response...\n");
    if (!wait_for_response(sc, request_path)) {
        fprintf(stderr, "[portal] Start failed (response code: %u)\n",
                sc->response_code);
        g_free(request_path);
        return false;
    }
    g_free(request_path);

    /*
     * Extract PipeWire node ID from streams array in response
     * Format: streams = [(node_id, {properties}), ...]
     */
    GVariant *streams_v = g_variant_lookup_value(sc->response_result, "streams",
                                                  G_VARIANT_TYPE("a(ua{sv})"));
    if (!streams_v) {
        fprintf(stderr, "[portal] No streams in response\n");
        g_variant_unref(sc->response_result);
        return false;
    }

    gsize n_streams = g_variant_n_children(streams_v);
    if (n_streams == 0) {
        fprintf(stderr, "[portal] Empty streams array\n");
        g_variant_unref(streams_v);
        g_variant_unref(sc->response_result);
        return false;
    }

    /* Get first stream's node ID */
    GVariant *first_stream = g_variant_get_child_value(streams_v, 0);
    GVariant *node_id_v = g_variant_get_child_value(first_stream, 0);
    sc->ctx->pipewire_node_id = g_variant_get_uint32(node_id_v);

    g_variant_unref(node_id_v);
    g_variant_unref(first_stream);
    g_variant_unref(streams_v);
    g_variant_unref(sc->response_result);
    sc->response_result = NULL;

    fprintf(stderr, "[portal] PipeWire node ID: %u\n", sc->ctx->pipewire_node_id);

    return true;
}

void gwb_portal_screencast_stop(struct gwb_portal_screencast *sc) {
    if (!sc || !sc->session_handle) {
        return;
    }

    /* Close the session by calling Close on the session object */
    GError *error = NULL;
    GVariant *result = g_dbus_connection_call_sync(
        sc->connection,
        PORTAL_BUS_NAME,
        sc->session_handle,
        "org.freedesktop.portal.Session",
        "Close",
        NULL,
        NULL,
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        /* Session might already be closed, that's OK */
        g_error_free(error);
        return;
    }
    g_variant_unref(result);

    fprintf(stderr, "[portal] Session closed\n");
}

/* Check if portal is available */
bool gwb_portal_screencast_available(void) {
    GError *error = NULL;
    GDBusConnection *connection = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &error);
    if (!connection) {
        g_error_free(error);
        return false;
    }

    /* Try to get AvailableSourceTypes property to verify portal exists */
    GVariant *result = g_dbus_connection_call_sync(
        connection,
        PORTAL_BUS_NAME,
        PORTAL_OBJECT_PATH,
        "org.freedesktop.DBus.Properties",
        "Get",
        g_variant_new("(ss)", PORTAL_SCREENCAST_INTERFACE, "AvailableSourceTypes"),
        G_VARIANT_TYPE("(v)"),
        G_DBUS_CALL_FLAGS_NONE,
        1000,  /* 1 second timeout */
        NULL,
        &error);

    g_object_unref(connection);

    if (!result) {
        g_error_free(error);
        return false;
    }

    g_variant_unref(result);
    return true;
}
