/*
 * pipewire-stream.c - PipeWire stream consumer
 *
 * Connects to GNOME's screen-cast PipeWire stream and forwards frames
 * to Wolf's Wayland surface. Supports both DMA-BUF (zero-copy) and
 * SHM (fallback) buffer types.
 *
 * Based on Mutter's MDK (mdk-stream.c) and wlroots' screencopy.
 */

/* Required for locale_t and other POSIX extensions used by PipeWire */
#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>

#include <pipewire/pipewire.h>
#include <spa/param/video/format-utils.h>
#include <spa/debug/types.h>
#include <spa/utils/result.h>

#include <drm_fourcc.h>

#include "gnome-wolf-bridge.h"

struct gwb_pipewire {
    struct gwb_context *ctx;

    struct pw_thread_loop *loop;
    struct pw_context *context;
    struct pw_core *core;
    struct spa_hook core_listener;

    struct pw_stream *stream;
    struct spa_hook stream_listener;

    uint32_t node_id;
    bool connected;

    /* Stream format info */
    uint32_t format;
    uint32_t width;
    uint32_t height;
    uint32_t stride;
    uint64_t modifier;

    /* Statistics */
    uint64_t frames_received;
    uint64_t frames_dmabuf;
    uint64_t frames_shm;
};

static uint32_t spa_to_drm_format(enum spa_video_format spa_format) {
    switch (spa_format) {
    case SPA_VIDEO_FORMAT_BGRA:
    case SPA_VIDEO_FORMAT_BGRx:
        return DRM_FORMAT_ARGB8888;
    case SPA_VIDEO_FORMAT_RGBA:
    case SPA_VIDEO_FORMAT_RGBx:
        return DRM_FORMAT_ABGR8888;
    case SPA_VIDEO_FORMAT_ARGB:
    case SPA_VIDEO_FORMAT_xRGB:
        return DRM_FORMAT_BGRA8888;
    case SPA_VIDEO_FORMAT_ABGR:
    case SPA_VIDEO_FORMAT_xBGR:
        return DRM_FORMAT_RGBA8888;
    case SPA_VIDEO_FORMAT_RGB:
        return DRM_FORMAT_RGB888;
    case SPA_VIDEO_FORMAT_BGR:
        return DRM_FORMAT_BGR888;
    default:
        fprintf(stderr, "[pipewire] Unknown SPA format: %d\n", spa_format);
        return DRM_FORMAT_XRGB8888;
    }
}

static void on_stream_state_changed(void *data, enum pw_stream_state old,
                                     enum pw_stream_state state,
                                     const char *error) {
    struct gwb_pipewire *pw = data;
    (void)old;

    fprintf(stderr, "[pipewire] Stream state: %s",
            pw_stream_state_as_string(state));
    if (error) {
        fprintf(stderr, " (error: %s)", error);
    }
    fprintf(stderr, "\n");

    switch (state) {
    case PW_STREAM_STATE_ERROR:
        pw->ctx->running = false;
        break;
    case PW_STREAM_STATE_STREAMING:
        pw->connected = true;
        break;
    default:
        break;
    }
}

static void on_stream_param_changed(void *data, uint32_t id,
                                     const struct spa_pod *param) {
    struct gwb_pipewire *pw = data;

    if (param == NULL || id != SPA_PARAM_Format) {
        return;
    }

    struct spa_video_info format;
    if (spa_format_parse(param, &format.media_type, &format.media_subtype) < 0) {
        return;
    }

    if (format.media_type != SPA_MEDIA_TYPE_video ||
        format.media_subtype != SPA_MEDIA_SUBTYPE_raw) {
        return;
    }

    if (spa_format_video_raw_parse(param, &format.info.raw) < 0) {
        return;
    }

    pw->width = format.info.raw.size.width;
    pw->height = format.info.raw.size.height;
    pw->format = spa_to_drm_format(format.info.raw.format);

    /* Try to get modifier if available */
    pw->modifier = DRM_FORMAT_MOD_INVALID;
    if (format.info.raw.modifier != 0) {
        pw->modifier = format.info.raw.modifier;
    }

    fprintf(stderr, "[pipewire] Stream format: %ux%u, format=%u, modifier=0x%lx\n",
            pw->width, pw->height, pw->format, (unsigned long)pw->modifier);

    /* Calculate stride (assume 4 bytes per pixel for ARGB/XRGB) */
    pw->stride = pw->width * 4;

    /* Update buffers with negotiated format */
    uint8_t buffer[1024];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));

    const struct spa_pod *params[1];
    params[0] = spa_pod_builder_add_object(&b,
        SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
        SPA_PARAM_BUFFERS_buffers, SPA_POD_CHOICE_RANGE_Int(4, 2, 8),
        SPA_PARAM_BUFFERS_blocks, SPA_POD_Int(1),
        SPA_PARAM_BUFFERS_size, SPA_POD_Int(pw->stride * pw->height),
        SPA_PARAM_BUFFERS_stride, SPA_POD_Int(pw->stride),
        SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int(
            (1 << SPA_DATA_DmaBuf) | (1 << SPA_DATA_MemPtr)));

    pw_stream_update_params(pw->stream, params, 1);
}

