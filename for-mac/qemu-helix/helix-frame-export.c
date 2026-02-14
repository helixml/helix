/*
 * Helix Frame Export for QEMU/UTM
 *
 * Zero-copy video encoding: virtio-gpu resource -> Metal texture ->
 * IOSurface -> VideoToolbox H.264 -> vsock back to guest
 *
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

#include "helix-frame-export.h"

#ifdef __APPLE__

#include <dispatch/dispatch.h>
#include <CoreMedia/CoreMedia.h>
#include <CoreVideo/CoreVideo.h>
#include <Metal/Metal.h>
#include <sys/socket.h>

/* virglrenderer includes */
#include "virglrenderer.h"

/* QEMU includes - paths will vary in actual QEMU tree */
// #include "qemu/osdep.h"
// #include "hw/virtio/virtio-gpu.h"
// #include "qemu/error-report.h"

/* Placeholder for QEMU error reporting */
#define error_report(...) fprintf(stderr, "helix: " __VA_ARGS__)

/*
 * VideoToolbox encoder output callback
 * Called asynchronously when a frame is encoded
 */
static void encoder_output_callback(void *outputCallbackRefCon,
                                    void *sourceFrameRefCon,
                                    OSStatus status,
                                    VTEncodeInfoFlags infoFlags,
                                    CMSampleBufferRef sampleBuffer)
{
    HelixFrameExport *fe = (HelixFrameExport *)outputCallbackRefCon;
    int64_t pts = (int64_t)sourceFrameRefCon;

    if (status != noErr) {
        error_report("VideoToolbox encode failed: %d\n", (int)status);
        fe->encode_errors++;
        return;
    }

    if (!sampleBuffer) {
        return;
    }

    /* Check if this is a keyframe */
    CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, false);
    bool is_keyframe = true;
    if (attachments && CFArrayGetCount(attachments) > 0) {
        CFDictionaryRef dict = CFArrayGetValueAtIndex(attachments, 0);
        CFBooleanRef notSync = CFDictionaryGetValue(dict, kCMSampleAttachmentKey_NotSync);
        if (notSync && CFBooleanGetValue(notSync)) {
            is_keyframe = false;
        }
    }

    /* Get the data buffer */
    CMBlockBufferRef dataBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
    if (!dataBuffer) {
        error_report("No data buffer in sample\n");
        return;
    }

    size_t totalLength = 0;
    char *dataPtr = NULL;
    OSStatus err = CMBlockBufferGetDataPointer(dataBuffer, 0, NULL,
                                                &totalLength, &dataPtr);
    if (err != noErr || !dataPtr) {
        error_report("Failed to get data pointer: %d\n", (int)err);
        return;
    }

    /* Build response message */
    size_t response_size = sizeof(HelixFrameResponse) + sizeof(uint32_t) + totalLength;
    uint8_t *response = malloc(response_size);
    if (!response) {
        error_report("Failed to allocate response buffer\n");
        return;
    }

    HelixFrameResponse *resp = (HelixFrameResponse *)response;
    resp->header.magic = HELIX_MSG_MAGIC;
    resp->header.msg_type = HELIX_MSG_FRAME_RESPONSE;
    resp->header.flags = 0;
    resp->header.session_id = fe->session_id;
    resp->header.payload_size = response_size - sizeof(HelixMsgHeader);

    CMTime decode_time = CMSampleBufferGetDecodeTimeStamp(sampleBuffer);
    resp->pts = pts;
    resp->dts = CMTimeGetSeconds(decode_time) * 1000000000LL;
    resp->is_keyframe = is_keyframe ? 1 : 0;
    resp->nal_count = 1;  /* Single NAL unit for now */

    /* Write NAL size and data */
    uint32_t nal_size = (uint32_t)totalLength;
    memcpy(response + sizeof(HelixFrameResponse), &nal_size, sizeof(nal_size));
    memcpy(response + sizeof(HelixFrameResponse) + sizeof(uint32_t),
           dataPtr, totalLength);

    /* Send response over vsock */
    ssize_t sent = send(fe->vsock_fd, response, response_size, 0);
    if (sent < 0) {
        error_report("Failed to send response: %s\n", strerror(errno));
    } else {
        fe->frames_encoded++;
        fe->bytes_sent += sent;
    }

    free(response);
}

/*
 * Create and configure VideoToolbox encoder session
 */
