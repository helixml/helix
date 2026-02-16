// drm-test-client is a simple test client for helix-drm-manager.
// Usage: drm-test-client request [width height]
//        drm-test-client release <scanout_id>
package main

import (
	"fmt"
	"os"
	"strconv"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: drm-test-client request [width height]")
		fmt.Println("       drm-test-client release <scanout_id>")
		os.Exit(1)
	}

	client := drmmanager.NewClient("/run/helix-drm/drm.sock")

	switch os.Args[1] {
	case "request":
		width, height := uint32(1920), uint32(1080)
		if len(os.Args) >= 4 {
			w, _ := strconv.Atoi(os.Args[2])
			h, _ := strconv.Atoi(os.Args[3])
			width, height = uint32(w), uint32(h)
		}
		result, err := client.RequestLease(width, height)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Lease granted!\n")
		fmt.Printf("  Scanout ID:  %d\n", result.ScanoutID)
		fmt.Printf("  Connector:   %s\n", result.ConnectorName)
		fmt.Printf("  Lease FD:    %d\n", result.LeaseFD)
		// Keep the FD open briefly so we can verify
		fmt.Printf("  (closing lease FD)\n")
		// In a real container, we'd pass this FD to Mutter

	case "release":
		if len(os.Args) < 3 {
			fmt.Println("Usage: drm-test-client release <scanout_id>")
			os.Exit(1)
		}
		id, _ := strconv.Atoi(os.Args[2])
		if err := client.ReleaseLease(uint32(id)); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Released scanout %d\n", id)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
