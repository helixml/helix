/*
 * GStreamer vsockenc - Video encoder that delegates to host via vsock
 */

#ifdef HAVE_CONFIG_H
#include "config.h"
#endif

#include "gstvsockenc.h"
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <linux/vm_sockets.h>
#include <libdrm/drm.h>
#include <xf86drm.h>
#include <sys/ioctl.h>
#include <fcntl.h>

GST_DEBUG_CATEGORY_STATIC(gst_vsockenc_debug);
#define GST_CAT_DEFAULT gst_vsockenc_debug

/* Properties */
enum {
    PROP_0,
    PROP_SOCKET_PATH,
    PROP_CID,
    PROP_PORT,
    PROP_BITRATE,
    PROP_KEYFRAME_INTERVAL,
};

#define DEFAULT_SOCKET_PATH NULL
#define DEFAULT_CID 2  /* VMADDR_CID_HOST */
#define DEFAULT_PORT 5000
#define DEFAULT_BITRATE 4000000
#define DEFAULT_KEYFRAME_INTERVAL 60

/* Pad templates */
static GstStaticPadTemplate sink_template = GST_STATIC_PAD_TEMPLATE(
    "sink",
    GST_PAD_SINK,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS(
        "video/x-raw, "
        "format = (string) { BGRx, BGRA, RGBx, RGBA, NV12 }, "
        "width = (int) [ 1, 8192 ], "
        "height = (int) [ 1, 8192 ], "
        "framerate = (fraction) [ 0/1, MAX ]"
    )
);

static GstStaticPadTemplate src_template = GST_STATIC_PAD_TEMPLATE(
    "src",
    GST_PAD_SRC,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS(
        "video/x-h264, "
        "stream-format = (string) byte-stream, "
        "alignment = (string) au"
    )
);

G_DEFINE_TYPE(GstVsockEnc, gst_vsockenc, GST_TYPE_VIDEO_ENCODER);

/* Forward declarations */
static void gst_vsockenc_finalize(GObject *object);
static void gst_vsockenc_set_property(GObject *object, guint prop_id,
                                       const GValue *value, GParamSpec *pspec);
static void gst_vsockenc_get_property(GObject *object, guint prop_id,
                                       GValue *value, GParamSpec *pspec);
static gboolean gst_vsockenc_start(GstVideoEncoder *encoder);
static gboolean gst_vsockenc_stop(GstVideoEncoder *encoder);
static gboolean gst_vsockenc_set_format(GstVideoEncoder *encoder,
                                         GstVideoCodecState *state);
static GstFlowReturn gst_vsockenc_handle_frame(GstVideoEncoder *encoder,
                                                GstVideoCodecFrame *frame);
static gpointer gst_vsockenc_recv_thread(gpointer data);
static gboolean gst_vsockenc_connect(GstVsockEnc *self);
static void gst_vsockenc_disconnect(GstVsockEnc *self);
static guint32 gst_vsockenc_get_resource_id(GstVsockEnc *self, GstBuffer *buffer);