static int create_encoder_session(HelixFrameExport *fe,
                                   int32_t width,
                                   int32_t height,
                                   int32_t bitrate,
                                   bool realtime)
{
    OSStatus status;

    /* Clean up existing session */
    if (fe->encoder_session) {
        VTCompressionSessionCompleteFrames(fe->encoder_session,
                                            kCMTimeInvalid);
        VTCompressionSessionInvalidate(fe->encoder_session);
        CFRelease(fe->encoder_session);
        fe->encoder_session = NULL;
    }

    /* Source image attributes (IOSurface-backed) */
    CFMutableDictionaryRef sourceAttrs = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);

    /* We accept IOSurface-backed pixel buffers */
    CFDictionarySetValue(sourceAttrs,
                         kCVPixelBufferIOSurfacePropertiesKey,
                         CFDictionaryCreate(kCFAllocatorDefault,
                                           NULL, NULL, 0,
                                           &kCFTypeDictionaryKeyCallBacks,
                                           &kCFTypeDictionaryValueCallBacks));

    /* Create compression session */
    status = VTCompressionSessionCreate(
        kCFAllocatorDefault,
        width,
        height,
        kCMVideoCodecType_H264,
        NULL,           /* encoderSpecification - let VT choose */
        sourceAttrs,    /* sourceImageBufferAttributes */
        NULL,           /* compressedDataAllocator */
        encoder_output_callback,
        fe,             /* outputCallbackRefCon */
        &fe->encoder_session
    );

    CFRelease(sourceAttrs);

    if (status != noErr) {
        error_report("VTCompressionSessionCreate failed: %d\n", (int)status);
        return -1;
    }

    /* Configure for low latency if requested */
    if (realtime) {
        VTSessionSetProperty(fe->encoder_session,
                             kVTCompressionPropertyKey_RealTime,
                             kCFBooleanTrue);

        /* Disable B-frames for lower latency */
        VTSessionSetProperty(fe->encoder_session,
                             kVTCompressionPropertyKey_AllowFrameReordering,
                             kCFBooleanFalse);

        /* Max keyframe interval (1 per second at 60fps) */
        int maxKeyFrameInterval = 60;
        CFNumberRef maxKeyFrameIntervalRef = CFNumberCreate(
            kCFAllocatorDefault, kCFNumberIntType, &maxKeyFrameInterval);
        VTSessionSetProperty(fe->encoder_session,
                             kVTCompressionPropertyKey_MaxKeyFrameInterval,
                             maxKeyFrameIntervalRef);
        CFRelease(maxKeyFrameIntervalRef);
    }

    /* Set bitrate */
    if (bitrate > 0) {
        CFNumberRef bitrateRef = CFNumberCreate(
            kCFAllocatorDefault, kCFNumberIntType, &bitrate);
        VTSessionSetProperty(fe->encoder_session,
                             kVTCompressionPropertyKey_AverageBitRate,
                             bitrateRef);
        CFRelease(bitrateRef);
    }

    /* H.264 Main Profile, Level 4.1 (suitable for 1080p60) */
    VTSessionSetProperty(fe->encoder_session,
                         kVTCompressionPropertyKey_ProfileLevel,
                         kVTProfileLevel_H264_Main_AutoLevel);

    /* Prepare to encode */
    status = VTCompressionSessionPrepareToEncodeFrames(fe->encoder_session);
    if (status != noErr) {
        error_report("PrepareToEncodeFrames failed: %d\n", (int)status);
        CFRelease(fe->encoder_session);
        fe->encoder_session = NULL;
        return -1;
    }

    fe->width = width;
    fe->height = height;
    fe->bitrate = bitrate;
    fe->realtime = realtime;
    fe->configured = true;

    return 0;
}

/*
 * Look up IOSurface for a virtio-gpu resource (zero-copy)
 *
 * The MTLTexture MUST be backed by IOSurface. If not, this fails
 * and we need to modify virglrenderer to create IOSurface-backed textures.
 */
IOSurfaceRef helix_get_iosurface_for_resource(void *virtio_gpu,
                                               uint32_t resource_id)
{
    struct virgl_renderer_resource_info_ext info_ext = {0};

    int ret = virgl_renderer_resource_get_info_ext(resource_id, &info_ext);
    if (ret != 0) {
        error_report("virgl_renderer_resource_get_info_ext failed: %d\n", ret);
        return NULL;
    }

    if (info_ext.native_type != VIRGL_NATIVE_HANDLE_METAL_TEXTURE) {
        error_report("Resource %u is not a Metal texture (type=%d)\n",
                     resource_id, info_ext.native_type);
        return NULL;
    }

    /* native_handle is MTLTexture* */
    id<MTLTexture> texture = (__bridge id<MTLTexture>)(void *)info_ext.native_handle;
    if (!texture) {
        error_report("Resource %u has NULL Metal texture\n", resource_id);
        return NULL;
    }

    /* Get IOSurface - texture MUST be backed by IOSurface for zero-copy */
    IOSurfaceRef surface = texture.iosurface;
    if (!surface) {
        error_report("Metal texture has no IOSurface backing - "
                     "virglrenderer must create IOSurface-backed textures\n");
        return NULL;
    }

    IOSurfaceIncrementUseCount(surface);
    return surface;
}

