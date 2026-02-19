/*
 * Helix Frame Export for QEMU/UTM
 *
 * This module provides zero-copy video encoding by:
 * 1. Listening for frame requests from guest via vsock
 * 2. Looking up virtio-gpu resources via virglrenderer
 * 3. Encoding with VideoToolbox using the native Metal texture
 * 4. Sending H.264 NAL units back to guest
 *
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

#ifndef HELIX_FRAME_EXPORT_H
#define HELIX_FRAME_EXPORT_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __APPLE__
#include <CoreFoundation/CoreFoundation.h>
#include <VideoToolbox/VideoToolbox.h>
#include <IOSurface/IOSurface.h>
#endif

/* vsock port for frame export (well-known port) */
#define HELIX_VSOCK_PORT 5000

/* Message types (guest <-> host) */
#define HELIX_MSG_FRAME_REQUEST   0x01  /* Guest -> Host: encode this resource */
#define HELIX_MSG_FRAME_RESPONSE  0x02  /* Host -> Guest: encoded NAL data */
#define HELIX_MSG_KEYFRAME_REQ    0x03  /* Guest -> Host: force keyframe */
#define HELIX_MSG_CONFIG_REQ      0x04  /* Guest -> Host: configure encoder */
#define HELIX_MSG_CONFIG_RESP     0x05  /* Host -> Guest: encoder config ack */
#define HELIX_MSG_PING            0x10  /* Keepalive */
#define HELIX_MSG_PONG            0x11  /* Keepalive response */
#define HELIX_MSG_ERROR           0xFF  /* Error response */

/* Pixel formats (matching DRM/GBM formats) */
#define HELIX_FORMAT_BGRA8888     0x34325241  /* DRM_FORMAT_ARGB8888 */
#define HELIX_FORMAT_RGBA8888     0x34324241  /* DRM_FORMAT_ABGR8888 */
#define HELIX_FORMAT_NV12         0x3231564E  /* DRM_FORMAT_NV12 */
#define HELIX_FORMAT_UNKNOWN      0x00000000

/* Message header (common to all messages) */
typedef struct HelixMsgHeader {
    uint32_t magic;         /* 'HXFR' = 0x52465848 */
    uint8_t  msg_type;
    uint8_t  flags;
    uint16_t session_id;    /* For multiplexing multiple streams */
    uint32_t payload_size;
} __attribute__((packed)) HelixMsgHeader;

#define HELIX_MSG_MAGIC 0x52465848  /* 'HXFR' in little-endian */

/* Frame request: guest asks host to encode a virtio-gpu resource */
typedef struct HelixFrameRequest {
    HelixMsgHeader header;
    uint32_t resource_id;   /* virtio-gpu resource ID */
    uint32_t width;
    uint32_t height;
    uint32_t format;        /* HELIX_FORMAT_* */
    uint32_t stride;        /* Bytes per row */
    int64_t  pts;           /* Presentation timestamp (nanoseconds) */
    int64_t  duration;      /* Frame duration (nanoseconds) */
    uint8_t  force_keyframe;
    uint8_t  reserved[7];
} __attribute__((packed)) HelixFrameRequest;

/* Frame response: host returns encoded H.264 data */
typedef struct HelixFrameResponse {
    HelixMsgHeader header;
    int64_t  pts;           /* Same as request */
    int64_t  dts;           /* Decode timestamp */
    uint8_t  is_keyframe;
    uint8_t  reserved[3];
    uint32_t nal_count;     /* Number of NAL units */
    /* Followed by: nal_count x (uint32_t size + NAL data) */
} __attribute__((packed)) HelixFrameResponse;

/* Encoder configuration request */
typedef struct HelixConfigRequest {
    HelixMsgHeader header;
    uint32_t width;
    uint32_t height;
    uint32_t bitrate;       /* Target bitrate in bits/sec */
    uint32_t framerate_num; /* Framerate numerator */
    uint32_t framerate_den; /* Framerate denominator */
    uint8_t  profile;       /* H.264 profile (66=baseline, 77=main, 100=high) */
    uint8_t  level;         /* H.264 level * 10 (e.g., 40 = level 4.0) */
    uint8_t  realtime;      /* 1 = optimize for low latency */
    uint8_t  reserved[5];
} __attribute__((packed)) HelixConfigRequest;

/* Error response */
typedef struct HelixErrorResponse {
    HelixMsgHeader header;
    int32_t  error_code;
    char     message[256];
} __attribute__((packed)) HelixErrorResponse;

/* Error codes */
#define HELIX_ERR_OK              0
#define HELIX_ERR_INVALID_MSG    -1
#define HELIX_ERR_RESOURCE_NOT_FOUND -2
#define HELIX_ERR_NOT_METAL_TEXTURE  -3
#define HELIX_ERR_NO_IOSURFACE       -4
#define HELIX_ERR_ENCODE_FAILED      -5
#define HELIX_ERR_NOT_CONFIGURED     -6
#define HELIX_ERR_INTERNAL           -99

#ifdef __APPLE__

/*
 * Frame export context - created per session
 */
typedef struct HelixFrameExport {
    /* Encoder state */
    VTCompressionSessionRef encoder_session;
    int32_t width;
    int32_t height;
    int32_t bitrate;
    bool realtime;
    bool configured;

    /* vsock connection */
    int vsock_fd;
    uint16_t session_id;

    /* Pending frame response queue */
    /* (encoder callbacks are async, need to queue responses) */
    void *response_queue;  /* dispatch_queue_t */

    /* Statistics */
    uint64_t frames_encoded;
    uint64_t bytes_sent;
    uint64_t encode_errors;

    /* Reference to virtio-gpu for resource lookup */
    void *virtio_gpu;
} HelixFrameExport;

/*
 * Initialize frame export subsystem
 * Called from virtio_gpu_virgl_init()
 */
int helix_frame_export_init(void *virtio_gpu, int vsock_port);

/*
 * Cleanup frame export
 */
void helix_frame_export_cleanup(HelixFrameExport *fe);

/*
 * Process incoming message from guest
 */
int helix_frame_export_process_msg(HelixFrameExport *fe,
                                    const uint8_t *data,
                                    size_t len);

/*
 * Look up IOSurface for a virtio-gpu resource
 * Returns IOSurfaceRef (retained) or NULL
 */
IOSurfaceRef helix_get_iosurface_for_resource(void *virtio_gpu,
                                               uint32_t resource_id);

/*
 * Encode an IOSurface frame
 */
int helix_encode_iosurface(HelixFrameExport *fe,
                           IOSurfaceRef surface,
                           int64_t pts,
                           int64_t duration,
                           bool force_keyframe);

#endif /* __APPLE__ */

#endif /* HELIX_FRAME_EXPORT_H */