static void on_stream_process(void *data) {
    struct gwb_pipewire *pw = data;

    struct pw_buffer *b = pw_stream_dequeue_buffer(pw->stream);
    if (!b) {
        return;
    }

    struct spa_buffer *buffer = b->buffer;
    pw->frames_received++;

    if (buffer->n_datas == 0 || buffer->datas[0].data == NULL) {
        /* Check for DMA-BUF */
        if (buffer->datas[0].type == SPA_DATA_DmaBuf) {
            int fd = buffer->datas[0].fd;
            uint32_t stride = buffer->datas[0].chunk->stride;

            if (stride == 0) {
                stride = pw->stride;
            }

            pw->frames_dmabuf++;

            bool ok = gwb_wayland_submit_dmabuf(pw->ctx->wayland,
                                                 fd,
                                                 pw->width,
                                                 pw->height,
                                                 stride,
                                                 pw->format,
                                                 pw->modifier);
            if (!ok && (pw->frames_dmabuf % 100) == 1) {
                fprintf(stderr, "[pipewire] DMA-BUF submit failed\n");
            }
        }
        goto done;
    }

    /* SHM/MemPtr buffer */
    void *frame_data = SPA_PTROFF(buffer->datas[0].data,
                                   buffer->datas[0].chunk->offset,
                                   void);
    uint32_t stride = buffer->datas[0].chunk->stride;
    if (stride == 0) {
        stride = pw->stride;
    }

    pw->frames_shm++;

    bool ok = gwb_wayland_submit_shm(pw->ctx->wayland,
                                      frame_data,
                                      pw->width,
                                      pw->height,
                                      stride,
                                      pw->format);
    if (!ok && (pw->frames_shm % 100) == 1) {
        fprintf(stderr, "[pipewire] SHM submit failed\n");
    }

done:
    pw_stream_queue_buffer(pw->stream, b);

    /* Print stats every 300 frames (~5 seconds at 60fps) */
    if ((pw->frames_received % 300) == 0) {
        fprintf(stderr, "[pipewire] Frames: %lu total, %lu DMA-BUF, %lu SHM\n",
                (unsigned long)pw->frames_received,
                (unsigned long)pw->frames_dmabuf,
                (unsigned long)pw->frames_shm);
    }
}

static const struct pw_stream_events stream_events = {
    PW_VERSION_STREAM_EVENTS,
    .state_changed = on_stream_state_changed,
    .param_changed = on_stream_param_changed,
    .process = on_stream_process,
};

static void on_core_error(void *data, uint32_t id, int seq, int res,
                           const char *message) {
    struct gwb_pipewire *pw = data;
    (void)seq;

    fprintf(stderr, "[pipewire] Core error: id=%u, res=%d (%s): %s\n",
            id, res, spa_strerror(res), message);

    if (id == PW_ID_CORE && res == -EPIPE) {
        pw->ctx->running = false;
    }
}

static const struct pw_core_events core_events = {
    PW_VERSION_CORE_EVENTS,
    .error = on_core_error,
};

struct gwb_pipewire *gwb_pipewire_create(struct gwb_context *ctx) {
    struct gwb_pipewire *pw = calloc(1, sizeof(*pw));
    if (!pw) {
        return NULL;
    }

    pw->ctx = ctx;

    /* Initialize PipeWire */
    pw_init(NULL, NULL);

    /* Create thread loop */
    pw->loop = pw_thread_loop_new("gnome-wolf-bridge", NULL);
    if (!pw->loop) {
        fprintf(stderr, "[pipewire] Failed to create thread loop\n");
        free(pw);
        return NULL;
    }

    /* Create context */
    pw->context = pw_context_new(pw_thread_loop_get_loop(pw->loop), NULL, 0);
    if (!pw->context) {
        fprintf(stderr, "[pipewire] Failed to create context\n");
        pw_thread_loop_destroy(pw->loop);
        free(pw);
        return NULL;
    }

