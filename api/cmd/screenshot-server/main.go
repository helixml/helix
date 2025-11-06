package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	http.HandleFunc("/clipboard", handleClipboard)
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

func handleClipboard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetClipboard(w, r)
	case http.MethodPost:
		handleSetClipboard(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetClipboard(w http.ResponseWriter, r *http.Request) {
	// Get Wayland display (same logic as screenshot)
	waylandDisplay := getWaylandDisplay()
	if waylandDisplay == "" {
		log.Printf("No Wayland display available for clipboard access")
		// Return empty clipboard instead of error (clipboard might just be empty)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
		return
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// First, detect clipboard MIME types to determine if it's text or image
	listCmd := exec.Command("wl-paste", "--list-types")
	listCmd.Env = append(os.Environ(),
		fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
		fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
	)

	typesOutput, err := listCmd.Output()
	if err != nil {
		// Clipboard is empty or unavailable
		log.Printf("wl-paste --list-types returned error (clipboard may be empty): %v", err)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
		return
	}

	types := string(typesOutput)
	isImage := strings.Contains(types, "image/png") || strings.Contains(types, "image/jpeg")

	if isImage {
		// Clipboard contains an image - retrieve as PNG
		cmd := exec.Command("wl-paste", "-t", "image/png")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)

		imageData, err := cmd.Output()
		if err != nil {
			log.Printf("Failed to retrieve image from clipboard: %v", err)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(""))
			return
		}

		// Return image as base64-encoded JSON
		// Format: {"type":"image","data":"base64..."}
		response := map[string]string{
			"type": "image",
			"data": base64.StdEncoding.EncodeToString(imageData),
		}
		jsonData, _ := json.Marshal(response)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)

		log.Printf("Clipboard image retrieved (%d bytes)", len(imageData))
	} else {
		// Clipboard contains text
		cmd := exec.Command("wl-paste", "--no-newline")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)

		textData, err := cmd.Output()
		if err != nil {
			log.Printf("wl-paste returned error (clipboard may be empty): %v", err)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(""))
			return
		}

		// Return text as JSON
		// Format: {"type":"text","data":"text content"}
		response := map[string]string{
			"type": "text",
			"data": string(textData),
		}
		jsonData, _ := json.Marshal(response)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)

		log.Printf("Clipboard text retrieved (%d bytes)", len(textData))
	}
}

func handleSetClipboard(w http.ResponseWriter, r *http.Request) {
	// Get Wayland display (same logic as screenshot)
	waylandDisplay := getWaylandDisplay()
	if waylandDisplay == "" {
		log.Printf("No Wayland display available for clipboard access")
		http.Error(w, "No Wayland display available", http.StatusInternalServerError)
		return
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// Read request body (JSON with type and data)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse JSON to determine clipboard type
	var clipboardData struct {
		Type string `json:"type"` // "text" or "image"
		Data string `json:"data"` // text content or base64-encoded image
	}
	if err := json.Unmarshal(body, &clipboardData); err != nil {
		log.Printf("Failed to parse clipboard JSON: %v", err)
		http.Error(w, "Invalid clipboard data format", http.StatusBadRequest)
		return
	}

	if clipboardData.Type == "image" {
		// Decode base64 image
		imageBytes, err := base64.StdEncoding.DecodeString(clipboardData.Data)
		if err != nil {
			log.Printf("Failed to decode base64 image: %v", err)
			http.Error(w, "Invalid base64 image data", http.StatusBadRequest)
			return
		}

		// Execute wl-copy with image/png MIME type
		cmd := exec.Command("wl-copy", "-t", "image/png")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmd.Stdin = bytes.NewReader(imageBytes)

		if err := cmd.Run(); err != nil {
			log.Printf("Failed to set image clipboard: %v", err)
			http.Error(w, "Failed to set image clipboard", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		log.Printf("Clipboard image set (%d bytes)", len(imageBytes))
	} else {
		// Text clipboard
		cmd := exec.Command("wl-copy")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		cmd.Stdin = strings.NewReader(clipboardData.Data)

		if err := cmd.Run(); err != nil {
			log.Printf("Failed to set text clipboard: %v", err)
			http.Error(w, "Failed to set clipboard", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		log.Printf("Clipboard text set (%d bytes)", len(clipboardData.Data))
	}
}

// getWaylandDisplay returns the cached or detected Wayland display
func getWaylandDisplay() string {
	// Return cached if available
	if cachedWaylandDisplay != "" {
		return cachedWaylandDisplay
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/sockets"
	}

	// Find all wayland-* sockets
	entries, err := os.ReadDir(xdgRuntimeDir)
	if err != nil {
		log.Printf("Failed to read XDG_RUNTIME_DIR: %v", err)
		return ""
	}

	waylandSockets := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wayland-") && !strings.HasSuffix(entry.Name(), ".lock") {
			waylandSockets = append(waylandSockets, entry.Name())
		}
	}

	if len(waylandSockets) == 0 {
		log.Printf("No Wayland sockets found in %s", xdgRuntimeDir)
		return ""
	}

	// Try each socket with a simple test (wl-paste will fail gracefully if clipboard is empty)
	for _, socket := range waylandSockets {
		cmd := exec.Command("wl-paste", "--list-types")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYLAND_DISPLAY=%s", socket),
			fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
		)
		if err := cmd.Run(); err == nil {
			cachedWaylandDisplay = socket
			log.Printf("Detected Wayland display for clipboard: %s", socket)
			return socket
		}
	}

	// If all failed, just use the first one and hope for the best
	cachedWaylandDisplay = waylandSockets[0]
	log.Printf("Using fallback Wayland display: %s", cachedWaylandDisplay)
	return cachedWaylandDisplay
}