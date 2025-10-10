package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var cachedWaylandDisplay string

func main() {
	port := os.Getenv("SCREENSHOT_PORT")
	if port == "" {
		port = "9876"
	}

	http.HandleFunc("/screenshot", handleScreenshot)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Screenshot server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create temporary file for screenshot
	tmpDir := os.TempDir()
	filename := filepath.Join(tmpDir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	defer os.Remove(filename)

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets" // Wolf-UI uses /tmp/sockets not /run/user/wolf
	}

	// Try all available Wayland sockets until one works
	// This handles race conditions where Wolf creates wayland-1 (capture only)
	// and Sway creates wayland-2 (supports screencopy protocol)
	var output []byte
	var err error

	// Try cached display first if available
	if cachedWaylandDisplay != "" {
		cmd := exec.Command("grim", filename)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", cachedWaylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		output, err = cmd.CombinedOutput()
	}

	// If cached failed or doesn't exist, try all sockets
	if err != nil || cachedWaylandDisplay == "" {
		// Find all wayland-* sockets
		entries, readErr := os.ReadDir(xdgRuntimeDir)
		if readErr != nil {
			log.Printf("Failed to read XDG_RUNTIME_DIR: %v", readErr)
			http.Error(w, "Failed to find Wayland sockets", http.StatusInternalServerError)
			return
		}

		waylandSockets := []string{}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "wayland-") && !strings.HasSuffix(entry.Name(), ".lock") {
				waylandSockets = append(waylandSockets, entry.Name())
			}
		}

		if len(waylandSockets) == 0 {
			log.Printf("No Wayland sockets found in %s", xdgRuntimeDir)
			http.Error(w, "No Wayland sockets available", http.StatusInternalServerError)
			return
		}

		log.Printf("Trying Wayland sockets: %v", waylandSockets)

		// Try each socket until one works
		for _, socket := range waylandSockets {
			cmd := exec.Command("grim", filename)
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("WAYLAND_DISPLAY=%s", socket),
				fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
			)
			output, err = cmd.CombinedOutput()
			if err == nil {
				cachedWaylandDisplay = socket // Cache for next time
				log.Printf("Successfully captured screenshot using %s", socket)
				break
			}
			log.Printf("Failed to capture with %s: %v, output: %s", socket, err, string(output))
		}

		if err != nil {
			log.Printf("Failed to capture screenshot with any Wayland socket")
			http.Error(w, fmt.Sprintf("Failed to capture screenshot: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Read the screenshot file
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("Failed to read screenshot file: %v", err)
		http.Error(w, "Failed to read screenshot", http.StatusInternalServerError)
		return
	}

	// Serve the PNG
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(data)

	log.Printf("Screenshot captured successfully (%d bytes)", len(data))
}