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

/* vsock protocol message types */
typedef enum {
    VSOCK_MSG_FRAME_REQUEST = 1,
    VSOCK_MSG_FRAME_RESPONSE = 2,
    VSOCK_MSG_KEYFRAME_REQ = 3,
    VSOCK_MSG_PING = 4,
    VSOCK_MSG_PONG = 5,
} VsockMsgType;

/* Frame request sent to host */
typedef struct {
    guint32 resource_id;
    guint32 width;
    guint32 height;
    guint32 format;
    gint64 pts;
    gint64 duration;
} __attribute__((packed)) VsockFrameRequest;

/* Frame response received from host */
typedef struct {
    gint64 pts;
    guint8 is_keyframe;
    guint32 nal_length;
    /* NAL data follows */
} __attribute__((packed)) VsockFrameResponseHeader;

G_END_DECLS

#endif /* __GST_VSOCKENC_H__ */