/*
 * Encode an IOSurface frame
 */
int helix_encode_iosurface(HelixFrameExport *fe,
                           IOSurfaceRef surface,
                           int64_t pts,
                           int64_t duration,
                           bool force_keyframe)
{
    if (!fe->configured || !fe->encoder_session) {
        return HELIX_ERR_NOT_CONFIGURED;
    }

    /* Create CVPixelBuffer from IOSurface (zero-copy) */
    CVPixelBufferRef pixelBuffer = NULL;
    CVReturn cvRet = CVPixelBufferCreateWithIOSurface(
        kCFAllocatorDefault,
        surface,
        NULL,  /* pixelBufferAttributes */
        &pixelBuffer
    );

    if (cvRet != kCVReturnSuccess || !pixelBuffer) {
        error_report("CVPixelBufferCreateWithIOSurface failed: %d\n", cvRet);
        return HELIX_ERR_NO_IOSURFACE;
    }

    /* Presentation timestamp */
    CMTime cmPts = CMTimeMake(pts, 1000000000);  /* nanoseconds */
    CMTime cmDuration = CMTimeMake(duration, 1000000000);

    /* Frame properties */
    CFMutableDictionaryRef frameProps = NULL;
    if (force_keyframe) {
        frameProps = CFDictionaryCreateMutable(
            kCFAllocatorDefault, 1,
            &kCFTypeDictionaryKeyCallBacks,
            &kCFTypeDictionaryValueCallBacks);
        CFDictionarySetValue(frameProps,
                             kVTEncodeFrameOptionKey_ForceKeyFrame,
                             kCFBooleanTrue);
    }

    /* Encode the frame */
    OSStatus status = VTCompressionSessionEncodeFrame(
        fe->encoder_session,
        pixelBuffer,
        cmPts,
        cmDuration,
        frameProps,
        (void *)pts,  /* sourceFrameRefCon - pass pts for callback */
        NULL          /* infoFlagsOut */
    );

    if (frameProps) {
        CFRelease(frameProps);
    }
    CVPixelBufferRelease(pixelBuffer);

    if (status != noErr) {
        error_report("VTCompressionSessionEncodeFrame failed: %d\n", (int)status);
        return HELIX_ERR_ENCODE_FAILED;
    }

    return HELIX_ERR_OK;
}

/*
 * Handle frame request from guest
 */
static int handle_frame_request(HelixFrameExport *fe,
                                 const HelixFrameRequest *req)
{
    /* Auto-configure encoder on first frame or resolution change */
    if (!fe->configured ||
        fe->width != (int32_t)req->width ||
        fe->height != (int32_t)req->height) {

        /* Scale bitrate with pixel count: 8 Mbps at 1080p, ~20 Mbps at 4K, ~40 Mbps at 5K */
        int64_t pixels = (int64_t)req->width * (int64_t)req->height;
        int32_t bitrate = (int32_t)(pixels * 4); /* ~4 bits per pixel */
        if (bitrate < 8000000) bitrate = 8000000;

        int ret = create_encoder_session(fe, req->width, req->height,
                                          bitrate,
                                          true);    /* realtime */
        if (ret != 0) {
            return HELIX_ERR_INTERNAL;
        }
    }

    /* Look up IOSurface for this resource */
    IOSurfaceRef surface = helix_get_iosurface_for_resource(
        fe->virtio_gpu, req->resource_id);

    if (!surface) {
        return HELIX_ERR_RESOURCE_NOT_FOUND;
    }

    /* Encode the frame */
    int ret = helix_encode_iosurface(fe, surface, req->pts, req->duration,
                                      req->force_keyframe != 0);

    IOSurfaceDecrementUseCount(surface);

    return ret;
}

/*
 * Handle config request from guest
 */
static int handle_config_request(HelixFrameExport *fe,
                                  const HelixConfigRequest *req)
{
    int ret = create_encoder_session(fe,
                                      req->width,
                                      req->height,
                                      req->bitrate,
                                      req->realtime != 0);
    return ret == 0 ? HELIX_ERR_OK : HELIX_ERR_INTERNAL;
}

