package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework IOSurface -framework CoreGraphics

#import <Foundation/Foundation.h>
#import <IOSurface/IOSurface.h>
#import <CoreGraphics/CoreGraphics.h>

// Probe for active IOSurfaces
// IOSurface global IDs are 32-bit integers, typically in a range
// We can try to lookup known IDs to find active surfaces

typedef struct {
    uint32_t id;
    uint32_t width;
    uint32_t height;
    uint32_t bytesPerElement;
    uint32_t pixelFormat;
    size_t allocSize;
} IOSurfaceInfo;

// Try to find IOSurface by ID
int probeIOSurface(uint32_t surfaceID, IOSurfaceInfo* info) {
    IOSurfaceRef surface = IOSurfaceLookup(surfaceID);
    if (surface == NULL) {
        return -1;
    }

    info->id = IOSurfaceGetID(surface);
    info->width = (uint32_t)IOSurfaceGetWidth(surface);
    info->height = (uint32_t)IOSurfaceGetHeight(surface);
    info->bytesPerElement = (uint32_t)IOSurfaceGetBytesPerElement(surface);
    info->pixelFormat = IOSurfaceGetPixelFormat(surface);
    info->allocSize = IOSurfaceGetAllocSize(surface);

    CFRelease(surface);
    return 0;
}

// Scan a range of IDs looking for surfaces
// Returns count of found surfaces
int scanIOSurfaces(uint32_t startID, uint32_t count, IOSurfaceInfo* results, int maxResults) {
    int found = 0;
    for (uint32_t id = startID; id < startID + count && found < maxResults; id++) {
        IOSurfaceRef surface = IOSurfaceLookup(id);
        if (surface != NULL) {
            results[found].id = id;
            results[found].width = (uint32_t)IOSurfaceGetWidth(surface);
            results[found].height = (uint32_t)IOSurfaceGetHeight(surface);
            results[found].bytesPerElement = (uint32_t)IOSurfaceGetBytesPerElement(surface);
            results[found].pixelFormat = IOSurfaceGetPixelFormat(surface);
            results[found].allocSize = IOSurfaceGetAllocSize(surface);
            CFRelease(surface);
            found++;
        }
    }
    return found;
}

// Get pixel format as string
const char* pixelFormatToString(uint32_t format) {
    switch (format) {
        case 'BGRA': return "BGRA (32-bit)";
        case 'ARGB': return "ARGB (32-bit)";
        case 'RGBA': return "RGBA (32-bit)";
        case '420v': return "420v (NV12 video)";
        case '420f': return "420f (NV12 full range)";
        case 'L008': return "L008 (8-bit luma)";
        default: {
            static char buf[32];
            // Print as 4 chars if printable
            if ((format >> 24) >= 32 && (format >> 24) < 127 &&
                ((format >> 16) & 0xFF) >= 32 && ((format >> 16) & 0xFF) < 127 &&
                ((format >> 8) & 0xFF) >= 32 && ((format >> 8) & 0xFF) < 127 &&
                (format & 0xFF) >= 32 && (format & 0xFF) < 127) {
                snprintf(buf, sizeof(buf), "'%c%c%c%c'",
                         (char)(format >> 24),
                         (char)((format >> 16) & 0xFF),
                         (char)((format >> 8) & 0xFF),
                         (char)(format & 0xFF));
            } else {
                snprintf(buf, sizeof(buf), "0x%08x", format);
            }
            return buf;
        }
    }
}

*/
import "C"
import (
	"fmt"
	"os/exec"
	"strings"
)

func main() {
	fmt.Println("=== IOSurface Probe ===")
	fmt.Println("Scanning for active IOSurfaces...")
	fmt.Println("")

	// Check if UTM is running
	cmd := exec.Command("pgrep", "-x", "UTM")
	output, _ := cmd.Output()
	if len(strings.TrimSpace(string(output))) == 0 {
		fmt.Println("⚠️  UTM is not running. Start UTM with a VM to see its IOSurfaces.")
		fmt.Println("")
	} else {
		fmt.Println("✓ UTM is running (PID: " + strings.TrimSpace(string(output)) + ")")
		fmt.Println("")
	}

	// Scan for IOSurfaces
	// IOSurface IDs typically start low and increment
	// Scan a reasonable range
	const maxResults = 100
	results := make([]C.IOSurfaceInfo, maxResults)

	// Scan different ranges
	ranges := []struct {
		start uint32
		count uint32
		name  string
	}{
		{1, 1000, "Low range (1-1000)"},
		{10000, 1000, "Mid range (10000-11000)"},
		{100000, 1000, "High range (100000-101000)"},
	}

	totalFound := 0

	for _, r := range ranges {
		found := int(C.scanIOSurfaces(C.uint32_t(r.start), C.uint32_t(r.count), &results[0], C.int(maxResults)))
		if found > 0 {
			fmt.Printf("%s: Found %d surfaces\n", r.name, found)
			for i := 0; i < found; i++ {
				info := results[i]
				pixelFormat := C.GoString(C.pixelFormatToString(info.pixelFormat))
				fmt.Printf("  ID=%d: %dx%d, %d bpp, format=%s, size=%d bytes\n",
					info.id, info.width, info.height,
					info.bytesPerElement*8, pixelFormat, info.allocSize)

				// Check if this looks like a VM display (typical resolutions)
				if info.width >= 800 && info.height >= 600 {
					fmt.Printf("    ^ This could be a VM display!\n")
				}
			}
			totalFound += found
		}
	}

	if totalFound == 0 {
		fmt.Println("No IOSurfaces found. This is expected if no apps are using GPU surfaces.")
		fmt.Println("")
		fmt.Println("To test:")
		fmt.Println("1. Start UTM with a Linux VM (GPU acceleration enabled)")
		fmt.Println("2. Run this probe again")
	} else {
		fmt.Printf("\nTotal: %d IOSurfaces found\n", totalFound)
		fmt.Println("")
		fmt.Println("Analysis:")
		fmt.Println("- Large surfaces (1920x1080, 2560x1440, etc.) are likely displays")
		fmt.Println("- Smaller surfaces may be textures or intermediate buffers")
		fmt.Println("- BGRA format surfaces can be directly encoded with VideoToolbox")
	}
}
