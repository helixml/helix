package main

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Foundation -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation -framework IOSurface -framework Metal

#import <VideoToolbox/VideoToolbox.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <IOSurface/IOSurface.h>
#import <Metal/Metal.h>
#import <stdbool.h>

// Callback context for encoded frames
typedef struct {
	void* goCallback;
} EncoderContext;

// Forward declaration
void encoderOutputCallback(void *outputCallbackRefCon,
                           void *sourceFrameRefCon,
                           OSStatus status,
                           VTEncodeInfoFlags infoFlags,
                           CMSampleBufferRef sampleBuffer);

// Helper functions to check for null (cgo has issues with nil comparisons)
static inline bool isSessionNull(VTCompressionSessionRef session) {
	return session == NULL;
}

static inline bool isSurfaceNull(IOSurfaceRef surface) {
	return surface == NULL;
}

static inline bool isSampleBufferNull(CMSampleBufferRef buffer) {
	return buffer == NULL;
}

static inline bool isArrayNull(CFArrayRef array) {
	return array == NULL;
}

static inline bool isDataBufferNull(CMBlockBufferRef buffer) {
	return buffer == NULL;
}

// Create VideoToolbox compression session for H.264 encoding
static VTCompressionSessionRef createCompressionSession(int width, int height, int fps, int bitrate, void* callbackCtx) {
	VTCompressionSessionRef session = NULL;

	// Configure encoder properties
	CFMutableDictionaryRef encoderSpec = CFDictionaryCreateMutable(
		kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks);

	// Force hardware encoder
	CFDictionarySetValue(encoderSpec,
		kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder,
		kCFBooleanTrue);

	// Create the compression session
	OSStatus status = VTCompressionSessionCreate(
		kCFAllocatorDefault,
		width,
		height,
		kCMVideoCodecType_H264,
		encoderSpec,
		NULL,  // sourceImageBufferAttributes - let it choose
		kCFAllocatorDefault,
		encoderOutputCallback,
		callbackCtx,
		&session);

	CFRelease(encoderSpec);

	if (status != noErr) {
		NSLog(@"Failed to create compression session: %d", (int)status);
		return NULL;
	}

	// Configure session properties for low-latency streaming
	VTSessionSetProperty(session, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
	VTSessionSetProperty(session, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);

	// Set profile to Baseline for maximum compatibility
	VTSessionSetProperty(session, kVTCompressionPropertyKey_ProfileLevel,
		kVTProfileLevel_H264_Baseline_AutoLevel);

	// Set bitrate
	CFNumberRef bitrateNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &bitrate);
	VTSessionSetProperty(session, kVTCompressionPropertyKey_AverageBitRate, bitrateNum);
	CFRelease(bitrateNum);

	// Set frame rate
	CFNumberRef fpsNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &fps);
	VTSessionSetProperty(session, kVTCompressionPropertyKey_ExpectedFrameRate, fpsNum);
	CFRelease(fpsNum);

	// Set keyframe interval (every 2 seconds)
	int keyframeInterval = fps * 2;
	CFNumberRef keyframeNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &keyframeInterval);
	VTSessionSetProperty(session, kVTCompressionPropertyKey_MaxKeyFrameInterval, keyframeNum);
	CFRelease(keyframeNum);

	// Prepare to encode
	VTCompressionSessionPrepareToEncodeFrames(session);

	return session;
}

// Encode a frame from IOSurface
static OSStatus encodeIOSurface(VTCompressionSessionRef session, IOSurfaceRef surface, int64_t pts, int64_t duration) {
	// Create CVPixelBuffer from IOSurface (zero-copy)
	CVPixelBufferRef pixelBuffer = NULL;
	OSStatus status = CVPixelBufferCreateWithIOSurface(
		kCFAllocatorDefault,
		surface,
		NULL,  // pixelBufferAttributes
		&pixelBuffer);

	if (status != noErr || pixelBuffer == NULL) {
		NSLog(@"Failed to create pixel buffer from IOSurface: %d", (int)status);
		return status;
	}

	// Create presentation timestamp
	CMTime presentationTime = CMTimeMake(pts, 1000000000);  // nanoseconds
	CMTime frameDuration = CMTimeMake(duration, 1000000000);

	// Encode the frame
	status = VTCompressionSessionEncodeFrame(
		session,
		pixelBuffer,
		presentationTime,
		frameDuration,
		NULL,  // frameProperties
		NULL,  // sourceFrameRefCon
		NULL); // infoFlagsOut

	CVPixelBufferRelease(pixelBuffer);

	return status;
}

// Encode a frame from CVPixelBuffer
static OSStatus encodePixelBuffer(VTCompressionSessionRef session, CVPixelBufferRef pixelBuffer, int64_t pts, int64_t duration) {
	CMTime presentationTime = CMTimeMake(pts, 1000000000);
	CMTime frameDuration = CMTimeMake(duration, 1000000000);

	return VTCompressionSessionEncodeFrame(
		session,
		pixelBuffer,
		presentationTime,
		frameDuration,
		NULL,
		NULL,
		NULL);
}