/*
 * Process incoming message from guest
 */
int helix_frame_export_process_msg(HelixFrameExport *fe,
                                    const uint8_t *data,
                                    size_t len)
{
    if (len < sizeof(HelixMsgHeader)) {
        return HELIX_ERR_INVALID_MSG;
    }

    const HelixMsgHeader *header = (const HelixMsgHeader *)data;

    if (header->magic != HELIX_MSG_MAGIC) {
        error_report("Invalid message magic: 0x%08x\n", header->magic);
        return HELIX_ERR_INVALID_MSG;
    }

    switch (header->msg_type) {
    case HELIX_MSG_FRAME_REQUEST:
        if (len < sizeof(HelixFrameRequest)) {
            return HELIX_ERR_INVALID_MSG;
        }
        return handle_frame_request(fe, (const HelixFrameRequest *)data);

    case HELIX_MSG_CONFIG_REQ:
        if (len < sizeof(HelixConfigRequest)) {
            return HELIX_ERR_INVALID_MSG;
        }
        return handle_config_request(fe, (const HelixConfigRequest *)data);

    case HELIX_MSG_KEYFRAME_REQ:
        /* Force next frame to be keyframe */
        /* This is handled implicitly via force_keyframe in frame request */
        return HELIX_ERR_OK;

    case HELIX_MSG_PING:
        {
            /* Send pong response */
            HelixMsgHeader pong = {
                .magic = HELIX_MSG_MAGIC,
                .msg_type = HELIX_MSG_PONG,
                .flags = 0,
                .session_id = header->session_id,
                .payload_size = 0
            };
            send(fe->vsock_fd, &pong, sizeof(pong), 0);
            return HELIX_ERR_OK;
        }

    default:
        error_report("Unknown message type: %d\n", header->msg_type);
        return HELIX_ERR_INVALID_MSG;
    }
}

/*
 * Cleanup frame export
 */
void helix_frame_export_cleanup(HelixFrameExport *fe)
{
    if (!fe) return;

    if (fe->encoder_session) {
        VTCompressionSessionCompleteFrames(fe->encoder_session, kCMTimeInvalid);
        VTCompressionSessionInvalidate(fe->encoder_session);
        CFRelease(fe->encoder_session);
    }

    if (fe->vsock_fd >= 0) {
        close(fe->vsock_fd);
    }

    free(fe);
}

/*
 * vsock server thread - listens for connections and processes messages
 */
static void *vsock_server_thread(void *arg)
{
    HelixFrameExport *fe = (HelixFrameExport *)arg;
    uint8_t buffer[65536];

    while (1) {
        ssize_t received = recv(fe->vsock_fd, buffer, sizeof(buffer), 0);
        if (received <= 0) {
            if (received < 0 && errno != EINTR) {
                error_report("vsock recv error: %s\n", strerror(errno));
            }
            break;
        }

        int ret = helix_frame_export_process_msg(fe, buffer, received);
        if (ret != HELIX_ERR_OK) {
            /* Send error response */
            HelixErrorResponse err = {
                .header = {
                    .magic = HELIX_MSG_MAGIC,
                    .msg_type = HELIX_MSG_ERROR,
                    .flags = 0,
                    .session_id = fe->session_id,
                    .payload_size = sizeof(HelixErrorResponse) - sizeof(HelixMsgHeader)
                },
                .error_code = ret
            };
            snprintf(err.message, sizeof(err.message), "Error: %d", ret);
            send(fe->vsock_fd, &err, sizeof(err), 0);
        }
    }

    return NULL;
}

/*
 * Initialize frame export subsystem
 * This would be called from virtio_gpu_virgl_init() in QEMU
 */
int helix_frame_export_init(void *virtio_gpu, int vsock_port)
{
    HelixFrameExport *fe = calloc(1, sizeof(HelixFrameExport));
    if (!fe) {
        return -1;
    }

    fe->virtio_gpu = virtio_gpu;
    fe->vsock_fd = -1;
    fe->session_id = 1;  /* Default session */

    /*
     * TODO: Set up vsock listener on vsock_port
     *
     * In QEMU, this would use the virtio-vsock device.
     * The guest connects to CID 2 (host), port HELIX_VSOCK_PORT.
     *
     * For now, this is a placeholder - actual vsock integration
     * depends on QEMU's vsock implementation.
     */

    error_report("Helix frame export initialized on vsock port %d\n", vsock_port);

    /* Store in virtio-gpu device for later access */
    /* TODO: Add helix_frame_export field to VirtIOGPU struct */

    return 0;
}

#endif /* __APPLE__ */
