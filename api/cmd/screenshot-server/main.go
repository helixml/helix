package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

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

	// Run grim to capture screenshot
	cmd := exec.Command("grim", "-o", filename)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("WAYLAND_DISPLAY=%s", os.Getenv("WAYLAND_DISPLAY")),
		fmt.Sprintf("XDG_RUNTIME_DIR=%s", os.Getenv("XDG_RUNTIME_DIR")),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to capture screenshot: %v, output: %s", err, string(output))
		http.Error(w, fmt.Sprintf("Failed to capture screenshot: %v", err), http.StatusInternalServerError)
		return
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