// Force keyframe on next encode
static void forceKeyframe(VTCompressionSessionRef session) {
	// This will be applied to the next frame
	// The actual implementation requires passing frameProperties to EncodeFrame
}

// Flush pending frames
static OSStatus flushEncoder(VTCompressionSessionRef session) {
	return VTCompressionSessionCompleteFrames(session, kCMTimeInvalid);
}

// Destroy compression session
static void destroyCompressionSession(VTCompressionSessionRef session) {
	if (session != NULL) {
		VTCompressionSessionInvalidate(session);
		CFRelease(session);
	}
}

// Get IOSurface from Metal texture
static IOSurfaceRef getIOSurfaceFromMetalTexture(void* metalTexture) {
	id<MTLTexture> texture = (__bridge id<MTLTexture>)metalTexture;
	return texture.iosurface;
}

// Lookup IOSurface by global ID (deprecated but still needed for cross-process sharing)
static IOSurfaceRef lookupIOSurface(uint32_t surfaceID) {
	return IOSurfaceLookup(surfaceID);
}

// Get IOSurface ID
static uint32_t getIOSurfaceID(IOSurfaceRef surface) {
	return IOSurfaceGetID(surface);
}

// Create IOSurface with specific properties for video encoding
// Note: kIOSurfaceIsGlobal is deprecated but required for cross-process sharing
static IOSurfaceRef createIOSurface(int width, int height) {
	NSDictionary *properties = @{
		(__bridge NSString *)kIOSurfaceWidth: @(width),
		(__bridge NSString *)kIOSurfaceHeight: @(height),
		(__bridge NSString *)kIOSurfaceBytesPerElement: @4,
		(__bridge NSString *)kIOSurfacePixelFormat: @(kCVPixelFormatType_32BGRA),
		(__bridge NSString *)kIOSurfaceIsGlobal: @YES,  // Allow cross-process sharing
	};

	return IOSurfaceCreate((__bridge CFDictionaryRef)properties);
}

// Release IOSurface
static void releaseIOSurface(IOSurfaceRef surface) {
	if (surface != NULL) {
		CFRelease(surface);
	}
}

// Process encoded sample buffer and extract NAL data
typedef struct {
	void* data;
	int length;
	bool isKeyframe;
	int64_t pts;
} EncodedFrame;

static EncodedFrame processEncodedBuffer(CMSampleBufferRef sampleBuffer) {
	EncodedFrame frame = {NULL, 0, false, 0};

	if (isSampleBufferNull(sampleBuffer)) {
		return frame;
	}

	// Check if this is a keyframe
	CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, false);
	if (!isArrayNull(attachments) && CFArrayGetCount(attachments) > 0) {
		CFDictionaryRef attachment = CFArrayGetValueAtIndex(attachments, 0);
		if (attachment != NULL) {
			CFBooleanRef notSync = NULL;
			if (CFDictionaryGetValueIfPresent(attachment, kCMSampleAttachmentKey_NotSync, (const void**)&notSync)) {
				frame.isKeyframe = (notSync != kCFBooleanTrue);
			} else {
				frame.isKeyframe = true;
			}
		}
	}

	// Get presentation timestamp
	CMTime pts = CMSampleBufferGetPresentationTimeStamp(sampleBuffer);
	frame.pts = (int64_t)(CMTimeGetSeconds(pts) * 1e9);

	// Get the data buffer
	CMBlockBufferRef dataBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
	if (isDataBufferNull(dataBuffer)) {
		return frame;
	}

	char* dataPtr = NULL;
	size_t dataLen = 0;
	CMBlockBufferGetDataPointer(dataBuffer, 0, NULL, &dataLen, &dataPtr);

	if (dataPtr != NULL && dataLen > 0) {
		frame.data = dataPtr;
		frame.length = (int)dataLen;
	}

	return frame;
}

*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// NALCallback is called when an encoded NAL unit is ready
type NALCallback func(nalUnit []byte, isKeyframe bool, pts int64)

// Global encoder registry for callback routing
var encoderRegistry = struct {
	sync.RWMutex
	encoders map[uintptr]*VideoToolboxEncoder
}{
	encoders: make(map[uintptr]*VideoToolboxEncoder),
}

// VideoToolboxEncoder encodes video frames using Apple VideoToolbox
type VideoToolboxEncoder struct {
	session    C.VTCompressionSessionRef
	width      int
	height     int
	fps        int
	bitrate    int
	callback   NALCallback
	mu         sync.Mutex
	running    bool
	frameCount uint64
	id         uintptr // Unique ID for callback routing
}

// NewVideoToolboxEncoder creates a new VideoToolbox encoder
func NewVideoToolboxEncoder(width, height, fps, bitrate int) *VideoToolboxEncoder {
	return &VideoToolboxEncoder{
		width:   width,
		height:  height,
		fps:     fps,
		bitrate: bitrate,
	}
}

