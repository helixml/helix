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
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <arpa/inet.h>
#include <linux/vm_sockets.h>
#include <libdrm/drm.h>
#include <drm/virtgpu_drm.h>
#include <xf86drm.h>
#include <sys/ioctl.h>
#include <fcntl.h>
#include <poll.h>

GST_DEBUG_CATEGORY_STATIC(gst_vsockenc_debug);
#define GST_CAT_DEFAULT gst_vsockenc_debug

/* Properties */
enum {
    PROP_0,
    PROP_SOCKET_PATH,
    PROP_CID,
    PROP_PORT,
    PROP_TCP_HOST,
    PROP_TCP_PORT,
    PROP_BITRATE,
    PROP_KEYFRAME_INTERVAL,
};

#define DEFAULT_SOCKET_PATH NULL
#define DEFAULT_CID 2  /* VMADDR_CID_HOST */
#define DEFAULT_PORT 5000
#define DEFAULT_TCP_HOST NULL
#define DEFAULT_TCP_PORT 15937
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
        "framerate = (fraction) [ 0/1, MAX ]; "
        "video/x-raw(memory:DMABuf), "
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
static gboolean gst_vsockenc_connect(GstVsockEnc *self);
static void gst_vsockenc_disconnect(GstVsockEnc *self);
static gboolean read_exact(int fd, void *buf, size_t n);
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

    g_object_class_install_property(gobject_class, PROP_TCP_HOST,
        g_param_spec_string("tcp-host", "TCP Host",
            "TCP hostname for testing (e.g., 10.0.2.2 for QEMU user-mode networking)",
            DEFAULT_TCP_HOST,
            G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));

    g_object_class_install_property(gobject_class, PROP_TCP_PORT,
        g_param_spec_uint("tcp-port", "TCP Port",
            "TCP port number (default 15937)",
            0, G_MAXUINT, DEFAULT_TCP_PORT,
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
    self->tcp_host = NULL;
    self->tcp_port = DEFAULT_TCP_PORT;
    self->bitrate = DEFAULT_BITRATE;
    self->keyframe_interval = DEFAULT_KEYFRAME_INTERVAL;
    self->socket_fd = -1;
    self->drm_fd = -1;
    self->connected = FALSE;
    self->frame_count = 0;
    self->running = FALSE;

    g_mutex_init(&self->lock);
    g_cond_init(&self->cond);
}

static void
gst_vsockenc_finalize(GObject *object)
{
    GstVsockEnc *self = GST_VSOCKENC(object);

    if (self->drm_fd >= 0) {
        close(self->drm_fd);
        self->drm_fd = -1;
    }
    g_free(self->socket_path);
    g_free(self->tcp_host);
    g_mutex_clear(&self->lock);
    g_cond_clear(&self->cond);

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
        case PROP_TCP_HOST:
            g_free(self->tcp_host);
            self->tcp_host = g_value_dup_string(value);
            break;
        case PROP_TCP_PORT:
            self->tcp_port = g_value_get_uint(value);
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
        case PROP_TCP_HOST:
            g_value_set_string(value, self->tcp_host);
            break;
        case PROP_TCP_PORT:
            g_value_set_uint(value, self->tcp_port);
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
        /* Connect via UNIX socket (for 9p/virtfs) */
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
        GST_INFO_OBJECT(self, "Connected via UNIX socket: %s", self->socket_path);
    } else if (self->tcp_host) {
        /* Connect via TCP (for QEMU user-mode networking) */
        struct sockaddr_in addr;

        self->socket_fd = socket(AF_INET, SOCK_STREAM, 0);
        if (self->socket_fd < 0) {
            GST_ERROR_OBJECT(self, "Failed to create TCP socket: %s",
                             g_strerror(errno));
            return FALSE;
        }

        memset(&addr, 0, sizeof(addr));
        addr.sin_family = AF_INET;
        addr.sin_port = htons(self->tcp_port);

        if (inet_pton(AF_INET, self->tcp_host, &addr.sin_addr) <= 0) {
            GST_ERROR_OBJECT(self, "Invalid TCP host address: %s", self->tcp_host);
            close(self->socket_fd);
            self->socket_fd = -1;
            return FALSE;
        }

        if (connect(self->socket_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
            GST_ERROR_OBJECT(self, "Failed to connect to %s:%u: %s",
                             self->tcp_host, self->tcp_port, g_strerror(errno));
            close(self->socket_fd);
            self->socket_fd = -1;
            return FALSE;
        }

        /* TCP tuning for low-latency frame streaming */
        int flag = 1;
        setsockopt(self->socket_fd, IPPROTO_TCP, TCP_NODELAY, &flag, sizeof(flag));

        /* Large send buffer for 777KB pixel data frames */
        int sndbuf = 1048576;  /* 1MB */
        setsockopt(self->socket_fd, SOL_SOCKET, SO_SNDBUF, &sndbuf, sizeof(sndbuf));

        GST_INFO_OBJECT(self, "Connected via TCP to %s:%u (TCP_NODELAY, sndbuf=1MB)",
                         self->tcp_host, self->tcp_port);
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
        GST_INFO_OBJECT(self, "Connected via vsock to %u:%u", self->cid, self->port);
    }

    self->connected = TRUE;
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

/* Open and cache the DRM device fd for resource ID lookups.
 * Must be called once before gst_vsockenc_get_resource_id(). */
static gboolean
gst_vsockenc_open_drm(GstVsockEnc *self)
{
    if (self->drm_fd >= 0)
        return TRUE;

    self->drm_fd = open("/dev/dri/renderD128", O_RDWR);
    if (self->drm_fd < 0) {
        self->drm_fd = open("/dev/dri/card0", O_RDWR);
        if (self->drm_fd < 0) {
            GST_WARNING_OBJECT(self, "Failed to open DRM device: %s",
                               g_strerror(errno));
            return FALSE;
        }
    }
    GST_INFO_OBJECT(self, "Opened DRM device fd=%d", self->drm_fd);
    return TRUE;
}

/* Extract virtio-gpu resource ID from DMA-BUF fd.
 *
 * The chain is:
 *   DMA-BUF fd  →  DRM_IOCTL_PRIME_FD_TO_HANDLE  →  GEM handle (per-process local)
 *   GEM handle  →  DRM_IOCTL_VIRTGPU_RESOURCE_INFO  →  res_handle (virtio-gpu resource ID)
 *
 * The resource ID is the same one that QEMU/virglrenderer uses on the host.
 * QEMU can read pixel data from it via virgl_renderer_transfer_read_iov().
 *
 * IMPORTANT: GEM handles are NOT resource IDs! They are per-process local
 * identifiers. The VIRTGPU_RESOURCE_INFO ioctl is required to get the
 * actual virtio-gpu resource ID that maps to the host-side backing store.
 */
static guint32
gst_vsockenc_get_resource_id(GstVsockEnc *self, GstBuffer *buffer)
{
    GstMemory *mem;
    gint dmabuf_fd;

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

    if (!gst_vsockenc_open_drm(self))
        return 0;

    /* Step 1: Import DMA-BUF fd to get per-process GEM handle */
    struct drm_prime_handle prime_handle = {
        .fd = dmabuf_fd,
        .flags = 0,
        .handle = 0,
    };

    if (ioctl(self->drm_fd, DRM_IOCTL_PRIME_FD_TO_HANDLE, &prime_handle) < 0) {
        GST_WARNING_OBJECT(self, "PRIME_FD_TO_HANDLE failed (fd=%d): %s",
                           dmabuf_fd, g_strerror(errno));
        return 0;
    }

    /* Step 2: Get virtio-gpu resource ID from GEM handle.
     * This is the globally unique ID that QEMU/virglrenderer uses. */
    struct drm_virtgpu_resource_info res_info = {
        .bo_handle = prime_handle.handle,
        .res_handle = 0,
        .size = 0,
        .blob_mem = 0,
    };

    if (ioctl(self->drm_fd, DRM_IOCTL_VIRTGPU_RESOURCE_INFO, &res_info) < 0) {
        GST_WARNING_OBJECT(self, "VIRTGPU_RESOURCE_INFO failed (gem=%u): %s",
                           prime_handle.handle, g_strerror(errno));
        /* Close the GEM handle to avoid leaking it */
        struct drm_gem_close gem_close = { .handle = prime_handle.handle };
        ioctl(self->drm_fd, DRM_IOCTL_GEM_CLOSE, &gem_close);
        return 0;
    }

    GST_INFO_OBJECT(self, "DMA-BUF fd=%d -> GEM handle=%u -> resource_id=%u (size=%u, blob_mem=%u)",
                    dmabuf_fd, prime_handle.handle, res_info.res_handle,
                    res_info.size, res_info.blob_mem);

    /* Close the GEM handle - we only needed it to look up the resource ID.
     * The DMA-BUF fd keeps the underlying resource alive. */
    struct drm_gem_close gem_close = { .handle = prime_handle.handle };
    ioctl(self->drm_fd, DRM_IOCTL_GEM_CLOSE, &gem_close);

    return res_info.res_handle;
}

/*
 * Read response from host and finish the pending frame.
 * If block=TRUE, waits until response arrives.
 * If block=FALSE, uses poll() to check without blocking.
 * Returns TRUE if a frame was finished, FALSE if no data available (non-blocking).
 */
static gboolean
gst_vsockenc_finish_pending(GstVsockEnc *self, GstVideoEncoder *encoder,
                             gboolean block)
{
    if (!self->pending_frame)
        return TRUE;  /* Nothing pending */

    if (!block) {
        /* Non-blocking check: is response data available? */
        struct pollfd pfd = { .fd = self->socket_fd, .events = POLLIN };
        int ret = poll(&pfd, 1, 0);  /* timeout=0 → immediate return */
        if (ret <= 0)
            return FALSE;  /* No data yet */
    }

    /* Read response header */
    HelixMsgHeader header;
    if (!read_exact(self->socket_fd, &header, sizeof(header))) {
        GST_ERROR_OBJECT(self, "Failed to read response header for pending frame");
        self->connected = FALSE;
        gst_video_encoder_finish_frame(encoder, self->pending_frame);
        self->pending_frame = NULL;
        return TRUE;
    }

    if (header.magic != HELIX_MSG_MAGIC) {
        GST_ERROR_OBJECT(self, "Invalid response magic for pending frame: 0x%08x", header.magic);
        gst_video_encoder_finish_frame(encoder, self->pending_frame);
        self->pending_frame = NULL;
        return TRUE;
    }

    if (header.msg_type == HELIX_MSG_ERROR) {
        HelixErrorResponse err;
        memcpy(&err.header, &header, sizeof(header));
        size_t remaining = sizeof(HelixErrorResponse) - sizeof(HelixMsgHeader);
        if (read_exact(self->socket_fd, ((guint8 *)&err) + sizeof(HelixMsgHeader), remaining)) {
            GST_ERROR_OBJECT(self, "Host encoder error %d: %s", err.error_code, err.message);
        }
        gst_video_encoder_finish_frame(encoder, self->pending_frame);
        self->pending_frame = NULL;
        return TRUE;
    }

    if (header.msg_type != HELIX_MSG_FRAME_RESPONSE) {
        GST_WARNING_OBJECT(self, "Unexpected message type: %d", header.msg_type);
        if (header.payload_size > 0) {
            guint8 *skip = g_malloc(header.payload_size);
            read_exact(self->socket_fd, skip, header.payload_size);
            g_free(skip);
        }
        gst_video_encoder_finish_frame(encoder, self->pending_frame);
        self->pending_frame = NULL;
        return TRUE;
    }

    /* Read frame response body */
    HelixFrameResponse resp;
    memcpy(&resp.header, &header, sizeof(header));
    size_t remaining = sizeof(HelixFrameResponse) - sizeof(HelixMsgHeader);
    if (!read_exact(self->socket_fd, ((guint8 *)&resp) + sizeof(HelixMsgHeader), remaining)) {
        GST_ERROR_OBJECT(self, "Failed to read frame response body");
        gst_video_encoder_finish_frame(encoder, self->pending_frame);
        self->pending_frame = NULL;
        return TRUE;
    }

    /* Read NAL units */
    GstBuffer *outbuf = gst_buffer_new();
    guint32 total_nal_size = 0;

    for (guint32 i = 0; i < resp.nal_count; i++) {
        guint32 nal_size;
        if (!read_exact(self->socket_fd, &nal_size, sizeof(nal_size))) {
            GST_WARNING_OBJECT(self, "Failed to read NAL size");
            gst_buffer_unref(outbuf);
            gst_video_encoder_finish_frame(encoder, self->pending_frame);
            self->pending_frame = NULL;
            return TRUE;
        }

        guint8 *nal_data = g_malloc(nal_size);
        if (!read_exact(self->socket_fd, nal_data, nal_size)) {
            GST_WARNING_OBJECT(self, "Failed to read NAL data");
            g_free(nal_data);
            gst_buffer_unref(outbuf);
            gst_video_encoder_finish_frame(encoder, self->pending_frame);
            self->pending_frame = NULL;
            return TRUE;
        }

        GstMemory *mem = gst_memory_new_wrapped(0, nal_data, nal_size,
                                                 0, nal_size, nal_data, g_free);
        gst_buffer_append_memory(outbuf, mem);
        total_nal_size += nal_size;
    }

    /* Set frame properties and finish */
    if (resp.is_keyframe) {
        GST_VIDEO_CODEC_FRAME_SET_SYNC_POINT(self->pending_frame);
    }

    GST_BUFFER_DTS(outbuf) = resp.dts;
    self->pending_frame->output_buffer = outbuf;

    GST_DEBUG_OBJECT(self, "Finished pending frame pts=%" G_GINT64_FORMAT
                     " keyframe=%d nal_count=%u total_size=%u",
                     resp.pts, resp.is_keyframe, resp.nal_count, total_nal_size);

    gst_video_encoder_finish_frame(encoder, self->pending_frame);
    self->pending_frame = NULL;
    return TRUE;
}

static gboolean
gst_vsockenc_start(GstVideoEncoder *encoder)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);

    self->frame_count = 0;
    self->running = TRUE;
    self->pending_frame = NULL;

    return TRUE;
}

