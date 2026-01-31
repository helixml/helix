package system

import (
	"bufio"
	"io"
	"strings"
	"sync"
	"time"
)

// LogEntry represents a single log line with metadata
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // ERROR, WARN, INFO, DEBUG
	Message   string    `json:"message"`
	Source    string    `json:"source"` // stdout, stderr
}

// ModelInstanceLogBuffer is a thread-safe circular buffer for storing log entries
// for a specific model instance with enhanced functionality
type ModelInstanceLogBuffer struct {
	entries  []LogEntry
	maxLines int
	head     int  // Next position to write
	full     bool // Whether buffer has wrapped around
	mu       sync.RWMutex

	// Model instance metadata
	SlotID    string    `json:"slot_id"`
	ModelID   string    `json:"model_id"`
	CreatedAt time.Time `json:"created_at"`
	LastError *string   `json:"last_error,omitempty"`
	Status    string    `json:"status"` // running, errored, stopped
}

// NewModelInstanceLogBuffer creates a new log buffer for a model instance
func NewModelInstanceLogBuffer(slotID, modelID string, maxLines int) *ModelInstanceLogBuffer {
	if maxLines <= 0 {
		maxLines = 1000 // Default to 1000 lines
	}

	return &ModelInstanceLogBuffer{
		entries:   make([]LogEntry, maxLines),
		maxLines:  maxLines,
		head:      0,
		full:      false,
		SlotID:    slotID,
		ModelID:   modelID,
		CreatedAt: time.Now(),
		Status:    "running",
	}
}

// Write implements io.Writer interface for capturing logs
func (b *ModelInstanceLogBuffer) Write(p []byte) (n int, err error) {
	// Parse log lines and add them as entries
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			b.AddEntry(LogEntry{
				Timestamp: time.Now(),
				Level:     b.extractLogLevel(line),
				Message:   line,
				Source:    "stderr", // Default to stderr since most process logs go there
			})
		}
	}
	return len(p), nil
}

// AddEntry adds a log entry to the circular buffer
func (b *ModelInstanceLogBuffer) AddEntry(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.maxLines

	if b.head == 0 {
		b.full = true
	}

	// Update last error if this is an error entry
	if entry.Level == "ERROR" {
		errorMsg := entry.Message
		b.LastError = &errorMsg
		b.Status = "errored"
	}
}

// GetLogs returns the most recent log entries
func (b *ModelInstanceLogBuffer) GetLogs(maxLines int, since *time.Time, level string) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []LogEntry

	// Determine the starting position and number of entries
	var start, count int
	if b.full {
		start = b.head
		count = b.maxLines
	} else {
		start = 0
		count = b.head
	}

	// Collect entries in chronological order
	for i := 0; i < count; i++ {
		idx := (start + i) % b.maxLines
		entry := b.entries[idx]

		// Apply filters
		if since != nil && entry.Timestamp.Before(*since) {
			continue
		}
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}

		result = append(result, entry)
	}

	// Limit results if requested
	if maxLines > 0 && len(result) > maxLines {
		result = result[len(result)-maxLines:]
	}

	return result
}

// GetMetadata returns metadata about this model instance
func (b *ModelInstanceLogBuffer) GetMetadata() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	metadata := map[string]interface{}{
		"slot_id":    b.SlotID,
		"model_id":   b.ModelID,
		"created_at": b.CreatedAt,
		"status":     b.Status,
	}

	if b.LastError != nil {
		metadata["last_error"] = *b.LastError
	}

	return metadata
}

// SetStatus updates the status of the model instance
func (b *ModelInstanceLogBuffer) SetStatus(status string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Status = status
}

// extractLogLevel attempts to extract log level from log message
func (b *ModelInstanceLogBuffer) extractLogLevel(message string) string {
	upperMsg := strings.ToUpper(message)

	if strings.Contains(upperMsg, "ERROR") || strings.Contains(upperMsg, "FATAL") {
		return "ERROR"
	}
	if strings.Contains(upperMsg, "WARN") {
		return "WARN"
	}
	if strings.Contains(upperMsg, "INFO") {
		return "INFO"
	}
	if strings.Contains(upperMsg, "DEBUG") {
		return "DEBUG"
	}

	// Default level
	return "INFO"
}

// CreateMultiWriter creates a writer that writes to both the original destination and the log buffer
func (b *ModelInstanceLogBuffer) CreateMultiWriter(originalWriter io.Writer) io.Writer {
	return io.MultiWriter(originalWriter, b)
}
