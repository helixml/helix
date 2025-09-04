package system

import (
	"sync"
	"time"
)

// LogManager manages log buffers for multiple model instances
type LogManager struct {
	buffers        map[string]*ModelInstanceLogBuffer // slotID -> buffer
	recentErrors   map[string]*ModelInstanceLogBuffer // slotID -> buffer (for errored instances)
	mu             sync.RWMutex
	maxLines       int
	errorRetention time.Duration // How long to keep errored instance logs
}

// NewLogManager creates a new log manager
func NewLogManager(maxLines int, errorRetention time.Duration) *LogManager {
	if maxLines <= 0 {
		maxLines = 1000
	}
	if errorRetention <= 0 {
		errorRetention = time.Hour // Default 1 hour retention for errored instances
	}

	lm := &LogManager{
		buffers:        make(map[string]*ModelInstanceLogBuffer),
		recentErrors:   make(map[string]*ModelInstanceLogBuffer),
		maxLines:       maxLines,
		errorRetention: errorRetention,
	}

	// Start cleanup goroutine for expired error logs
	go lm.cleanupExpiredErrors()

	return lm
}

// CreateBuffer creates a new log buffer for a model instance
func (lm *LogManager) CreateBuffer(slotID, modelID string) *ModelInstanceLogBuffer {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	buffer := NewModelInstanceLogBuffer(slotID, modelID, lm.maxLines)
	lm.buffers[slotID] = buffer

	return buffer
}

// GetBuffer returns the log buffer for a slot ID
func (lm *LogManager) GetBuffer(slotID string) *ModelInstanceLogBuffer {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	return lm.buffers[slotID]
}

// RemoveBuffer removes a log buffer and moves it to recent errors if it errored
func (lm *LogManager) RemoveBuffer(slotID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if buffer, exists := lm.buffers[slotID]; exists {
		// If the instance had errors, keep it in recent errors for a while
		if buffer.Status == "errored" || buffer.LastError != nil {
			buffer.SetStatus("stopped")
			lm.recentErrors[slotID] = buffer
		}

		delete(lm.buffers, slotID)
	}
}

// GetActiveBuffers returns all currently active log buffers
func (lm *LogManager) GetActiveBuffers() map[string]*ModelInstanceLogBuffer {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make(map[string]*ModelInstanceLogBuffer)
	for k, v := range lm.buffers {
		result[k] = v
	}
	return result
}

// GetRecentErrors returns recently errored instances
func (lm *LogManager) GetRecentErrors() map[string]*ModelInstanceLogBuffer {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make(map[string]*ModelInstanceLogBuffer)
	for k, v := range lm.recentErrors {
		result[k] = v
	}
	return result
}

// GetAllBuffers returns both active and recently errored buffers
func (lm *LogManager) GetAllBuffers() map[string]*ModelInstanceLogBuffer {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	result := make(map[string]*ModelInstanceLogBuffer)
	for k, v := range lm.buffers {
		result[k] = v
	}
	for k, v := range lm.recentErrors {
		result[k] = v
	}
	return result
}

// cleanupExpiredErrors removes old error logs periodically
func (lm *LogManager) cleanupExpiredErrors() {
	ticker := time.NewTicker(10 * time.Minute) // Check every 10 minutes
	defer ticker.Stop()

	for range ticker.C {
		lm.mu.Lock()
		now := time.Now()

		for slotID, buffer := range lm.recentErrors {
			if now.Sub(buffer.CreatedAt) > lm.errorRetention {
				delete(lm.recentErrors, slotID)
			}
		}

		lm.mu.Unlock()
	}
}

// GetLogsSummary returns a summary of all log buffers for dashboard
func (lm *LogManager) GetLogsSummary() map[string]interface{} {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	activeCount := len(lm.buffers)
	errorCount := len(lm.recentErrors)

	// Count instances with recent errors
	recentErrorCount := 0
	for _, buffer := range lm.buffers {
		if buffer.LastError != nil {
			recentErrorCount++
		}
	}

	return map[string]interface{}{
		"active_instances":      activeCount,
		"recent_errors":         errorCount,
		"instances_with_errors": recentErrorCount,
		"max_lines_per_buffer":  lm.maxLines,
		"error_retention_hours": lm.errorRetention.Hours(),
	}
}
