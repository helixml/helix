package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Metal -framework IOSurface

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#import <IOSurface/IOSurface.h>

// virglrenderer types and functions
// These would come from linking against UTM's virglrenderer framework
// For now, we define the interface we need

// Native handle types from virglrenderer.h
typedef enum {
    VIRGL_NATIVE_HANDLE_NONE = 0,
    VIRGL_NATIVE_HANDLE_D3D_TEX2D = 1,
    VIRGL_NATIVE_HANDLE_METAL_TEXTURE = 2,
} virgl_renderer_native_handle_type;

// Resource info structure
typedef struct {
    uint32_t handle;
    uint32_t virgl_format;
    uint32_t width;
    uint32_t height;
    uint32_t depth;
    uint32_t flags;
    uint32_t tex_id;
    uint32_t stride;
    int drm_fourcc;
} virgl_renderer_resource_info;

typedef struct {
    virgl_renderer_resource_info base;
    void* native_handle;  // MTLTexture* on macOS
    virgl_renderer_native_handle_type native_type;
} virgl_renderer_resource_info_ext;

// Function pointer type for virglrenderer lookup
// This will be resolved at runtime from UTM's virglrenderer framework
typedef int (*virgl_renderer_resource_get_info_ext_fn)(
    int res_handle,
    virgl_renderer_resource_info_ext *info
);

// Global function pointer - set when we connect to UTM's virglrenderer
static virgl_renderer_resource_get_info_ext_fn g_virgl_get_info_ext = NULL;

// Set the virglrenderer function pointer
void setVirglRendererGetInfoExt(void* fn) {
    g_virgl_get_info_ext = (virgl_renderer_resource_get_info_ext_fn)fn;
}

// Check if virglrenderer is available
int isVirglRendererAvailable(void) {
    return g_virgl_get_info_ext != NULL;
}

// Get IOSurface for a virtio-gpu resource ID
// Returns IOSurfaceRef or NULL on failure
IOSurfaceRef getIOSurfaceForResource(uint32_t resourceID) {
    if (g_virgl_get_info_ext == NULL) {
        NSLog(@"virglrenderer not initialized");
        return NULL;
    }

    virgl_renderer_resource_info_ext info = {0};
    int ret = g_virgl_get_info_ext(resourceID, &info);
    if (ret != 0) {
        NSLog(@"Failed to get resource info for %u: %d", resourceID, ret);
        return NULL;
    }

    if (info.native_type != VIRGL_NATIVE_HANDLE_METAL_TEXTURE) {
        NSLog(@"Resource %u is not a Metal texture (type=%d)", resourceID, info.native_type);
        return NULL;
    }

    if (info.native_handle == NULL) {
        NSLog(@"Resource %u has NULL native handle", resourceID);
        return NULL;
    }

    // Cast to MTLTexture and get IOSurface
    id<MTLTexture> texture = (__bridge id<MTLTexture>)info.native_handle;
    IOSurfaceRef surface = texture.iosurface;

    if (surface == NULL) {
        NSLog(@"MTLTexture for resource %u has no IOSurface backing", resourceID);
        return NULL;
    }

    // Retain the IOSurface before returning
    CFRetain(surface);
    return surface;
}

// Alternative: Look up IOSurface by global ID
// UTM passes IOSurfaceIDs between processes for display
// We can potentially intercept these
IOSurfaceRef lookupIOSurfaceByID(uint32_t surfaceID) {
    return IOSurfaceLookup(surfaceID);
}

// Helper to check if IOSurface is null
static inline int isIOSurfaceNull(IOSurfaceRef surface) {
    return surface == NULL;
}

// Get resource dimensions
static int getResourceDimensions(uint32_t resourceID, uint32_t* width, uint32_t* height) {
    if (g_virgl_get_info_ext == NULL) {
        return -1;
    }

    virgl_renderer_resource_info_ext info = {0};
    int ret = g_virgl_get_info_ext(resourceID, &info);
    if (ret != 0) {
        return ret;
    }

    *width = info.base.width;
    *height = info.base.height;
    return 0;
}

