package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WhisperManager handles lazy loading and idle timeout for the Whisper model.
// It manages a whisper-server process that loads on first use and shuts down
// after 5 minutes of inactivity to save GPU memory.
type WhisperManager struct {
	mu            sync.Mutex
	process       *exec.Cmd
	serverURL     string
	lastUsed      time.Time
	idleTimeout   time.Duration
	shutdownTimer *time.Timer
	modelPath     string
	logger        interface{ Info(string, ...any) }
}

// Global whisper manager instance
var whisperManager *WhisperManager
var whisperManagerOnce sync.Once

// getWhisperManager returns the singleton WhisperManager instance.
func getWhisperManager(logger interface{ Info(string, ...any) }) *WhisperManager {
	whisperManagerOnce.Do(func() {
		// Look for model in common locations
		modelPaths := []string{
			"/usr/share/whisper/models/ggml-base.en.bin",
			"/opt/whisper/models/ggml-base.en.bin",
			"/home/helix/.cache/whisper/ggml-base.en.bin",
		}

		var modelPath string
		for _, p := range modelPaths {
			if _, err := os.Stat(p); err == nil {
				modelPath = p
				break
			}
		}

		whisperManager = &WhisperManager{
			serverURL:   "http://127.0.0.1:8178",
			idleTimeout: 5 * time.Minute,
			modelPath:   modelPath,
			logger:      logger,
		}
	})
	return whisperManager
}

// ensureRunning starts the whisper server if not already running.
// Returns the server URL to use for transcription requests.
func (wm *WhisperManager) ensureRunning() (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Reset idle timer
	wm.lastUsed = time.Now()
	if wm.shutdownTimer != nil {
		wm.shutdownTimer.Stop()
	}
	wm.shutdownTimer = time.AfterFunc(wm.idleTimeout, wm.shutdown)

	// Check if already running by pinging health endpoint
	if wm.process != nil && wm.process.Process != nil {
		// Quick health check to verify server is still responsive
		client := &http.Client{Timeout: 500 * time.Millisecond}
		resp, err := client.Get(wm.serverURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return wm.serverURL, nil
			}
		}
		// Server not responding, clean up and restart
		wm.process.Process.Kill()
		wm.process.Wait()
		wm.process = nil
	}

	// Start whisper server
	if wm.modelPath == "" {
		return "", fmt.Errorf("whisper model not found - please install ggml-base.en.bin")
	}

	wm.logger.Info("starting whisper server",
		"model", wm.modelPath,
		"idle_timeout", wm.idleTimeout)

	// Use whisper-server (from whisper.cpp) if available
	cmd := exec.Command("whisper-server",
		"--model", wm.modelPath,
		"--host", "127.0.0.1",
		"--port", "8178",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start whisper server: %w", err)
	}

	wm.process = cmd

	// Wait for server to be ready (poll health endpoint)
	for i := 0; i < 30; i++ { // Up to 30 seconds
		time.Sleep(1 * time.Second)
		resp, err := http.Get(wm.serverURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				wm.logger.Info("whisper server ready", "url", wm.serverURL)
				return wm.serverURL, nil
			}
		}
	}

	// Server didn't start in time, kill it
	if wm.process != nil && wm.process.Process != nil {
		wm.process.Process.Kill()
	}
	wm.process = nil
	return "", fmt.Errorf("whisper server failed to start within 30 seconds")
}

// shutdown stops the whisper server to free GPU memory.
func (wm *WhisperManager) shutdown() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.process != nil && wm.process.Process != nil {
		wm.logger.Info("shutting down whisper server (idle timeout)")
		wm.process.Process.Kill()
		wm.process.Wait()
		wm.process = nil
	}
}

// VoiceResponse is the response from the voice endpoint.
type VoiceResponse struct {
	Text   string `json:"text"`
	Status string `json:"status"` // "success" or "error"
	Error  string `json:"error,omitempty"`
}

// handleVoice handles voice input requests.
// It receives audio in WebM format, converts to WAV, transcribes with Whisper,
// and types the result using wtype (Wayland text input).
func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read audio data
	audioData, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendVoiceError(w, "Failed to read audio data", err)
		return
	}
	defer r.Body.Close()

	if len(audioData) == 0 {
		s.sendVoiceError(w, "Empty audio data", nil)
		return
	}

	s.logger.Info("received voice input", "size", len(audioData))

	// Create temp files for audio conversion
	tmpDir := os.TempDir()
	webmPath := filepath.Join(tmpDir, fmt.Sprintf("voice_%d.webm", time.Now().UnixNano()))
	wavPath := filepath.Join(tmpDir, fmt.Sprintf("voice_%d.wav", time.Now().UnixNano()))
	defer os.Remove(webmPath)
	defer os.Remove(wavPath)

	// Write WebM audio to temp file
	if err := os.WriteFile(webmPath, audioData, 0644); err != nil {
		s.sendVoiceError(w, "Failed to write audio file", err)
		return
	}

	// Convert WebM to WAV using ffmpeg
	// Whisper expects 16kHz mono WAV
	ffmpegCmd := exec.Command("ffmpeg",
		"-y", // Overwrite output
		"-i", webmPath,
		"-ar", "16000", // 16kHz sample rate
		"-ac", "1", // Mono
		"-f", "wav",
		wavPath,
	)
	var ffmpegStderr bytes.Buffer
	ffmpegCmd.Stderr = &ffmpegStderr
	if err := ffmpegCmd.Run(); err != nil {
		s.sendVoiceError(w, fmt.Sprintf("ffmpeg conversion failed: %s", ffmpegStderr.String()), err)
		return
	}

	// Get whisper manager and ensure server is running
	wm := getWhisperManager(s.logger)
	serverURL, err := wm.ensureRunning()
	if err != nil {
		// Fallback: use whisper CLI directly if server won't start
		text, cliErr := s.transcribeWithCLI(wavPath)
		if cliErr != nil {
			s.sendVoiceError(w, "Whisper transcription failed", cliErr)
			return
		}
		s.typeAndRespond(w, text)
		return
	}

	// Send audio to whisper server
	text, err := s.transcribeWithServer(serverURL, wavPath)
	if err != nil {
		s.sendVoiceError(w, "Whisper transcription failed", err)
		return
	}

	s.typeAndRespond(w, text)
}

