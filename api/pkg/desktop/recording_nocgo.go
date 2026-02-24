//go:build !cgo || !linux

package desktop

import "fmt"

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
	Subtitles []Subtitle `json:"subtitles"`
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
type RecordingManager struct {
	sessionID string
	nodeID    uint32
}

// NewRecordingManager creates a new RecordingManager for a session.
func NewRecordingManager(sessionID string, nodeID uint32) *RecordingManager {
	return &RecordingManager{
		sessionID: sessionID,
		nodeID:    nodeID,
	}
}

// StartRecording is a stub for non-CGO builds.
func (m *RecordingManager) StartRecording(title string) (*Recording, error) {
	return nil, fmt.Errorf("recording not supported without CGO")
}

// StopRecording is a stub for non-CGO builds.
func (m *RecordingManager) StopRecording() (*RecordingResult, error) {
	return nil, fmt.Errorf("recording not supported without CGO")
}

// AddSubtitle is a stub for non-CGO builds.
func (m *RecordingManager) AddSubtitle(text string, startMs, endMs int64) error {
	return fmt.Errorf("recording not supported without CGO")
}

// SetSubtitles is a stub for non-CGO builds.
func (m *RecordingManager) SetSubtitles(subtitles []Subtitle) error {
	return fmt.Errorf("recording not supported without CGO")
}

// GetStatus returns the current recording status.
func (m *RecordingManager) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"recording": false,
		"error":     "recording not supported without CGO",
	}
}

// IsRecording returns true if there's an active recording.
func (m *RecordingManager) IsRecording() bool {
	return false
}

// GetActiveRecordingID returns the ID of the active recording, or empty string if none.
func (m *RecordingManager) GetActiveRecordingID() string {
	return ""
}
