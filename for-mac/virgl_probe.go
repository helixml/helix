package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -ldl

#include <dlfcn.h>
#include <stdio.h>
#include <stdint.h>

// virglrenderer types (from virglrenderer.h)
struct virgl_renderer_resource_info {
   uint32_t handle;
   uint32_t virgl_format;
   uint32_t width;
   uint32_t height;
   uint32_t depth;
   uint32_t flags;
   uint32_t tex_id;
   uint32_t stride;
   int drm_fourcc;
};

// Native handle types
enum virgl_renderer_native_handle_type {
    VIRGL_NATIVE_HANDLE_NONE = 0,
    VIRGL_NATIVE_HANDLE_D3D_TEX2D = 1,
    VIRGL_NATIVE_HANDLE_METAL_TEXTURE = 2,
};

// Extended resource info (from UTM 5.0's virglrenderer)
struct virgl_renderer_resource_info_ext {
    int version;
    struct virgl_renderer_resource_info base;
    bool has_dmabuf_export;
    int planes;
    uint64_t modifiers;
    void* native_handle;  // Metal texture on macOS
    enum virgl_renderer_native_handle_type native_type;
};

// Function pointer types
typedef void* (*virgl_renderer_borrow_texture_fn)(int res_handle);
typedef int (*virgl_renderer_resource_get_info_fn)(int res_handle, struct virgl_renderer_resource_info *info);
typedef int (*virgl_renderer_resource_get_info_ext_fn)(int res_handle, struct virgl_renderer_resource_info_ext *info);
typedef int (*virgl_renderer_get_fd_for_texture_fn)(uint32_t tex_id, int *fd);

// Global handles
static void* g_virgl_handle = NULL;
static virgl_renderer_borrow_texture_fn g_borrow_texture = NULL;
static virgl_renderer_resource_get_info_fn g_get_resource_info = NULL;
static virgl_renderer_resource_get_info_ext_fn g_get_resource_info_ext = NULL;
static virgl_renderer_get_fd_for_texture_fn g_get_fd_for_texture = NULL;

// Load UTM's virglrenderer
int loadVirglRenderer(const char* path, const char* frameworkDir) {
    // First load the dependency (epoxy)
    char epoxyPath[512];
    snprintf(epoxyPath, sizeof(epoxyPath), "%s/epoxy.0.framework/epoxy.0", frameworkDir);
    void* epoxy = dlopen(epoxyPath, RTLD_NOW | RTLD_GLOBAL);
    if (!epoxy) {
        printf("Failed to load epoxy: %s\n", dlerror());
        // Continue anyway, might work
    } else {
        printf("Loaded epoxy successfully\n");
    }

    g_virgl_handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (!g_virgl_handle) {
        printf("Failed to load virglrenderer: %s\n", dlerror());
        return -1;
    }

    // Try to find the functions we need
    g_borrow_texture = (virgl_renderer_borrow_texture_fn)dlsym(g_virgl_handle, "virgl_renderer_borrow_texture_for_scanout");
    g_get_resource_info = (virgl_renderer_resource_get_info_fn)dlsym(g_virgl_handle, "virgl_renderer_resource_get_info");
    g_get_resource_info_ext = (virgl_renderer_resource_get_info_ext_fn)dlsym(g_virgl_handle, "virgl_renderer_resource_get_info_ext");
    g_get_fd_for_texture = (virgl_renderer_get_fd_for_texture_fn)dlsym(g_virgl_handle, "virgl_renderer_get_fd_for_texture");

    printf("virglrenderer loaded successfully\n");
    printf("  borrow_texture_for_scanout: %p\n", (void*)g_borrow_texture);
    printf("  resource_get_info: %p\n", (void*)g_get_resource_info);
    printf("  resource_get_info_ext: %p\n", (void*)g_get_resource_info_ext);
    printf("  get_fd_for_texture: %p\n", (void*)g_get_fd_for_texture);

    return 0;
}

// List all exported symbols
void listSymbols(const char* path) {
    // We'll do this from Go using nm command instead
}

int isVirglLoaded() {
    return g_virgl_handle != NULL;
}

void unloadVirglRenderer() {
    if (g_virgl_handle) {
        dlclose(g_virgl_handle);
        g_virgl_handle = NULL;
    }
}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"strings"
)

func probeVirglRenderer() {
	frameworkPath := "/Applications/UTM.app/Contents/Frameworks/virglrenderer.1.framework/virglrenderer.1"

	fmt.Println("=== Probing UTM's virglrenderer ===")
	fmt.Printf("Framework path: %s\n\n", frameworkPath)

	// List all symbols using nm
	fmt.Println("=== Exported symbols ===")
	cmd := exec.Command("nm", "-gU", frameworkPath)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("nm failed: %v\n", err)
	} else {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, " T _") {
				// Extract just the symbol name
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					symbol := strings.TrimPrefix(parts[2], "_")
					fmt.Printf("  %s\n", symbol)
				}
			}
		}
	}

	fmt.Println("\n=== Attempting to load library ===")
	cPath := C.CString(frameworkPath)
	cFrameworkDir := C.CString("/Applications/UTM.app/Contents/Frameworks")
	result := C.loadVirglRenderer(cPath, cFrameworkDir)

	if result == 0 {
		fmt.Println("\n=== Analysis ===")
		fmt.Println("virglrenderer loaded successfully!")
		fmt.Println("")
		fmt.Println("Key findings:")
		fmt.Println("- virgl_renderer_borrow_texture_for_scanout: Returns texture for display scanout")
		fmt.Println("- virgl_renderer_get_fd_for_texture: Exports texture to FD (DMA-BUF on Linux)")
		fmt.Println("")
		fmt.Println("However, these functions require an active virglrenderer context,")
		fmt.Println("which is created by QEMU when a VM starts.")
		fmt.Println("")
		fmt.Println("To use this:")
		fmt.Println("1. We need to run within UTM's process space, OR")
		fmt.Println("2. UTM needs to expose an IPC mechanism for resource lookup")

		C.unloadVirglRenderer()
	}
}

func main() {
	probeVirglRenderer()
}