*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// VirglRenderer provides access to virglrenderer's resource management
type VirglRenderer struct {
	initialized bool
	mu          sync.RWMutex
}

// Global virglrenderer instance
var virglRenderer = &VirglRenderer{}

// InitVirglRenderer initializes the virglrenderer connection
// This needs to be called after UTM's QEMU process starts and we have
// access to the virglrenderer context
func InitVirglRenderer(getInfoExtFn unsafe.Pointer) error {
	virglRenderer.mu.Lock()
	defer virglRenderer.mu.Unlock()

	if virglRenderer.initialized {
		return nil
	}

	C.setVirglRendererGetInfoExt(getInfoExtFn)
	virglRenderer.initialized = true
	return nil
}

// IsVirglRendererAvailable checks if virglrenderer is available
func IsVirglRendererAvailable() bool {
	return C.isVirglRendererAvailable() != 0
}

// GetIOSurfaceForResource returns the IOSurface backing a virtio-gpu resource
func GetIOSurfaceForResource(resourceID uint32) (unsafe.Pointer, error) {
	surface := C.getIOSurfaceForResource(C.uint32_t(resourceID))
	if C.isIOSurfaceNull(surface) != 0 {
		return nil, fmt.Errorf("failed to get IOSurface for resource %d", resourceID)
	}
	return unsafe.Pointer(surface), nil
}

// LookupIOSurfaceByID looks up an IOSurface by its global ID
// This is an alternative path when we receive IOSurfaceIDs directly
func LookupIOSurfaceByID(surfaceID uint32) (unsafe.Pointer, error) {
	surface := C.lookupIOSurfaceByID(C.uint32_t(surfaceID))
	if C.isIOSurfaceNull(surface) != 0 {
		return nil, fmt.Errorf("failed to lookup IOSurface with ID %d", surfaceID)
	}
	return unsafe.Pointer(surface), nil
}

// GetResourceDimensions returns the width and height of a resource
func GetResourceDimensions(resourceID uint32) (width, height uint32, err error) {
	var w, h C.uint32_t
	ret := C.getResourceDimensions(C.uint32_t(resourceID), &w, &h)
	if ret != 0 {
		return 0, 0, fmt.Errorf("failed to get dimensions for resource %d", resourceID)
	}
	return uint32(w), uint32(h), nil
}

// ResourceToIOSurfaceID converts a virtio-gpu resource ID to an IOSurface ID
// This is the main entry point for the encoding pipeline
func ResourceToIOSurfaceID(resourceID uint32) (uint32, error) {
	surface, err := GetIOSurfaceForResource(resourceID)
	if err != nil {
		return 0, err
	}
	defer C.CFRelease(C.CFTypeRef(surface))

	surfaceID := C.IOSurfaceGetID(C.IOSurfaceRef(surface))
	return uint32(surfaceID), nil
}

// EncodeResourceWithVideoToolbox encodes a virtio-gpu resource using VideoToolbox
// This is the high-level API that combines resource lookup and encoding
func EncodeResourceWithVideoToolbox(encoder *VideoToolboxEncoder, resourceID uint32, pts, duration int64) error {
	// Get IOSurface for the resource
	surfaceID, err := ResourceToIOSurfaceID(resourceID)
	if err != nil {
		return fmt.Errorf("failed to get IOSurface for resource %d: %w", resourceID, err)
	}

	// Encode using VideoToolbox
	return encoder.EncodeIOSurface(surfaceID, pts, duration)
}

// ConnectToUTMVirglRenderer attempts to connect to UTM's virglrenderer
// This is called when we detect that UTM is running a VM
func ConnectToUTMVirglRenderer() error {
	// TODO: Implementation depends on how we integrate with UTM
	// Options:
	// 1. Load virglrenderer.framework from UTM.app and get function pointers
	// 2. Use XPC to communicate with QEMUHelper
	// 3. Modify UTM to expose an API for resource lookup

	// For now, check if UTM's virglrenderer framework is loadable
	// Path: /Applications/UTM.app/Contents/Frameworks/virglrenderer.0.framework

	return fmt.Errorf("UTM virglrenderer connection not yet implemented - requires UTM integration")
}
