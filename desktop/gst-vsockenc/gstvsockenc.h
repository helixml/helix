/*
 * GStreamer vsockenc - Video encoder that delegates to host via vsock
 *
 * This element receives DMA-BUF backed video frames, extracts the
 * virtio-gpu resource ID, sends it to the host for VideoToolbox encoding,
 * and outputs the H.264 NAL units received back.
 *
 * Used for zero-copy video encoding on macOS hosts running Linux VMs.
 */

#ifndef __GST_VSOCKENC_H__
#define __GST_VSOCKENC_H__

#include <gst/gst.h>
#include <gst/video/video.h>
#include <gst/video/gstvideometa.h>
#include <gst/allocators/gstdmabuf.h>

G_BEGIN_DECLS

#define GST_TYPE_VSOCKENC (gst_vsockenc_get_type())
G_DECLARE_FINAL_TYPE(GstVsockEnc, gst_vsockenc, GST, VSOCKENC, GstVideoEncoder)

struct _GstVsockEnc {
    GstVideoEncoder parent;

    /* Properties */
    gchar *socket_path;     /* vsock socket path (for QEMU/UTM) */
    guint cid;              /* vsock CID (for native vsock) */
    guint port;             /* vsock port */
    gint bitrate;           /* Target bitrate in bps */
    gint keyframe_interval; /* Keyframe interval in frames */

    /* State */
    GstVideoCodecState *input_state;
    gint socket_fd;
    gboolean connected;
    GMutex lock;
    GCond cond;

    /* Frame tracking */
    guint64 frame_count;
    GQueue *pending_frames;  /* Frames waiting for encoded response */

    /* Thread for receiving encoded data */
    GThread *recv_thread;
    gboolean running;
};

/*
 * Helix Frame Export Protocol
 * Must match for-mac/qemu-helix/helix-frame-export.h
 */

/* Message magic: 'HXFR' in little-endian */
#define HELIX_MSG_MAGIC 0x52465848

/* Message types */
#define HELIX_MSG_FRAME_REQUEST   0x01
#define HELIX_MSG_FRAME_RESPONSE  0x02
#define HELIX_MSG_KEYFRAME_REQ    0x03
#define HELIX_MSG_CONFIG_REQ      0x04
#define HELIX_MSG_CONFIG_RESP     0x05
#define HELIX_MSG_PING            0x10
#define HELIX_MSG_PONG            0x11
#define HELIX_MSG_ERROR           0xFF

/* Pixel formats (matching DRM formats) */
#define HELIX_FORMAT_BGRA8888     0x34325241
#define HELIX_FORMAT_RGBA8888     0x34324241
#define HELIX_FORMAT_NV12         0x3231564E

/* Default vsock port */
#define HELIX_VSOCK_PORT 5000

/* Message header (common to all messages) */
typedef struct {
    guint32 magic;          /* HELIX_MSG_MAGIC */
    guint8  msg_type;
    guint8  flags;
    guint16 session_id;
    guint32 payload_size;
} __attribute__((packed)) HelixMsgHeader;

/* Frame request sent to host */
typedef struct {
    HelixMsgHeader header;
    guint32 resource_id;
    guint32 width;
    guint32 height;
    guint32 format;
    guint32 stride;
    gint64  pts;
    gint64  duration;
    guint8  force_keyframe;
    guint8  reserved[7];
} __attribute__((packed)) HelixFrameRequest;

/* Frame response received from host */
typedef struct {
    HelixMsgHeader header;
    gint64  pts;
    gint64  dts;
    guint8  is_keyframe;
    guint8  reserved[3];
    guint32 nal_count;
    /* Followed by: nal_count x (guint32 size + NAL data) */
} __attribute__((packed)) HelixFrameResponse;

/* Encoder configuration request */
typedef struct {
    HelixMsgHeader header;
    guint32 width;
    guint32 height;
    guint32 bitrate;
    guint32 framerate_num;
    guint32 framerate_den;
    guint8  profile;
    guint8  level;
    guint8  realtime;
    guint8  reserved[5];
} __attribute__((packed)) HelixConfigRequest;

/* Error response */
typedef struct {
    HelixMsgHeader header;
    gint32  error_code;
    gchar   message[256];
} __attribute__((packed)) HelixErrorResponse;

G_END_DECLS

#endif /* __GST_VSOCKENC_H__ */