// Start starts the encoder
func (e *VideoToolboxEncoder) Start(callback NALCallback) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("encoder already running")
	}

	e.callback = callback

	// Generate unique ID for callback routing
	e.id = uintptr(unsafe.Pointer(e))

	// Register encoder for callback routing
	encoderRegistry.Lock()
	encoderRegistry.encoders[e.id] = e
	encoderRegistry.Unlock()

	// Create compression session with encoder ID as callback context
	e.session = C.createCompressionSession(
		C.int(e.width),
		C.int(e.height),
		C.int(e.fps),
		C.int(e.bitrate),
		unsafe.Pointer(e.id))

	if C.isSessionNull(e.session) {
		// Unregister on failure
		encoderRegistry.Lock()
		delete(encoderRegistry.encoders, e.id)
		encoderRegistry.Unlock()
		return fmt.Errorf("failed to create VideoToolbox compression session")
	}

	e.running = true
	return nil
}

// Stop stops the encoder
func (e *VideoToolboxEncoder) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil
	}

	// Unregister encoder
	encoderRegistry.Lock()
	delete(encoderRegistry.encoders, e.id)
	encoderRegistry.Unlock()

	// Flush pending frames
	C.flushEncoder(e.session)

	// Destroy session
	C.destroyCompressionSession(e.session)
	// session is now invalid, mark as zero value
	e.running = false

	return nil
}

// EncodeIOSurface encodes a frame from an IOSurface (zero-copy path)
func (e *VideoToolboxEncoder) EncodeIOSurface(surfaceID uint32, pts int64, duration int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("encoder not running")
	}

	// Lookup IOSurface by global ID
	surface := C.lookupIOSurface(C.uint32_t(surfaceID))
	if C.isSurfaceNull(surface) {
		return fmt.Errorf("failed to lookup IOSurface with ID %d", surfaceID)
	}
	defer C.releaseIOSurface(surface)

	status := C.encodeIOSurface(e.session, surface, C.int64_t(pts), C.int64_t(duration))
	if status != 0 {
		return fmt.Errorf("failed to encode frame: status %d", status)
	}

	e.frameCount++
	return nil
}

// EncodeFromMetalTexture encodes a frame from a Metal texture (zero-copy path)
func (e *VideoToolboxEncoder) EncodeFromMetalTexture(metalTexture unsafe.Pointer, pts int64, duration int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("encoder not running")
	}

	// Get IOSurface from Metal texture
	surface := C.getIOSurfaceFromMetalTexture(metalTexture)
	if C.isSurfaceNull(surface) {
		return fmt.Errorf("failed to get IOSurface from Metal texture")
	}

	status := C.encodeIOSurface(e.session, surface, C.int64_t(pts), C.int64_t(duration))
	if status != 0 {
		return fmt.Errorf("failed to encode frame: status %d", status)
	}

	e.frameCount++
	return nil
}

// GetFrameCount returns the number of frames encoded
func (e *VideoToolboxEncoder) GetFrameCount() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.frameCount
}

// IsRunning returns whether the encoder is running
func (e *VideoToolboxEncoder) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// CreateIOSurface creates a new IOSurface for frame capture
func CreateIOSurface(width, height int) (uint32, error) {
	surface := C.createIOSurface(C.int(width), C.int(height))
	if C.isSurfaceNull(surface) {
		return 0, fmt.Errorf("failed to create IOSurface")
	}

	surfaceID := uint32(C.getIOSurfaceID(surface))
	// Note: The IOSurface is now owned by the caller via its global ID
	// Don't release it here

	return surfaceID, nil
}

// LookupIOSurface looks up an IOSurface by its global ID
func LookupIOSurface(surfaceID uint32) (unsafe.Pointer, error) {
	surface := C.lookupIOSurface(C.uint32_t(surfaceID))
	if C.isSurfaceNull(surface) {
		return nil, fmt.Errorf("failed to lookup IOSurface with ID %d", surfaceID)
	}
	return unsafe.Pointer(surface), nil
}

// The output callback is called by VideoToolbox when a frame is encoded
// We need to export this function to C

//export encoderOutputCallback
func encoderOutputCallback(
	outputCallbackRefCon unsafe.Pointer,
	sourceFrameRefCon unsafe.Pointer,
	status C.OSStatus,
	infoFlags C.VTEncodeInfoFlags,
	sampleBuffer C.CMSampleBufferRef,
) {
	if status != 0 {
		fmt.Printf("Encoder callback error: %d\n", status)
		return
	}

	// Process the encoded buffer using the C helper function
	frame := C.processEncodedBuffer(sampleBuffer)
	if frame.data == nil || frame.length == 0 {
		return
	}

	// Copy the NAL data to Go
	nalData := C.GoBytes(frame.data, frame.length)
	isKeyframe := bool(frame.isKeyframe)
	ptsNano := int64(frame.pts)

	// Route to the appropriate encoder via the callback context
	encoderID := uintptr(outputCallbackRefCon)

	encoderRegistry.RLock()
	encoder, ok := encoderRegistry.encoders[encoderID]
	encoderRegistry.RUnlock()

	if ok && encoder.callback != nil {
		encoder.callback(nalData, isKeyframe, ptsNano)
	}
}
