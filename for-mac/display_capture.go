package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework IOSurface -framework CoreVideo -framework VideoToolbox -framework CoreMedia

#import <Foundation/Foundation.h>
#import <IOSurface/IOSurface.h>
#import <CoreVideo/CoreVideo.h>
#import <VideoToolbox/VideoToolbox.h>
#import <CoreMedia/CoreMedia.h>

// DisplayCapture manages capturing and encoding a display IOSurface
typedef struct {
    uint32_t surfaceID;
    uint32_t width;
    uint32_t height;
    VTCompressionSessionRef encoder;
    int64_t frameCount;
    int running;
} DisplayCapture;

static DisplayCapture g_capture = {0};

// Callback for encoded frames
void captureOutputCallback(void *outputCallbackRefCon,
                           void *sourceFrameRefCon,
                           OSStatus status,
                           VTEncodeInfoFlags infoFlags,
                           CMSampleBufferRef sampleBuffer);

// Find UTM's display IOSurface by scanning known ID ranges
uint32_t findUTMDisplaySurface(uint32_t* width, uint32_t* height) {
    // UTM typically creates surfaces with IDs in a certain range
    // We look for surfaces that look like VM displays:
    // - Resolution >= 800x600
    // - BGRA pixel format
    // - Reasonable size (not tiny textures)

    uint32_t foundID = 0;
    uint32_t foundWidth = 0;
    uint32_t foundHeight = 0;
    size_t foundSize = 0;

    // Scan ID ranges
    uint32_t ranges[][2] = {
        {1, 2000},
        {10000, 12000},
        {100000, 102000},
    };

    for (int r = 0; r < 3; r++) {
        for (uint32_t id = ranges[r][0]; id < ranges[r][1]; id++) {
            IOSurfaceRef surface = IOSurfaceLookup(id);
            if (surface == NULL) continue;

            size_t w = IOSurfaceGetWidth(surface);
            size_t h = IOSurfaceGetHeight(surface);
            size_t size = IOSurfaceGetAllocSize(surface);
            OSType format = IOSurfaceGetPixelFormat(surface);

            CFRelease(surface);

            // Look for display-sized BGRA surfaces
            if (w >= 800 && h >= 600 && format == 'BGRA') {
                // Prefer larger surfaces (more likely to be main display)
                if (size > foundSize) {
                    foundID = id;
                    foundWidth = (uint32_t)w;
                    foundHeight = (uint32_t)h;
                    foundSize = size;
                }
            }
        }
    }

    if (foundID != 0) {
        *width = foundWidth;
        *height = foundHeight;
    }

    return foundID;
}

// Initialize display capture with a specific IOSurface ID
int initDisplayCapture(uint32_t surfaceID) {
    IOSurfaceRef surface = IOSurfaceLookup(surfaceID);
    if (surface == NULL) {
        NSLog(@"Failed to lookup IOSurface %u", surfaceID);
        return -1;
    }

    g_capture.surfaceID = surfaceID;
    g_capture.width = (uint32_t)IOSurfaceGetWidth(surface);
    g_capture.height = (uint32_t)IOSurfaceGetHeight(surface);

    CFRelease(surface);

    // Create VideoToolbox encoder
    CFMutableDictionaryRef encoderSpec = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);

    CFDictionarySetValue(encoderSpec,
        kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder,
        kCFBooleanTrue);

    OSStatus status = VTCompressionSessionCreate(
        kCFAllocatorDefault,
        g_capture.width,
        g_capture.height,
        kCMVideoCodecType_H264,
        encoderSpec,
        NULL,
        kCFAllocatorDefault,
        captureOutputCallback,
        NULL,
        &g_capture.encoder);

    CFRelease(encoderSpec);

    if (status != noErr) {
        NSLog(@"Failed to create compression session: %d", (int)status);
        return -2;
    }

    // Configure encoder for low-latency streaming
    VTSessionSetProperty(g_capture.encoder, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
    VTSessionSetProperty(g_capture.encoder, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
    VTSessionSetProperty(g_capture.encoder, kVTCompressionPropertyKey_ProfileLevel,
        kVTProfileLevel_H264_Baseline_AutoLevel);

    int bitrate = 5000000;  // 5 Mbps
    CFNumberRef bitrateNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &bitrate);
    VTSessionSetProperty(g_capture.encoder, kVTCompressionPropertyKey_AverageBitRate, bitrateNum);
    CFRelease(bitrateNum);

    int fps = 60;
    CFNumberRef fpsNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &fps);
    VTSessionSetProperty(g_capture.encoder, kVTCompressionPropertyKey_ExpectedFrameRate, fpsNum);
    CFRelease(fpsNum);

    VTCompressionSessionPrepareToEncodeFrames(g_capture.encoder);

    g_capture.running = 1;
    NSLog(@"Display capture initialized: %ux%u, surface ID %u", g_capture.width, g_capture.height, g_capture.surfaceID);

    return 0;
}

// Capture and encode current frame
int captureFrame(int64_t pts, int64_t duration) {
    if (!g_capture.running || g_capture.encoder == NULL) {
        return -1;
    }

    // Lookup IOSurface
    IOSurfaceRef surface = IOSurfaceLookup(g_capture.surfaceID);
    if (surface == NULL) {
        NSLog(@"IOSurface %u no longer available", g_capture.surfaceID);
        return -2;
    }

    // Create CVPixelBuffer from IOSurface (zero-copy)
    CVPixelBufferRef pixelBuffer = NULL;
    OSStatus status = CVPixelBufferCreateWithIOSurface(
        kCFAllocatorDefault,
        surface,
        NULL,
        &pixelBuffer);

    CFRelease(surface);

    if (status != noErr || pixelBuffer == NULL) {
        NSLog(@"Failed to create pixel buffer: %d", (int)status);
        return -3;
    }

    // Encode
    CMTime presentationTime = CMTimeMake(pts, 1000000000);
    CMTime frameDuration = CMTimeMake(duration, 1000000000);

    status = VTCompressionSessionEncodeFrame(
        g_capture.encoder,
        pixelBuffer,
        presentationTime,
        frameDuration,
        NULL,
        NULL,
        NULL);

    CVPixelBufferRelease(pixelBuffer);

    if (status != noErr) {
        NSLog(@"Encode failed: %d", (int)status);
        return -4;
    }

    g_capture.frameCount++;
    return 0;
}