// transcribeWithServer sends audio to the whisper-server HTTP API.
// whisper.cpp server expects multipart/form-data with a "file" field.
func (s *Server) transcribeWithServer(serverURL, wavPath string) (string, error) {
	// Read WAV file
	wavData, err := os.ReadFile(wavPath)
	if err != nil {
		return "", fmt.Errorf("failed to read WAV file: %w", err)
	}

	// Create multipart form with file field
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(wavPath))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(wavData); err != nil {
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}
	writer.Close()

	// POST to whisper server
	resp, err := http.Post(serverURL+"/inference",
		writer.FormDataContentType(),
		&body)
	if err != nil {
		return "", fmt.Errorf("failed to send audio to whisper: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("whisper server returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (whisper-server returns JSON with "text" field)
	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse whisper response: %w", err)
	}

	return strings.TrimSpace(result.Text), nil
}

// transcribeWithCLI uses the whisper CLI as a fallback.
func (s *Server) transcribeWithCLI(wavPath string) (string, error) {
	// Find model path
	modelPaths := []string{
		"/usr/share/whisper/models/ggml-base.en.bin",
		"/opt/whisper/models/ggml-base.en.bin",
		"/home/helix/.cache/whisper/ggml-base.en.bin",
	}

	var modelPath string
	for _, p := range modelPaths {
		if _, err := os.Stat(p); err == nil {
			modelPath = p
			break
		}
	}

	if modelPath == "" {
		return "", fmt.Errorf("whisper model not found")
	}

	// Run whisper CLI (whisper.cpp)
	// whisper.cpp outputs transcription to stdout
	cmd := exec.Command("whisper",
		"-m", modelPath,
		"-f", wavPath,
		"-nt",  // No timestamps
		"-np",  // No progress output
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper CLI failed: %s", stderr.String())
	}

	// whisper.cpp outputs transcription directly to stdout
	return strings.TrimSpace(stdout.String()), nil
}

// typeAndRespond types the transcribed text and sends the response.
func (s *Server) typeAndRespond(w http.ResponseWriter, text string) {
	if text == "" {
		s.sendVoiceResponse(w, "", "success")
		return
	}

	s.logger.Info("transcribed text", "text", text)

	// Type text using the appropriate method for the compositor
	if err := s.typeText(text); err != nil {
		s.logger.Info("text typing failed", "error", err)
		// Still return success with the text - let frontend handle display
	}

	s.sendVoiceResponse(w, text, "success")
}

// typeText types text using the appropriate method for the compositor.
// - Sway: uses wtype (wlr-virtual-keyboard protocol)
// - GNOME: uses D-Bus RemoteDesktop NotifyKeyboardKeysym
func (s *Server) typeText(text string) error {
	if s.compositorType == "sway" {
		return s.typeWithWtype(text)
	}
	// GNOME: use D-Bus RemoteDesktop
	return s.typeWithDBus(text)
}

// typeWithWtype types text using wtype (Wayland text input utility).
// Works on wlroots compositors (Sway) via wlr-virtual-keyboard.
func (s *Server) typeWithWtype(text string) error {
	cmd := exec.Command("wtype", text)
	cmd.Env = append(os.Environ(),
		"WAYLAND_DISPLAY=wayland-0",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wtype failed: %s", stderr.String())
	}

	return nil
}

// typeWithDBus types text using GNOME RemoteDesktop D-Bus NotifyKeyboardKeysym.
// Each character is sent as a keysym press/release pair.
func (s *Server) typeWithDBus(text string) error {
	if s.conn == nil || s.rdSessionPath == "" {
		return fmt.Errorf("D-Bus RemoteDesktop session not available")
	}

	rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

	for _, r := range text {
		// Convert character to X11 keysym
		// For ASCII (0x20-0x7E), keysym == character code
		// For Unicode, keysym = 0x01000000 + unicode_codepoint
		var keysym uint32
		if r >= 0x20 && r <= 0x7E {
			keysym = uint32(r)
		} else if r == '\n' {
			keysym = 0xff0d // XK_Return
		} else if r == '\t' {
			keysym = 0xff09 // XK_Tab
		} else {
			// Unicode keysym
			keysym = 0x01000000 + uint32(r)
		}

		// Press
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, true).Err; err != nil {
			return fmt.Errorf("keysym press failed for '%c': %w", r, err)
		}

		// Release
		if err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeysym", 0, keysym, false).Err; err != nil {
			return fmt.Errorf("keysym release failed for '%c': %w", r, err)
		}
	}

	return nil
}

// sendVoiceResponse sends a successful voice response.
func (s *Server) sendVoiceResponse(w http.ResponseWriter, text, status string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VoiceResponse{
		Text:   text,
		Status: status,
	})
}

// sendVoiceError sends an error response.
func (s *Server) sendVoiceError(w http.ResponseWriter, message string, err error) {
	s.logger.Info("voice error", "message", message, "error", err)

	errMsg := message
	if err != nil {
		errMsg = fmt.Sprintf("%s: %v", message, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(VoiceResponse{
		Status: "error",
		Error:  errMsg,
	})
}
