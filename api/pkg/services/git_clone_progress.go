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
// go-git formats (what we actually receive):
//
//	"Enumerating objects: 31867, done."
//	"Counting objects: 100% (31867/31867), done."
//	"Compressing objects: 100% (123/456), done."
//	"Total 31867 (delta 0), reused 0 (delta 0), pack-reused 31867 (from 1)"
//
// Standard git formats:
//
//	"Receiving objects:  75% (6/8), 1.20 MiB | 500.00 KiB/s"
//	"Resolving deltas: 100% (123/123), done."
var (
	// Matches "Phase: X% (current/total)" with optional size and speed
	progressPattern = regexp.MustCompile(`(?i)^([\w\s]+):\s*(\d+)%\s*\((\d+)/(\d+)\)(?:,\s*([\d.]+)\s*(\w+)(?:\s*\|\s*([\d.]+)\s*(\w+/s))?)?`)
	// Matches go-git's "Phase: count, done." format
	goGitPattern = regexp.MustCompile(`(?i)^([\w\s]+):\s*(\d+),?\s*done\.?`)
	// Matches "Total X (delta Y)" summary line
	totalPattern = regexp.MustCompile(`(?i)^Total\s+(\d+)`)
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

		var progress *types.CloneProgress

		// Try standard git progress format: "Phase: X% (current/total)"
		if matches := progressPattern.FindStringSubmatch(line); matches != nil {
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

			progress = &types.CloneProgress{
				Phase:         phase,
				Percentage:    percentage,
				Current:       current,
				Total:         total,
				BytesReceived: bytesReceived,
				Speed:         speed,
				StartedAt:     w.startedAt,
			}
		} else if matches := goGitPattern.FindStringSubmatch(line); matches != nil {
			// go-git format: "Phase: count, done."
			phase := strings.ToLower(strings.TrimSpace(matches[1]))
			count, _ := strconv.Atoi(matches[2])

			progress = &types.CloneProgress{
				Phase:      phase,
				Percentage: 100, // "done" means complete
				Current:    count,
				Total:      count,
				StartedAt:  w.startedAt,
			}
		} else if matches := totalPattern.FindStringSubmatch(line); matches != nil {
			// Total line indicates completion
			total, _ := strconv.Atoi(matches[1])

			progress = &types.CloneProgress{
				Phase:      "receiving",
				Percentage: 100,
				Current:    total,
				Total:      total,
				StartedAt:  w.startedAt,
			}
		}

		if progress == nil {
			// Log unrecognized progress lines for debugging
			log.Debug().
				Str("repo_id", w.repoID).
				Str("line", line).
				Msg("Unrecognized git progress line")
			continue
		}

		log.Debug().
			Str("repo_id", w.repoID).
			Str("phase", progress.Phase).
			Int("percentage", progress.Percentage).
			Int("current", progress.Current).
			Int("total", progress.Total).
			Msg("Git clone progress update")

		// Throttle DB updates to every 500ms to avoid excessive writes
		w.mu.Lock()
		if time.Since(w.lastSave) < 500*time.Millisecond {
			w.mu.Unlock()
			continue
		}
		w.lastSave = time.Now()
		w.mu.Unlock()

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