static void
gst_vsockenc_class_init(GstVsockEncClass *klass)
{
    GObjectClass *gobject_class = G_OBJECT_CLASS(klass);
    GstElementClass *element_class = GST_ELEMENT_CLASS(klass);
    GstVideoEncoderClass *video_encoder_class = GST_VIDEO_ENCODER_CLASS(klass);

    gobject_class->finalize = gst_vsockenc_finalize;
    gobject_class->set_property = gst_vsockenc_set_property;
    gobject_class->get_property = gst_vsockenc_get_property;

    g_object_class_install_property(gobject_class, PROP_SOCKET_PATH,
        g_param_spec_string("socket-path", "Socket Path",
            "UNIX socket path for vsock (for QEMU/UTM)",
            DEFAULT_SOCKET_PATH,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    g_object_class_install_property(gobject_class, PROP_CID,
        g_param_spec_uint("cid", "CID",
            "vsock Context ID (2=host)",
            0, G_MAXUINT, DEFAULT_CID,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    g_object_class_install_property(gobject_class, PROP_PORT,
        g_param_spec_uint("port", "Port",
            "vsock port number",
            0, G_MAXUINT, DEFAULT_PORT,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    g_object_class_install_property(gobject_class, PROP_BITRATE,
        g_param_spec_int("bitrate", "Bitrate",
            "Target bitrate in bits per second",
            0, G_MAXINT, DEFAULT_BITRATE,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    g_object_class_install_property(gobject_class, PROP_KEYFRAME_INTERVAL,
        g_param_spec_int("keyframe-interval", "Keyframe Interval",
            "Interval between keyframes in frames",
            0, G_MAXINT, DEFAULT_KEYFRAME_INTERVAL,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    gst_element_class_add_static_pad_template(element_class, &sink_template);
    gst_element_class_add_static_pad_template(element_class, &src_template);

    gst_element_class_set_static_metadata(element_class,
        "vsock Video Encoder",
        "Codec/Encoder/Video",
        "Delegates video encoding to host via vsock (for VM→host VideoToolbox)",
        "Helix <support@helix.ml>");

    video_encoder_class->start = GST_DEBUG_FUNCPTR(gst_vsockenc_start);
    video_encoder_class->stop = GST_DEBUG_FUNCPTR(gst_vsockenc_stop);
    video_encoder_class->set_format = GST_DEBUG_FUNCPTR(gst_vsockenc_set_format);
    video_encoder_class->handle_frame = GST_DEBUG_FUNCPTR(gst_vsockenc_handle_frame);

    GST_DEBUG_CATEGORY_INIT(gst_vsockenc_debug, "vsockenc", 0,
        "vsock video encoder");
}

static void
gst_vsockenc_init(GstVsockEnc *self)
{
    self->socket_path = NULL;
    self->cid = DEFAULT_CID;
    self->port = DEFAULT_PORT;
    self->bitrate = DEFAULT_BITRATE;
    self->keyframe_interval = DEFAULT_KEYFRAME_INTERVAL;
    self->socket_fd = -1;
    self->connected = FALSE;
    self->frame_count = 0;
    self->running = FALSE;

    g_mutex_init(&self->lock);
    g_cond_init(&self->cond);
    self->pending_frames = g_queue_new();
}

static void
gst_vsockenc_finalize(GObject *object)
{
    GstVsockEnc *self = GST_VSOCKENC(object);

    g_free(self->socket_path);
    g_mutex_clear(&self->lock);
    g_cond_clear(&self->cond);
    g_queue_free(self->pending_frames);

    G_OBJECT_CLASS(gst_vsockenc_parent_class)->finalize(object);
}

static void
gst_vsockenc_set_property(GObject *object, guint prop_id,
                           const GValue *value, GParamSpec *pspec)
{
    GstVsockEnc *self = GST_VSOCKENC(object);

    switch (prop_id) {
        case PROP_SOCKET_PATH:
            g_free(self->socket_path);
            self->socket_path = g_value_dup_string(value);
            break;
        case PROP_CID:
            self->cid = g_value_get_uint(value);
            break;
        case PROP_PORT:
            self->port = g_value_get_uint(value);
            break;
        case PROP_BITRATE:
            self->bitrate = g_value_get_int(value);
            break;
        case PROP_KEYFRAME_INTERVAL:
            self->keyframe_interval = g_value_get_int(value);
            break;
        default:
            G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
            break;
    }
}

static void
gst_vsockenc_get_property(GObject *object, guint prop_id,
                           GValue *value, GParamSpec *pspec)
{
    GstVsockEnc *self = GST_VSOCKENC(object);

    switch (prop_id) {
        case PROP_SOCKET_PATH:
            g_value_set_string(value, self->socket_path);
            break;
        case PROP_CID:
            g_value_set_uint(value, self->cid);
            break;
        case PROP_PORT:
            g_value_set_uint(value, self->port);
            break;
        case PROP_BITRATE:
            g_value_set_int(value, self->bitrate);
            break;
        case PROP_KEYFRAME_INTERVAL:
            g_value_set_int(value, self->keyframe_interval);
            break;
        default:
            G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
            break;
    }
}

static gboolean
gst_vsockenc_connect(GstVsockEnc *self)
{
    if (self->connected)
        return TRUE;

    if (self->socket_path) {
        /* Connect via UNIX socket (for QEMU/UTM) */
        struct sockaddr_un addr;

        self->socket_fd = socket(AF_UNIX, SOCK_STREAM, 0);
        if (self->socket_fd < 0) {
            GST_ERROR_OBJECT(self, "Failed to create UNIX socket: %s",
                             g_strerror(errno));
            return FALSE;
        }

        memset(&addr, 0, sizeof(addr));
        addr.sun_family = AF_UNIX;
        strncpy(addr.sun_path, self->socket_path, sizeof(addr.sun_path) - 1);

        if (connect(self->socket_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
            GST_ERROR_OBJECT(self, "Failed to connect to %s: %s",
                             self->socket_path, g_strerror(errno));
            close(self->socket_fd);
            self->socket_fd = -1;
            return FALSE;
        }
    } else {
        /* Connect via native vsock */
        struct sockaddr_vm addr;

        self->socket_fd = socket(AF_VSOCK, SOCK_STREAM, 0);
        if (self->socket_fd < 0) {
            GST_ERROR_OBJECT(self, "Failed to create vsock socket: %s",
                             g_strerror(errno));
            return FALSE;
        }

        memset(&addr, 0, sizeof(addr));
        addr.svm_family = AF_VSOCK;
        addr.svm_cid = self->cid;
        addr.svm_port = self->port;

        if (connect(self->socket_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
            GST_ERROR_OBJECT(self, "Failed to connect to vsock %u:%u: %s",
                             self->cid, self->port, g_strerror(errno));
            close(self->socket_fd);
            self->socket_fd = -1;
            return FALSE;
        }
    }

    self->connected = TRUE;
    GST_INFO_OBJECT(self, "Connected to host encoder");
    return TRUE;
}

static void
gst_vsockenc_disconnect(GstVsockEnc *self)
{
    if (self->socket_fd >= 0) {
        close(self->socket_fd);
        self->socket_fd = -1;
    }
    self->connected = FALSE;
}

/* Extract virtio-gpu resource ID from DMA-BUF fd */
static guint32
gst_vsockenc_get_resource_id(GstVsockEnc *self, GstBuffer *buffer)
{
    GstMemory *mem;
    gint dmabuf_fd;
    guint32 resource_id = 0;

    mem = gst_buffer_peek_memory(buffer, 0);
    if (!gst_is_dmabuf_memory(mem)) {
        GST_WARNING_OBJECT(self, "Buffer is not DMA-BUF backed");
        return 0;
    }

    dmabuf_fd = gst_dmabuf_memory_get_fd(mem);
    if (dmabuf_fd < 0) {
        GST_WARNING_OBJECT(self, "Failed to get DMA-BUF fd");
        return 0;
    }

    /* Convert DMA-BUF fd to GEM handle, then get resource ID */
    /* With virtio-gpu, we need to go through the DRM subsystem */

    /* Open the DRM device (virtio-gpu) */
    int drm_fd = open("/dev/dri/renderD128", O_RDWR);
    if (drm_fd < 0) {
        /* Try card0 as fallback */
        drm_fd = open("/dev/dri/card0", O_RDWR);
        if (drm_fd < 0) {
            GST_WARNING_OBJECT(self, "Failed to open DRM device");
            return 0;
        }
    }

    /* Import DMA-BUF to get GEM handle */
    struct drm_prime_handle prime_handle = {
        .fd = dmabuf_fd,
        .flags = 0,
        .handle = 0,
    };

    if (ioctl(drm_fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, &prime_handle) < 0) {
        GST_WARNING_OBJECT(self, "Failed to get GEM handle from DMA-BUF: %s",
                           g_strerror(errno));
        close(drm_fd);
        return 0;
    }

    /* For virtio-gpu, the GEM handle IS the resource ID
     * (the kernel driver uses them interchangeably) */
    resource_id = prime_handle.handle;

    GST_DEBUG_OBJECT(self, "DMA-BUF fd %d -> GEM handle/resource ID %u",
                     dmabuf_fd, resource_id);

    close(drm_fd);
    return resource_id;
}

static gboolean
gst_vsockenc_start(GstVideoEncoder *encoder)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);

    self->frame_count = 0;
    self->running = TRUE;

    /* Start receive thread */
    self->recv_thread = g_thread_new("vsockenc-recv",
                                      gst_vsockenc_recv_thread, self);

    return TRUE;
}

static gboolean
gst_vsockenc_stop(GstVideoEncoder *encoder)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);

    self->running = FALSE;

    /* Signal receive thread to exit */
    g_mutex_lock(&self->lock);
    g_cond_signal(&self->cond);
    g_mutex_unlock(&self->lock);

    gst_vsockenc_disconnect(self);

    if (self->recv_thread) {
        g_thread_join(self->recv_thread);
        self->recv_thread = NULL;
    }

    if (self->input_state) {
        gst_video_codec_state_unref(self->input_state);
        self->input_state = NULL;
    }

    return TRUE;
}

static gboolean
gst_vsockenc_set_format(GstVideoEncoder *encoder, GstVideoCodecState *state)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);
    GstVideoCodecState *output_state;

    if (self->input_state)
        gst_video_codec_state_unref(self->input_state);

    self->input_state = gst_video_codec_state_ref(state);

    /* Set output caps */
    output_state = gst_video_encoder_set_output_state(encoder,
        gst_caps_new_simple("video/x-h264",
            "stream-format", G_TYPE_STRING, "byte-stream",
            "alignment", G_TYPE_STRING, "au",
            NULL),
        state);
    gst_video_codec_state_unref(output_state);

    return TRUE;
}

static GstFlowReturn
gst_vsockenc_handle_frame(GstVideoEncoder *encoder, GstVideoCodecFrame *frame)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);
    GstVideoInfo *info;
    VsockFrameRequest req;
    guint8 header[5];
    guint32 resource_id;
    ssize_t written;

    if (!self->connected) {
        if (!gst_vsockenc_connect(self)) {
            GST_ERROR_OBJECT(self, "Not connected to host encoder");
            return GST_FLOW_ERROR;
        }
    }

    info = &self->input_state->info;

    /* Get the virtio-gpu resource ID for this frame's DMA-BUF */
    resource_id = gst_vsockenc_get_resource_id(self, frame->input_buffer);
    if (resource_id == 0) {
        GST_WARNING_OBJECT(self, "Failed to get resource ID for frame");
        /* Fall through with resource_id=0, host will handle error */
    }

    /* Build frame request */
    req.resource_id = resource_id;
    req.width = GST_VIDEO_INFO_WIDTH(info);
    req.height = GST_VIDEO_INFO_HEIGHT(info);
    req.format = GST_VIDEO_INFO_FORMAT(info);
    req.pts = GST_BUFFER_PTS(frame->input_buffer);
    req.duration = GST_BUFFER_DURATION(frame->input_buffer);

    /* Send header: type (1 byte) + length (4 bytes) */
    header[0] = VSOCK_MSG_FRAME_REQUEST;
    *((guint32 *)&header[1]) = sizeof(VsockFrameRequest);

    g_mutex_lock(&self->lock);

    written = write(self->socket_fd, header, 5);
    if (written != 5) {
        g_mutex_unlock(&self->lock);
        GST_ERROR_OBJECT(self, "Failed to write header");
        return GST_FLOW_ERROR;
    }

    written = write(self->socket_fd, &req, sizeof(req));
    if (written != sizeof(req)) {
        g_mutex_unlock(&self->lock);
        GST_ERROR_OBJECT(self, "Failed to write frame request");
        return GST_FLOW_ERROR;
    }

    /* Queue frame for later completion when we receive encoded data */
    g_queue_push_tail(self->pending_frames, gst_video_codec_frame_ref(frame));

    g_mutex_unlock(&self->lock);

    self->frame_count++;

    GST_DEBUG_OBJECT(self, "Sent frame %lu, resource_id=%u, size=%ux%u",
                     self->frame_count, resource_id,
                     req.width, req.height);

    return GST_FLOW_OK;
}

/* Thread to receive encoded frames from host */
static gpointer
gst_vsockenc_recv_thread(gpointer data)
{
    GstVsockEnc *self = GST_VSOCKENC(data);
    guint8 header[5];
    ssize_t nread;

    GST_DEBUG_OBJECT(self, "Receive thread started");

    while (self->running) {
        if (!self->connected) {
            g_usleep(100000);  /* 100ms */
            continue;
        }

        /* Read message header */
        nread = read(self->socket_fd, header, 5);
        if (nread <= 0) {
            if (nread < 0 && errno == EAGAIN)
                continue;
            if (self->running) {
                GST_WARNING_OBJECT(self, "Connection lost");
                self->connected = FALSE;
            }
            continue;
        }

        guint8 msg_type = header[0];
        guint32 msg_len = *((guint32 *)&header[1]);

        if (msg_type == VSOCK_MSG_FRAME_RESPONSE && msg_len > 0) {
            /* Read response header */
            VsockFrameResponseHeader resp_header;
            nread = read(self->socket_fd, &resp_header, sizeof(resp_header));
            if (nread != sizeof(resp_header)) {
                GST_WARNING_OBJECT(self, "Failed to read response header");
                continue;
            }

            /* Read NAL data */
            guint8 *nal_data = g_malloc(resp_header.nal_length);
            nread = read(self->socket_fd, nal_data, resp_header.nal_length);
            if (nread != resp_header.nal_length) {
                GST_WARNING_OBJECT(self, "Failed to read NAL data");
                g_free(nal_data);
                continue;
            }

            /* Find matching pending frame by PTS */
            g_mutex_lock(&self->lock);

            GstVideoCodecFrame *frame = NULL;
            GList *l;
            for (l = self->pending_frames->head; l; l = l->next) {
                GstVideoCodecFrame *f = l->data;
                if (GST_BUFFER_PTS(f->input_buffer) == resp_header.pts) {
                    frame = f;
                    g_queue_delete_link(self->pending_frames, l);
                    break;
                }
            }

            g_mutex_unlock(&self->lock);

            if (frame) {
                /* Create output buffer with NAL data */
                GstBuffer *outbuf = gst_buffer_new_wrapped(nal_data, resp_header.nal_length);

                if (resp_header.is_keyframe) {
                    GST_VIDEO_CODEC_FRAME_SET_SYNC_POINT(frame);
                }

                frame->output_buffer = outbuf;

                /* Finish the frame */
                gst_video_encoder_finish_frame(GST_VIDEO_ENCODER(self), frame);

                GST_DEBUG_OBJECT(self, "Finished frame pts=%" G_GINT64_FORMAT
                                 " keyframe=%d nal_size=%u",
                                 resp_header.pts, resp_header.is_keyframe,
                                 resp_header.nal_length);
            } else {
                GST_WARNING_OBJECT(self, "No pending frame for pts=%" G_GINT64_FORMAT,
                                   resp_header.pts);
                g_free(nal_data);
            }
        }
    }

    GST_DEBUG_OBJECT(self, "Receive thread exiting");
    return NULL;
}

/* Plugin registration */
static gboolean
plugin_init(GstPlugin *plugin)
{
    return gst_element_register(plugin, "vsockenc",
                                GST_RANK_PRIMARY, GST_TYPE_VSOCKENC);
}

GST_PLUGIN_DEFINE(
    GST_VERSION_MAJOR,
    GST_VERSION_MINOR,
    vsockenc,
    "Video encoder delegating to host via vsock",
    plugin_init,
    "1.0",
    "LGPL",
    "Helix",
    "https://helix.ml"
)
