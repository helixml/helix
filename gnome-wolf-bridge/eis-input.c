/*
 * eis-input.c - Input forwarding via EIS (Emulated Input Subsystem)
 *
 * Forwards input events from Wolf's Wayland seat to GNOME headless
 * via the libei library and org.freedesktop.RemoteDesktop D-Bus interface.
 *
 * This is optional - keyboard/mouse input can also go through the standard
 * XWayland path if GNOME is running with XWayland enabled.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef HAVE_LIBEI

#include <libei.h>
#include <gio/gio.h>

#include "gnome-wolf-bridge.h"

#define REMOTE_DESKTOP_BUS_NAME "org.gnome.Mutter.RemoteDesktop"
#define REMOTE_DESKTOP_PATH "/org/gnome/Mutter/RemoteDesktop"
#define REMOTE_DESKTOP_INTERFACE "org.gnome.Mutter.RemoteDesktop"
#define RD_SESSION_INTERFACE "org.gnome.Mutter.RemoteDesktop.Session"

struct gwb_input {
    struct gwb_context *ctx;

    GDBusConnection *connection;
    char *session_path;

    struct ei *ei;
    struct ei_seat *seat;
    struct ei_device *pointer;
    struct ei_device *keyboard;
};

struct gwb_input *gwb_input_create(struct gwb_context *ctx) {
    struct gwb_input *input = calloc(1, sizeof(*input));
    if (!input) {
        return NULL;
    }

    input->ctx = ctx;

    GError *error = NULL;
    input->connection = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &error);
    if (!input->connection) {
        fprintf(stderr, "[eis] Failed to connect to session bus: %s\n",
                error->message);
        g_error_free(error);
        free(input);
        return NULL;
    }

    return input;
}

void gwb_input_destroy(struct gwb_input *input) {
    if (!input) {
        return;
    }

    if (input->keyboard) {
        ei_device_unref(input->keyboard);
    }
    if (input->pointer) {
        ei_device_unref(input->pointer);
    }
    if (input->seat) {
        ei_seat_unref(input->seat);
    }
    if (input->ei) {
        ei_unref(input->ei);
    }

    g_free(input->session_path);
    if (input->connection) {
        g_object_unref(input->connection);
    }

    free(input);
}

bool gwb_input_connect(struct gwb_input *input) {
    GError *error = NULL;
    GVariant *result;

    /*
     * Step 1: Create a RemoteDesktop session
     */
    result = g_dbus_connection_call_sync(
        input->connection,
        REMOTE_DESKTOP_BUS_NAME,
        REMOTE_DESKTOP_PATH,
        REMOTE_DESKTOP_INTERFACE,
        "CreateSession",
        NULL,
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[eis] CreateSession failed: %s\n", error->message);
        g_error_free(error);
        return false;
    }

    g_variant_get(result, "(o)", &input->session_path);
    g_variant_unref(result);
    fprintf(stderr, "[eis] RemoteDesktop session: %s\n", input->session_path);

    /*
     * Step 2: Start the session
     */
    result = g_dbus_connection_call_sync(
        input->connection,
        REMOTE_DESKTOP_BUS_NAME,
        input->session_path,
        RD_SESSION_INTERFACE,
        "Start",
        NULL,
        NULL,
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[eis] Start failed: %s\n", error->message);
        g_error_free(error);
        return false;
    }
    g_variant_unref(result);

    /*
     * Step 3: Connect to EIS
     */
    result = g_dbus_connection_call_sync(
        input->connection,
        REMOTE_DESKTOP_BUS_NAME,
        input->session_path,
        RD_SESSION_INTERFACE,
        "ConnectToEIS",
        g_variant_new("(a{sv})", NULL),
        G_VARIANT_TYPE("(h)"),
        G_DBUS_CALL_FLAGS_NONE,
        -1,
        NULL,
        &error);

    if (!result) {
        fprintf(stderr, "[eis] ConnectToEIS failed: %s\n", error->message);
        g_error_free(error);
        return false;
    }

    int fd_index;
    g_variant_get(result, "(h)", &fd_index);
    g_variant_unref(result);

    /* Get the actual fd from the message */
    GUnixFDList *fd_list = g_dbus_connection_get_fd_list(input->connection);
    if (!fd_list) {
        fprintf(stderr, "[eis] No fd list in response\n");
        return false;
    }

    int eis_fd = g_unix_fd_list_get(fd_list, fd_index, &error);
    if (eis_fd < 0) {
        fprintf(stderr, "[eis] Failed to get EIS fd: %s\n", error->message);
        g_error_free(error);
        return false;
    }

    /*
     * Step 4: Initialize libei with the fd
     */
    input->ei = ei_new_sender(NULL);
    if (!input->ei) {
        fprintf(stderr, "[eis] Failed to create EI context\n");
        close(eis_fd);
        return false;
    }

    if (ei_setup_backend_fd(input->ei, eis_fd) < 0) {
        fprintf(stderr, "[eis] Failed to setup EI backend\n");
        ei_unref(input->ei);
        input->ei = NULL;
        return false;
    }

    fprintf(stderr, "[eis] EIS connected\n");
    return true;
}

void gwb_input_send_pointer_motion(struct gwb_input *input,
                                    double dx, double dy) {
    if (!input || !input->pointer) {
        return;
    }

    ei_device_pointer_motion(input->pointer, dx, dy);
    ei_device_frame(input->pointer, ei_now(input->ei));
}

void gwb_input_send_pointer_button(struct gwb_input *input,
                                    uint32_t button, bool pressed) {
    if (!input || !input->pointer) {
        return;
    }

    ei_device_button_button(input->pointer, button, pressed);
    ei_device_frame(input->pointer, ei_now(input->ei));
}

void gwb_input_send_keyboard_key(struct gwb_input *input,
                                  uint32_t key, bool pressed) {
    if (!input || !input->keyboard) {
        return;
    }

    ei_device_keyboard_key(input->keyboard, key, pressed);
    ei_device_frame(input->keyboard, ei_now(input->ei));
}

#else /* !HAVE_LIBEI */

/* Stub implementations when libei is not available */

struct gwb_input;

struct gwb_input *gwb_input_create(struct gwb_context *ctx) {
    (void)ctx;
    fprintf(stderr, "[eis] libei not available, input forwarding disabled\n");
    return NULL;
}

void gwb_input_destroy(struct gwb_input *input) {
    (void)input;
}

bool gwb_input_connect(struct gwb_input *input) {
    (void)input;
    return false;
}

void gwb_input_send_pointer_motion(struct gwb_input *input,
                                    double dx, double dy) {
    (void)input;
    (void)dx;
    (void)dy;
}

void gwb_input_send_pointer_button(struct gwb_input *input,
                                    uint32_t button, bool pressed) {
    (void)input;
    (void)button;
    (void)pressed;
}

void gwb_input_send_keyboard_key(struct gwb_input *input,
                                  uint32_t key, bool pressed) {
    (void)input;
    (void)key;
    (void)pressed;
}

#endif /* HAVE_LIBEI */
