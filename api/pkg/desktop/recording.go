//go:build cgo && linux

// Package desktop provides recording functionality for AI agent screencasts.
// RecordingManager captures video from SharedVideoSource and creates MP4 files
// with optional WebVTT subtitles for narration.
package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Subtitle represents a single subtitle entry with timing information.
type Subtitle struct {
	Text    string `json:"text"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
}

// Recording represents an active or completed recording session.
type Recording struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	StartTime time.Time  `json:"start_time"`
	EndTime   time.Time  `json:"end_time,omitempty"`
	Subtitles []Subtitle `json:"subtitles"`

	// Internal state
	outputDir    string
	rawH264Path  string
	mp4Path      string
	vttPath      string
	frameCount   int
	file         *os.File
	clientID     uint64
	frameCh      <-chan VideoFrame
	errorCh      <-chan error
	source       *SharedVideoSource
	cancel       context.CancelFunc
	done         chan struct{}
	mu           sync.Mutex
	durationMs   int64
	lastFrameErr error
}

// RecordingResult is returned when a recording is stopped.
type RecordingResult struct {
	RecordingID   string `json:"recording_id"`
	Title         string `json:"title"`
	DurationMs    int64  `json:"duration_ms"`
	FrameCount    int    `json:"frame_count"`
	VideoPath     string `json:"video_path"`
	SubtitlesPath string `json:"subtitles_path,omitempty"`
}

// RecordingManager manages video recordings for a desktop session.
// It subscribes to SharedVideoSource to capture frames without creating
// additional GPU encoder load.
type RecordingManager struct {
	sessionID string
	nodeID    uint32
	active    *Recording
	mu        sync.Mutex
}

// NewRecordingManager creates a new RecordingManager for a session.
func NewRecordingManager(sessionID string, nodeID uint32) *RecordingManager {
	return &RecordingManager{
		sessionID: sessionID,
		nodeID:    nodeID,
	}
}

// StartRecording begins recording video from the SharedVideoSource.
// Only one recording can be active at a time per session.
func (m *RecordingManager) StartRecording(title string) (*Recording, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil {
		return nil, fmt.Errorf("recording already in progress: %s", m.active.ID)
	}

	// Get the shared video source
	source := GetSharedVideoRegistry().GetExisting(m.nodeID)
	if source == nil {
		return nil, fmt.Errorf("no active video source for node %d", m.nodeID)
	}

	// Create recording directory
	recordingID := "rec_" + uuid.New().String()[:8]
	outputDir := filepath.Join("/tmp/helix-recordings", m.sessionID, recordingID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recording directory: %w", err)
	}

	// Create raw H.264 output file
	rawH264Path := filepath.Join(outputDir, "video.h264")
	file, err := os.Create(rawH264Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create video file: %w", err)
	}

	// Subscribe to the video source
	frameCh, errorCh, clientID, err := source.Subscribe()
	if err != nil {
		file.Close()
		os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to subscribe to video source: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	recording := &Recording{
		ID:          recordingID,
		Title:       title,
		StartTime:   time.Now(),
		Subtitles:   make([]Subtitle, 0),
		outputDir:   outputDir,
		rawH264Path: rawH264Path,
		mp4Path:     filepath.Join(outputDir, "recording.mp4"),
		vttPath:     filepath.Join(outputDir, "recording.vtt"),
		file:        file,
		clientID:    clientID,
		frameCh:     frameCh,
		errorCh:     errorCh,
		source:      source,
		cancel:      cancel,
		done:        make(chan struct{}),
	}

	// Start frame capture goroutine
	go recording.captureFrames(ctx)

	m.active = recording
	fmt.Printf("[RECORDING] Started recording %s for session %s\n", recordingID, m.sessionID)

	return recording, nil
}

// StopRecording stops the active recording, finalizes the MP4, and returns the result.
func (m *RecordingManager) StopRecording() (*RecordingResult, error) {
	m.mu.Lock()
	recording := m.active
	m.active = nil
	m.mu.Unlock()

	if recording == nil {
		return nil, fmt.Errorf("no active recording")
	}

	// Stop capture
	recording.cancel()
	<-recording.done

	// Unsubscribe from video source
	recording.source.Unsubscribe(recording.clientID)

	// Close the raw file
	recording.file.Close()

	recording.EndTime = time.Now()
	recording.durationMs = recording.EndTime.Sub(recording.StartTime).Milliseconds()

	fmt.Printf("[RECORDING] Stopped recording %s: %d frames, %dms\n",
		recording.ID, recording.frameCount, recording.durationMs)

	// Convert raw H.264 to MP4
	if err := recording.convertToMP4(); err != nil {
		return nil, fmt.Errorf("failed to convert to MP4: %w", err)
	}

	// Generate WebVTT if there are subtitles
	if len(recording.Subtitles) > 0 {
		if err := recording.generateWebVTT(); err != nil {
			fmt.Printf("[RECORDING] Warning: failed to generate WebVTT: %v\n", err)
		}
	}

	// Clean up raw H.264 file
	os.Remove(recording.rawH264Path)

	result := &RecordingResult{
		RecordingID: recording.ID,
		Title:       recording.Title,
		DurationMs:  recording.durationMs,
		FrameCount:  recording.frameCount,
		VideoPath:   recording.mp4Path,
	}

	if len(recording.Subtitles) > 0 {
		result.SubtitlesPath = recording.vttPath
	}

	return result, nil
}

// AddSubtitle adds a single subtitle entry to the active recording.
func (m *RecordingManager) AddSubtitle(text string, startMs, endMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active recording")
	}

	m.active.mu.Lock()
	m.active.Subtitles = append(m.active.Subtitles, Subtitle{
		Text:    text,
		StartMs: startMs,
		EndMs:   endMs,
	})
	m.active.mu.Unlock()

	return nil
}

// SetSubtitles replaces the entire subtitle track.
func (m *RecordingManager) SetSubtitles(subtitles []Subtitle) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active recording")
	}

	m.active.mu.Lock()
	m.active.Subtitles = subtitles
	m.active.mu.Unlock()

	return nil
}

// GetStatus returns the current recording status.
func (m *RecordingManager) GetStatus() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return map[string]interface{}{
			"recording": false,
		}
	}

	m.active.mu.Lock()
	defer m.active.mu.Unlock()

	return map[string]interface{}{
		"recording":      true,
		"recording_id":   m.active.ID,
		"title":          m.active.Title,
		"start_time":     m.active.StartTime,
		"duration_ms":    time.Since(m.active.StartTime).Milliseconds(),
		"frame_count":    m.active.frameCount,
		"subtitle_count": len(m.active.Subtitles),
	}
}

// IsRecording returns true if there's an active recording.
func (m *RecordingManager) IsRecording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active != nil
}

// GetActiveRecordingID returns the ID of the active recording, or empty string if none.
func (m *RecordingManager) GetActiveRecordingID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return ""
	}
	return m.active.ID
}

// captureFrames reads frames from the video source and writes them to the raw file.
func (r *Recording) captureFrames(ctx context.Context) {
	defer close(r.done)

	for {
		select {
		case <-ctx.Done():
			return

		case frame, ok := <-r.frameCh:
			if !ok {
				return
			}

			// Skip replay frames (GOP warmup frames shouldn't be recorded)
			if frame.IsReplay {
				continue
			}

			// Write frame data to file
			r.mu.Lock()
			_, err := r.file.Write(frame.Data)
			if err != nil {
				r.lastFrameErr = err
				r.mu.Unlock()
				return
			}
			r.frameCount++
			r.mu.Unlock()

		case err, ok := <-r.errorCh:
			if ok && err != nil {
				r.mu.Lock()
				r.lastFrameErr = err
				r.mu.Unlock()
				fmt.Printf("[RECORDING] Video source error: %v\n", err)
				return
			}
		}
	}
}

// convertToMP4 uses ffmpeg to mux raw H.264 into an MP4 container.
func (r *Recording) convertToMP4() error {
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "h264",
		"-i", r.rawH264Path,
		"-c:v", "copy",
		"-movflags", "+faststart",
		r.mp4Path,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %w, output: %s", err, string(output))
	}

	return nil
}

// generateWebVTT creates a WebVTT subtitle file from the recorded subtitles.
func (r *Recording) generateWebVTT() error {
	file, err := os.Create(r.vttPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write WebVTT header
	file.WriteString("WEBVTT\n\n")

	// Write each subtitle cue
	for i, sub := range r.Subtitles {
		// Format: HH:MM:SS.mmm --> HH:MM:SS.mmm
		startTime := formatVTTTime(sub.StartMs)
		endTime := formatVTTTime(sub.EndMs)

		fmt.Fprintf(file, "%d\n", i+1)
		fmt.Fprintf(file, "%s --> %s\n", startTime, endTime)
		fmt.Fprintf(file, "%s\n\n", sub.Text)
	}

	return nil
}

// formatVTTTime converts milliseconds to WebVTT timestamp format (HH:MM:SS.mmm).
func formatVTTTime(ms int64) string {
	hours := ms / 3600000
	ms %= 3600000
	minutes := ms / 60000
	ms %= 60000
	seconds := ms / 1000
	milliseconds := ms % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds)
}
