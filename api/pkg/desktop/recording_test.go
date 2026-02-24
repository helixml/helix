//go:build cgo && linux

package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatVTTTime(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{
			name:     "zero",
			ms:       0,
			expected: "00:00:00.000",
		},
		{
			name:     "one second",
			ms:       1000,
			expected: "00:00:01.000",
		},
		{
			name:     "one minute",
			ms:       60000,
			expected: "00:01:00.000",
		},
		{
			name:     "one hour",
			ms:       3600000,
			expected: "01:00:00.000",
		},
		{
			name:     "complex time",
			ms:       3723456,
			expected: "01:02:03.456",
		},
		{
			name:     "milliseconds only",
			ms:       123,
			expected: "00:00:00.123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatVTTTime(tt.ms)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateWebVTT(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "recording-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	recording := &Recording{
		ID:      "test-recording",
		Title:   "Test Recording",
		vttPath: filepath.Join(tmpDir, "test.vtt"),
		Subtitles: []Subtitle{
			{Text: "Hello, world!", StartMs: 0, EndMs: 2000},
			{Text: "This is a test.", StartMs: 2500, EndMs: 5000},
			{Text: "Goodbye!", StartMs: 5500, EndMs: 7000},
		},
	}

	err = recording.generateWebVTT()
	require.NoError(t, err)

	// Read and verify the generated VTT file
	content, err := os.ReadFile(recording.vttPath)
	require.NoError(t, err)

	vtt := string(content)

	// Check WebVTT header
	assert.True(t, strings.HasPrefix(vtt, "WEBVTT\n\n"), "should start with WEBVTT header")

	// Check cue 1
	assert.Contains(t, vtt, "1\n")
	assert.Contains(t, vtt, "00:00:00.000 --> 00:00:02.000\n")
	assert.Contains(t, vtt, "Hello, world!\n")

	// Check cue 2
	assert.Contains(t, vtt, "2\n")
	assert.Contains(t, vtt, "00:00:02.500 --> 00:00:05.000\n")
	assert.Contains(t, vtt, "This is a test.\n")

	// Check cue 3
	assert.Contains(t, vtt, "3\n")
	assert.Contains(t, vtt, "00:00:05.500 --> 00:00:07.000\n")
	assert.Contains(t, vtt, "Goodbye!\n")
}

func TestSubtitleStruct(t *testing.T) {
	sub := Subtitle{
		Text:    "Test subtitle",
		StartMs: 1000,
		EndMs:   3000,
	}

	assert.Equal(t, "Test subtitle", sub.Text)
	assert.Equal(t, int64(1000), sub.StartMs)
	assert.Equal(t, int64(3000), sub.EndMs)
}

func TestRecordingResultStruct(t *testing.T) {
	result := RecordingResult{
		RecordingID:   "rec_12345",
		Title:         "Demo Recording",
		DurationMs:    45000,
		FrameCount:    1350,
		VideoPath:     "/tmp/helix-recordings/ses_abc/rec_12345/recording.mp4",
		SubtitlesPath: "/tmp/helix-recordings/ses_abc/rec_12345/recording.vtt",
	}

	assert.Equal(t, "rec_12345", result.RecordingID)
	assert.Equal(t, "Demo Recording", result.Title)
	assert.Equal(t, int64(45000), result.DurationMs)
	assert.Equal(t, 1350, result.FrameCount)
	assert.Contains(t, result.VideoPath, "recording.mp4")
	assert.Contains(t, result.SubtitlesPath, "recording.vtt")
}

func TestRecordingManagerNewRecordingManager(t *testing.T) {
	manager := NewRecordingManager("ses_test123", 42)

	assert.NotNil(t, manager)
	assert.Equal(t, "ses_test123", manager.sessionID)
	assert.Equal(t, uint32(42), manager.nodeID)
	assert.False(t, manager.IsRecording())
	assert.Equal(t, "", manager.GetActiveRecordingID())
}

func TestRecordingManagerGetStatus(t *testing.T) {
	manager := NewRecordingManager("ses_test", 1)

	status := manager.GetStatus()
	assert.False(t, status["recording"].(bool))
}