static gboolean
gst_vsockenc_stop(GstVideoEncoder *encoder)
{
    GstVsockEnc *self = GST_VSOCKENC(encoder);

    self->running = FALSE;

    /* Drain last pending frame (blocking read) */
    if (self->pending_frame && self->connected) {
        GST_DEBUG_OBJECT(self, "Draining last pending frame on stop");
        gst_vsockenc_finish_pending(self, encoder, TRUE);
    }

    gst_vsockenc_disconnect(self);

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
    HelixFrameRequest req;
    guint32 resource_id;
    ssize_t written;
    gboolean force_keyframe;

    if (!self->connected) {
        if (!gst_vsockenc_connect(self)) {
            GST_ERROR_OBJECT(self, "Not connected to host encoder");
            return GST_FLOW_ERROR;
        }
    }

    /*
     * Pipelined encoding: finish the PREVIOUS frame's response before
     * sending the current frame. This overlaps host encoding of frame N
     * with GStreamer processing of frame N+1.
     *
     * First, try non-blocking: if the response arrived already, finish it.
     * If not (host still encoding), block and wait - this is the slower
     * path but ensures correctness.
     */
    if (self->pending_frame) {
        if (!gst_vsockenc_finish_pending(self, encoder, FALSE)) {
            /* Response not ready yet - block for it */
            gst_vsockenc_finish_pending(self, encoder, TRUE);
        }
    }

    info = &self->input_state->info;

    /* Get the virtio-gpu resource ID for this frame's DMA-BUF */
    resource_id = gst_vsockenc_get_resource_id(self, frame->input_buffer);
    if (resource_id == 0) {
        GST_WARNING_OBJECT(self, "Failed to get resource ID for frame");
        /* Fall through with resource_id=0, host will handle error */
    }

    /* Check if keyframe is requested */
    force_keyframe = GST_VIDEO_CODEC_FRAME_IS_FORCE_KEYFRAME(frame) ||
                     (self->keyframe_interval > 0 &&
                      self->frame_count % self->keyframe_interval == 0);

    /* Build frame request with Helix protocol header */
    memset(&req, 0, sizeof(req));
    req.header.magic = HELIX_MSG_MAGIC;
    req.header.msg_type = HELIX_MSG_FRAME_REQUEST;
    req.header.flags = 0;
    req.header.session_id = 1;  /* Default session */
    req.header.payload_size = sizeof(HelixFrameRequest) - sizeof(HelixMsgHeader);

    req.resource_id = resource_id;
    req.width = GST_VIDEO_INFO_WIDTH(info);
    req.height = GST_VIDEO_INFO_HEIGHT(info);
    req.stride = GST_VIDEO_INFO_PLANE_STRIDE(info, 0);
    req.pts = GST_BUFFER_PTS(frame->input_buffer);
    req.duration = GST_BUFFER_DURATION(frame->input_buffer);
    req.force_keyframe = force_keyframe ? 1 : 0;

    /* Map GStreamer format to Helix format */
    switch (GST_VIDEO_INFO_FORMAT(info)) {
        case GST_VIDEO_FORMAT_BGRx:
        case GST_VIDEO_FORMAT_BGRA:
            req.format = HELIX_FORMAT_BGRA8888;
            break;
        case GST_VIDEO_FORMAT_RGBx:
        case GST_VIDEO_FORMAT_RGBA:
            req.format = HELIX_FORMAT_RGBA8888;
            break;
        case GST_VIDEO_FORMAT_NV12:
            req.format = HELIX_FORMAT_NV12;
            break;
        default:
            req.format = HELIX_FORMAT_BGRA8888;  /* Default */
            break;
    }

    /*
     * If buffer is SHM (resource_id=0), send raw pixel data with the request.
     * The host can't read from GPU resources for container-internal screens,
     * so we must send the actual pixels.
     */
    GstMapInfo map_info;
    gboolean has_pixel_data = FALSE;

    if (resource_id == 0 &&
        gst_buffer_map(frame->input_buffer, &map_info, GST_MAP_READ)) {
        has_pixel_data = TRUE;
        req.header.flags |= HELIX_FLAG_PIXEL_DATA;
        req.header.payload_size = sizeof(HelixFrameRequest) - sizeof(HelixMsgHeader) + map_info.size;
    }

    /* Send frame request header to host encoder */
    written = write(self->socket_fd, &req, sizeof(req));
    if (written != sizeof(req)) {
        GST_ERROR_OBJECT(self, "Failed to write frame request: %s",
                         g_strerror(errno));
        if (has_pixel_data) gst_buffer_unmap(frame->input_buffer, &map_info);
        return GST_FLOW_ERROR;
    }

    /* Send pixel data if SHM buffer */
    if (has_pixel_data) {
        size_t total_written = 0;
        while (total_written < map_info.size) {
            ssize_t w = write(self->socket_fd,
                              map_info.data + total_written,
                              map_info.size - total_written);
            if (w <= 0) {
                if (w < 0 && errno == EINTR) continue;
                GST_ERROR_OBJECT(self, "Failed to write pixel data: %s",
                                 g_strerror(errno));
                gst_buffer_unmap(frame->input_buffer, &map_info);
                self->connected = FALSE;
                return GST_FLOW_ERROR;
            }
            total_written += w;
        }
        GST_DEBUG_OBJECT(self, "Sent %zu bytes of pixel data", map_info.size);
        gst_buffer_unmap(frame->input_buffer, &map_info);
    }

    self->frame_count++;

    GST_DEBUG_OBJECT(self, "Sent frame %lu, resource_id=%u, size=%ux%u, keyframe=%d",
                     self->frame_count, resource_id,
                     req.width, req.height, force_keyframe);

    /*
     * Pipelined: store this frame as pending and return immediately.
     * The response will be read at the start of the NEXT handle_frame call,
     * overlapping host encoding with upstream GStreamer processing.
     * All calls stay on the streaming thread (no go-gst threading issues).
     */
    self->pending_frame = frame;
    return GST_FLOW_OK;
}

/* Read exactly n bytes from socket */
static gboolean
read_exact(int fd, void *buf, size_t n)
{
    size_t total = 0;
    while (total < n) {
        ssize_t r = read(fd, (guint8 *)buf + total, n - total);
        if (r <= 0) {
            if (r < 0 && errno == EINTR)
                continue;
            return FALSE;
        }
        total += r;
    }
    return TRUE;
}

/* recv_thread removed - synchronous send/receive in handle_frame */

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