    /* Start the loop */
    if (pw_thread_loop_start(pw->loop) < 0) {
        fprintf(stderr, "[pipewire] Failed to start thread loop\n");
        pw_context_destroy(pw->context);
        pw_thread_loop_destroy(pw->loop);
        free(pw);
        return NULL;
    }

    pw_thread_loop_lock(pw->loop);

    /* Connect to PipeWire */
    pw->core = pw_context_connect(pw->context, NULL, 0);
    if (!pw->core) {
        fprintf(stderr, "[pipewire] Failed to connect to PipeWire\n");
        pw_thread_loop_unlock(pw->loop);
        pw_thread_loop_stop(pw->loop);
        pw_context_destroy(pw->context);
        pw_thread_loop_destroy(pw->loop);
        free(pw);
        return NULL;
    }

    pw_core_add_listener(pw->core, &pw->core_listener, &core_events, pw);

    pw_thread_loop_unlock(pw->loop);

    return pw;
}

void gwb_pipewire_destroy(struct gwb_pipewire *pw) {
    if (!pw) {
        return;
    }

    pw_thread_loop_lock(pw->loop);

    if (pw->stream) {
        pw_stream_destroy(pw->stream);
    }
    if (pw->core) {
        pw_core_disconnect(pw->core);
    }

    pw_thread_loop_unlock(pw->loop);

    pw_thread_loop_stop(pw->loop);
    pw_context_destroy(pw->context);
    pw_thread_loop_destroy(pw->loop);

    free(pw);
}

bool gwb_pipewire_connect(struct gwb_pipewire *pw, uint32_t node_id) {
    pw->node_id = node_id;

    pw_thread_loop_lock(pw->loop);

    /* Create stream */
    struct pw_properties *props = pw_properties_new(
        PW_KEY_MEDIA_TYPE, "Video",
        PW_KEY_MEDIA_CATEGORY, "Capture",
        PW_KEY_MEDIA_ROLE, "Screen",
        NULL);

    pw->stream = pw_stream_new(pw->core, "gnome-wolf-bridge", props);
    if (!pw->stream) {
        fprintf(stderr, "[pipewire] Failed to create stream\n");
        pw_thread_loop_unlock(pw->loop);
        return false;
    }

    pw_stream_add_listener(pw->stream, &pw->stream_listener,
                            &stream_events, pw);

    /* Build format params - prefer DMA-BUF but accept SHM */
    uint8_t buffer[1024];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));

    const struct spa_pod *params[1];
    params[0] = spa_pod_builder_add_object(&b,
        SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
        SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_video),
        SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
        SPA_FORMAT_VIDEO_format, SPA_POD_CHOICE_ENUM_Id(5,
            SPA_VIDEO_FORMAT_BGRx,
            SPA_VIDEO_FORMAT_BGRA,
            SPA_VIDEO_FORMAT_RGBx,
            SPA_VIDEO_FORMAT_RGBA,
            SPA_VIDEO_FORMAT_xRGB),
        SPA_FORMAT_VIDEO_size, SPA_POD_CHOICE_RANGE_Rectangle(
            &SPA_RECTANGLE(pw->ctx->width, pw->ctx->height),
            &SPA_RECTANGLE(1, 1),
            &SPA_RECTANGLE(8192, 8192)),
        SPA_FORMAT_VIDEO_framerate, SPA_POD_CHOICE_RANGE_Fraction(
            &SPA_FRACTION(60, 1),
            &SPA_FRACTION(1, 1),
            &SPA_FRACTION(120, 1)));

    /* Connect to the screen-cast stream */
    char target[64];
    snprintf(target, sizeof(target), "%u", node_id);

    int res = pw_stream_connect(pw->stream,
                                 PW_DIRECTION_INPUT,
                                 node_id,
                                 PW_STREAM_FLAG_AUTOCONNECT |
                                 PW_STREAM_FLAG_MAP_BUFFERS,
                                 params, 1);
    if (res < 0) {
        fprintf(stderr, "[pipewire] Failed to connect stream: %s\n",
                spa_strerror(res));
        pw_stream_destroy(pw->stream);
        pw->stream = NULL;
        pw_thread_loop_unlock(pw->loop);
        return false;
    }

    pw_thread_loop_unlock(pw->loop);

    fprintf(stderr, "[pipewire] Connecting to node %u...\n", node_id);
    return true;
}

int gwb_pipewire_get_fd(struct gwb_pipewire *pw) {
    /* PipeWire uses its own thread, so we don't poll its fd directly */
    (void)pw;
    return -1;
}

int gwb_pipewire_dispatch(struct gwb_pipewire *pw) {
    /* PipeWire runs in its own thread, events are handled there */
    (void)pw;
    return 0;
}
