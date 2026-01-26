package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CloneProgressWriter parses git progress output and updates database
type CloneProgressWriter struct {
	store     store.Store
	repoID    string
	startedAt time.Time
	mu        sync.Mutex
	lastSave  time.Time
}

// Regex patterns for parsing git progress output
// Examples:
//   "Counting objects:  12% (1/8)"
//   "Receiving objects:  75% (6/8), 1.20 MiB | 500.00 KiB/s"
//   "Resolving deltas: 100% (123/123), done."
var (
	progressPattern = regexp.MustCompile(`(?i)^([\w\s]+):\s*(\d+)%\s*\((\d+)/(\d+)\)(?:,\s*([\d.]+)\s*(\w+)(?:\s*\|\s*([\d.]+)\s*(\w+/s))?)?`)
)

// NewCloneProgressWriter creates a new progress writer for tracking clone progress
func NewCloneProgressWriter(s store.Store, repoID string) *CloneProgressWriter {
	return &CloneProgressWriter{
		store:     s,
		repoID:    repoID,
		startedAt: time.Now(),
	}
}

// Write implements io.Writer to capture git progress output
func (w *CloneProgressWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Also handle carriage return for progress updates that overwrite the line
		line = strings.TrimPrefix(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := progressPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		// Parse progress
		phase := strings.ToLower(strings.TrimSpace(matches[1]))
		percentage, _ := strconv.Atoi(matches[2])
		current, _ := strconv.Atoi(matches[3])
		total, _ := strconv.Atoi(matches[4])

		var bytesReceived int64
		var speed string
		if len(matches) > 5 && matches[5] != "" {
			bytes, _ := strconv.ParseFloat(matches[5], 64)
			unit := matches[6]
			bytesReceived = w.parseBytes(bytes, unit)
			if len(matches) > 7 && matches[7] != "" {
				speed = matches[7] + " " + matches[8]
			}
		}

		// Throttle DB updates to every 500ms to avoid excessive writes
		w.mu.Lock()
		if time.Since(w.lastSave) < 500*time.Millisecond {
			w.mu.Unlock()
			continue
		}
		w.lastSave = time.Now()
		w.mu.Unlock()

		// Update database with progress
		progress := &types.CloneProgress{
			Phase:         phase,
			Percentage:    percentage,
			Current:       current,
			Total:         total,
			BytesReceived: bytesReceived,
			Speed:         speed,
			StartedAt:     w.startedAt,
		}

		// Run update in goroutine to not block git clone
		go func(prog *types.CloneProgress) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := w.store.UpdateGitRepositoryCloneProgress(ctx, w.repoID, prog); err != nil {
				log.Warn().
					Err(err).
					Str("repo_id", w.repoID).
					Str("phase", prog.Phase).
					Int("percentage", prog.Percentage).
					Msg("Failed to update clone progress")
			}
		}(progress)
	}

	return len(p), nil
}

// parseBytes converts a value with unit to bytes
func (w *CloneProgressWriter) parseBytes(value float64, unit string) int64 {
	unit = strings.ToUpper(strings.TrimSpace(unit))
	switch unit {
	case "B", "BYTES":
		return int64(value)
	case "KIB", "KB":
		return int64(value * 1024)
	case "MIB", "MB":
		return int64(value * 1024 * 1024)
	case "GIB", "GB":
		return int64(value * 1024 * 1024 * 1024)
	default:
		return int64(value)
	}
}