// Stop capture
void stopDisplayCapture(void) {
    if (g_capture.encoder != NULL) {
        VTCompressionSessionCompleteFrames(g_capture.encoder, kCMTimeInvalid);
        VTCompressionSessionInvalidate(g_capture.encoder);
        CFRelease(g_capture.encoder);
        g_capture.encoder = NULL;
    }
    g_capture.running = 0;
    NSLog(@"Display capture stopped after %lld frames", g_capture.frameCount);
}

// Get frame count
int64_t getCaptureFrameCount(void) {
    return g_capture.frameCount;
}

// Get capture dimensions
void getCaptureDimensions(uint32_t* width, uint32_t* height) {
    *width = g_capture.width;
    *height = g_capture.height;
}

// Callback - for now just count bytes
static int64_t g_totalBytes = 0;
static int64_t g_keyframes = 0;

void captureOutputCallback(void *outputCallbackRefCon,
                           void *sourceFrameRefCon,
                           OSStatus status,
                           VTEncodeInfoFlags infoFlags,
                           CMSampleBufferRef sampleBuffer) {
    if (status != noErr || sampleBuffer == NULL) {
        return;
    }

    CMBlockBufferRef dataBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
    if (dataBuffer != NULL) {
        size_t length = CMBlockBufferGetDataLength(dataBuffer);
        g_totalBytes += length;
    }

    // Check if keyframe
    CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, false);
    if (attachments != NULL && CFArrayGetCount(attachments) > 0) {
        CFDictionaryRef attachment = CFArrayGetValueAtIndex(attachments, 0);
        CFBooleanRef notSync = NULL;
        if (CFDictionaryGetValueIfPresent(attachment, kCMSampleAttachmentKey_NotSync, (const void**)&notSync)) {
            if (notSync != kCFBooleanTrue) {
                g_keyframes++;
            }
        } else {
            g_keyframes++;
        }
    }
}

int64_t getTotalEncodedBytes(void) {
    return g_totalBytes;
}

int64_t getKeyframeCount(void) {
    return g_keyframes;
}

*/
import "C"
import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Println("=== Display Capture Test ===")
	fmt.Println("Looking for UTM display IOSurface...")

	var width, height C.uint32_t
	surfaceID := C.findUTMDisplaySurface(&width, &height)

	if surfaceID == 0 {
		fmt.Println("❌ No suitable display IOSurface found")
		fmt.Println("")
		fmt.Println("Make sure:")
		fmt.Println("1. UTM is running with a Linux VM")
		fmt.Println("2. GPU acceleration is enabled")
		fmt.Println("3. The VM is displaying something")
		os.Exit(1)
	}

	fmt.Printf("✓ Found display surface: ID=%d, %dx%d\n", surfaceID, width, height)
	fmt.Println("")

	// Initialize capture
	result := C.initDisplayCapture(surfaceID)
	if result != 0 {
		fmt.Printf("❌ Failed to initialize capture: %d\n", result)
		os.Exit(1)
	}

	fmt.Println("✓ VideoToolbox hardware encoder initialized")
	fmt.Println("")
	fmt.Println("Capturing frames... Press Ctrl+C to stop")
	fmt.Println("")

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Capture loop at 60 FPS
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	startTime := time.Now()
	lastReport := startTime

	go func() {
		<-sigChan
		ticker.Stop()
	}()

	running := true
	for running {
		select {
		case <-ticker.C:
			pts := time.Since(startTime).Nanoseconds()
			duration := int64(time.Second / 60)

			result := C.captureFrame(C.int64_t(pts), C.int64_t(duration))
			if result != 0 {
				fmt.Printf("Capture error: %d\n", result)
				running = false
				break
			}

			// Report every second
			if time.Since(lastReport) >= time.Second {
				frames := int64(C.getCaptureFrameCount())
				bytes := int64(C.getTotalEncodedBytes())
				keyframes := int64(C.getKeyframeCount())
				elapsed := time.Since(startTime).Seconds()

				fps := float64(frames) / elapsed
				bitrate := float64(bytes*8) / elapsed / 1000000 // Mbps

				fmt.Printf("Frames: %d | FPS: %.1f | Bitrate: %.2f Mbps | Keyframes: %d\n",
					frames, fps, bitrate, keyframes)
				lastReport = time.Now()
			}

		case <-sigChan:
			running = false
		}
	}

	fmt.Println("")
	fmt.Println("Stopping capture...")
	C.stopDisplayCapture()

	// Final stats
	frames := int64(C.getCaptureFrameCount())
	bytes := int64(C.getTotalEncodedBytes())
	elapsed := time.Since(startTime).Seconds()

	fmt.Println("")
	fmt.Println("=== Results ===")
	fmt.Printf("Total frames: %d\n", frames)
	fmt.Printf("Total encoded: %.2f MB\n", float64(bytes)/1000000)
	fmt.Printf("Average FPS: %.1f\n", float64(frames)/elapsed)
	fmt.Printf("Average bitrate: %.2f Mbps\n", float64(bytes*8)/elapsed/1000000)
	fmt.Println("")
	fmt.Println("✓ VideoToolbox hardware encoding works!")
}
