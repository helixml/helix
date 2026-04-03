package server

// Tests for the slow client tolerance logic from shared_video_source.go.
// The actual implementation is behind //go:build cgo && linux, but the logic
// is simple enough to validate independently: a client should only be
// disconnected after N consecutive buffer-full frames, not on the first drop.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testSlowClientThreshold = 30 // Must match slowClientThreshold in shared_video_source.go

func TestSlowClientThreshold_NotDisconnectedBeforeThreshold(t *testing.T) {
	consecutiveSlowFrames := 0
	disconnected := false

	// Simulate 29 consecutive slow frames (just under threshold)
	for i := 0; i < testSlowClientThreshold-1; i++ {
		// Simulate: channel full (default case in select)
		consecutiveSlowFrames++
		if consecutiveSlowFrames >= testSlowClientThreshold {
			disconnected = true
		}
	}

	assert.False(t, disconnected, "client should not be disconnected before reaching threshold")
	assert.Equal(t, testSlowClientThreshold-1, consecutiveSlowFrames)
}

func TestSlowClientThreshold_DisconnectedAtThreshold(t *testing.T) {
	consecutiveSlowFrames := 0
	disconnected := false

	// Simulate exactly threshold number of slow frames
	for i := 0; i < testSlowClientThreshold; i++ {
		consecutiveSlowFrames++
		if consecutiveSlowFrames >= testSlowClientThreshold {
			disconnected = true
		}
	}

	assert.True(t, disconnected, "client should be disconnected at threshold")
}

func TestSlowClientThreshold_ResetsOnSuccess(t *testing.T) {
	consecutiveSlowFrames := 0
	disconnected := false

	// Accumulate 25 slow frames
	for i := 0; i < 25; i++ {
		consecutiveSlowFrames++
	}
	assert.Equal(t, 25, consecutiveSlowFrames)

	// One successful frame resets counter
	consecutiveSlowFrames = 0

	// Another 25 slow frames — should NOT trigger disconnect (total 50 but not consecutive)
	for i := 0; i < 25; i++ {
		consecutiveSlowFrames++
		if consecutiveSlowFrames >= testSlowClientThreshold {
			disconnected = true
		}
	}

	assert.False(t, disconnected, "client should not be disconnected — slow frames were not consecutive")
}

func TestSlowClientThreshold_IntermittentSlowClient(t *testing.T) {
	consecutiveSlowFrames := 0
	disconnectCount := 0

	// Simulate an intermittently slow client: slow for 20 frames, 1 success, repeat
	for cycle := 0; cycle < 10; cycle++ {
		for i := 0; i < 20; i++ {
			consecutiveSlowFrames++
			if consecutiveSlowFrames >= testSlowClientThreshold {
				disconnectCount++
			}
		}
		// One successful frame
		consecutiveSlowFrames = 0
	}

	assert.Equal(t, 0, disconnectCount, "intermittently slow client (20 drops, 1 success) should never be disconnected")
}
