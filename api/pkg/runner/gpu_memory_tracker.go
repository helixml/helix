package runner

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

// GPUMemoryStatsInternal tracks GPU memory stabilization statistics with mutex for internal use
type GPUMemoryStatsInternal struct {
	mu                       sync.RWMutex
	TotalStabilizations      int                                 `json:"total_stabilizations"`
	SuccessfulStabilizations int                                 `json:"successful_stabilizations"`
	FailedStabilizations     int                                 `json:"failed_stabilizations"`
	LastStabilization        *time.Time                          `json:"last_stabilization,omitempty"`
	RecentEvents             []types.GPUMemoryStabilizationEvent `json:"recent_events"` // Last 20 events

	// Statistics
	AverageWaitTimeSeconds float64 `json:"average_wait_time_seconds"`
	MaxWaitTimeSeconds     int     `json:"max_wait_time_seconds"`
	MinWaitTimeSeconds     int     `json:"min_wait_time_seconds"`

	// Time-series data (last 10 minutes)
	MemoryTimeSeries []types.GPUMemoryDataPoint `json:"memory_time_series"` // Last 10 minutes of memory data
	SchedulingEvents []types.SchedulingEvent    `json:"scheduling_events"`  // Last 10 minutes of scheduling events
}

// GPUMemoryTracker manages GPU memory stabilization tracking
type GPUMemoryTracker struct {
	stats      GPUMemoryStatsInternal
	gpuManager *GPUManager
	ctx        context.Context
	cancel     context.CancelFunc

	// For calculating allocated memory per GPU
	slots *xsync.MapOf[uuid.UUID, *Slot]
}

// NewGPUMemoryTracker creates a new GPU memory tracker
func NewGPUMemoryTracker(ctx context.Context, gpuManager *GPUManager, slots *xsync.MapOf[uuid.UUID, *Slot]) *GPUMemoryTracker {
	trackerCtx, cancel := context.WithCancel(ctx)

	tracker := &GPUMemoryTracker{
		stats: GPUMemoryStatsInternal{
			RecentEvents:     make([]types.GPUMemoryStabilizationEvent, 0, 20),
			MemoryTimeSeries: make([]types.GPUMemoryDataPoint, 0),
			SchedulingEvents: make([]types.SchedulingEvent, 0),
		},
		gpuManager: gpuManager,
		ctx:        trackerCtx,
		cancel:     cancel,
		slots:      slots,
	}

	// Start time-series data collection
	if gpuManager != nil {
		go tracker.startTimeSeriesCollection()
	}

	return tracker
}

// Stop stops the GPU memory tracker
func (gmt *GPUMemoryTracker) Stop() {
	if gmt.cancel != nil {
		gmt.cancel()
	}
}

// startTimeSeriesCollection starts collecting GPU memory data every 10 seconds
func (gmt *GPUMemoryTracker) startTimeSeriesCollection() {
	ticker := time.NewTicker(10 * time.Second) // Collect data every 10 seconds
	defer ticker.Stop()

	log.Info().Msg("GPU_MEMORY_TRACKER: Started time-series data collection")

	for {
		select {
		case <-gmt.ctx.Done():
			log.Info().Msg("GPU_MEMORY_TRACKER: Stopping time-series data collection")
			return
		case <-ticker.C:
			gmt.collectMemoryDataPoint()
		}
	}
}

// collectMemoryDataPoint collects current memory state for all GPUs
func (gmt *GPUMemoryTracker) collectMemoryDataPoint() {
	if gmt.gpuManager == nil {
		return
	}

	now := time.Now()
	gpuInfo := gmt.gpuManager.GetGPUInfo()

	gmt.stats.mu.Lock()
	defer gmt.stats.mu.Unlock()

	// Calculate allocated memory per GPU from slots
	allocatedPerGPU := make(map[int]uint64)
	gmt.slots.Range(func(_ uuid.UUID, slot *Slot) bool {
		if slot.ModelMemoryRequirement > 0 {
			if len(slot.GPUIndices) > 0 {
				// Multi-GPU: split memory across GPUs
				memoryPerGPU := slot.ModelMemoryRequirement / uint64(len(slot.GPUIndices))
				for _, gpuIndex := range slot.GPUIndices {
					allocatedPerGPU[gpuIndex] += memoryPerGPU
				}
			} else if slot.GPUIndex != nil {
				// Single GPU
				allocatedPerGPU[*slot.GPUIndex] += slot.ModelMemoryRequirement
			}
		}
		return true
	})

	// Add data points for each GPU in sorted order to prevent randomness
	// First collect all GPU indices and sort them
	var gpuIndices []int
	for gpuIndex := range gpuInfo {
		gpuIndices = append(gpuIndices, gpuIndex)
	}

	// Sort indices to ensure consistent ordering
	for i := 0; i < len(gpuIndices); i++ {
		for j := i + 1; j < len(gpuIndices); j++ {
			if gpuIndices[i] > gpuIndices[j] {
				gpuIndices[i], gpuIndices[j] = gpuIndices[j], gpuIndices[i]
			}
		}
	}

	// Add data points in sorted order
	for _, gpuIndex := range gpuIndices {
		gpu := gpuInfo[gpuIndex]
		allocated := allocatedPerGPU[gpu.Index]

		dataPoint := types.GPUMemoryDataPoint{
			Timestamp:     now,
			GPUIndex:      gpu.Index,
			AllocatedMB:   allocated / (1024 * 1024),
			ActualUsedMB:  gpu.UsedMemory / (1024 * 1024),
			ActualFreeMB:  gpu.FreeMemory / (1024 * 1024),
			ActualTotalMB: gpu.TotalMemory / (1024 * 1024),
		}

		gmt.stats.MemoryTimeSeries = append(gmt.stats.MemoryTimeSeries, dataPoint)
	}

	// Keep only last 10 minutes of data (60 data points at 10-second intervals)
	cutoff := now.Add(-10 * time.Minute)
	gmt.pruneOldData(cutoff)
}

// pruneOldData removes data older than the cutoff time
func (gmt *GPUMemoryTracker) pruneOldData(cutoff time.Time) {
	// Prune memory time series
	var newTimeSeries []types.GPUMemoryDataPoint
	for _, point := range gmt.stats.MemoryTimeSeries {
		if point.Timestamp.After(cutoff) {
			newTimeSeries = append(newTimeSeries, point)
		}
	}
	gmt.stats.MemoryTimeSeries = newTimeSeries

	// Prune scheduling events
	var newEvents []types.SchedulingEvent
	for _, event := range gmt.stats.SchedulingEvents {
		if event.Timestamp.After(cutoff) {
			newEvents = append(newEvents, event)
		}
	}
	gmt.stats.SchedulingEvents = newEvents
}

// StartStabilization begins tracking a new stabilization event
func (gmt *GPUMemoryTracker) StartStabilization(context, slotID, runtime string, timeoutSeconds, pollIntervalMs, requiredStablePolls int, memoryDeltaThresholdMB uint64) *types.GPUMemoryStabilizationEvent {
	event := &types.GPUMemoryStabilizationEvent{
		Timestamp:              time.Now(),
		Context:                context,
		SlotID:                 slotID,
		Runtime:                runtime,
		TimeoutSeconds:         timeoutSeconds,
		PollIntervalMs:         pollIntervalMs,
		RequiredStablePolls:    requiredStablePolls,
		MemoryDeltaThresholdMB: memoryDeltaThresholdMB,
		MemoryReadings:         make([]types.GPUMemoryReading, 0),
	}

	return event
}

// AddMemoryReading adds a memory reading to the current stabilization event
func (gmt *GPUMemoryTracker) AddMemoryReading(event *types.GPUMemoryStabilizationEvent, pollNumber int, memoryMB uint64, deltaMB int64, stableCount int, isStable bool) {
	reading := types.GPUMemoryReading{
		PollNumber:  pollNumber,
		MemoryMB:    memoryMB,
		DeltaMB:     deltaMB,
		StableCount: stableCount,
		IsStable:    isStable,
	}

	event.MemoryReadings = append(event.MemoryReadings, reading)
}

// CompleteStabilization completes a stabilization event and updates statistics
func (gmt *GPUMemoryTracker) CompleteStabilization(event *types.GPUMemoryStabilizationEvent, success bool, pollsTaken int, stabilizedMemoryMB uint64, errorMessage string) {
	gmt.stats.mu.Lock()
	defer gmt.stats.mu.Unlock()

	// Update the event
	event.Success = success
	event.PollsTaken = pollsTaken
	event.TotalWaitSeconds = pollsTaken * event.PollIntervalMs / 1000
	event.StabilizedMemoryMB = stabilizedMemoryMB
	event.ErrorMessage = errorMessage

	// Update statistics
	gmt.stats.TotalStabilizations++
	if success {
		gmt.stats.SuccessfulStabilizations++
	} else {
		gmt.stats.FailedStabilizations++
	}

	now := time.Now()
	gmt.stats.LastStabilization = &now

	// Update wait time statistics
	waitTime := event.TotalWaitSeconds
	if gmt.stats.TotalStabilizations == 1 {
		gmt.stats.AverageWaitTimeSeconds = float64(waitTime)
		gmt.stats.MaxWaitTimeSeconds = waitTime
		gmt.stats.MinWaitTimeSeconds = waitTime
	} else {
		// Update average
		gmt.stats.AverageWaitTimeSeconds = (gmt.stats.AverageWaitTimeSeconds*float64(gmt.stats.TotalStabilizations-1) + float64(waitTime)) / float64(gmt.stats.TotalStabilizations)

		// Update max/min
		if waitTime > gmt.stats.MaxWaitTimeSeconds {
			gmt.stats.MaxWaitTimeSeconds = waitTime
		}
		if waitTime < gmt.stats.MinWaitTimeSeconds {
			gmt.stats.MinWaitTimeSeconds = waitTime
		}
	}

	// Add to recent events (keep last 20)
	gmt.stats.RecentEvents = append(gmt.stats.RecentEvents, *event)
	if len(gmt.stats.RecentEvents) > 20 {
		gmt.stats.RecentEvents = gmt.stats.RecentEvents[1:]
	}
}

// AddSchedulingEvent adds a scheduling event for correlation with memory usage
func (gmt *GPUMemoryTracker) AddSchedulingEvent(eventType, slotID, modelName, runtime string, gpuIndices []int, memoryMB uint64, description string) {
	gmt.stats.mu.Lock()
	defer gmt.stats.mu.Unlock()

	event := types.SchedulingEvent{
		Timestamp:   time.Now(),
		EventType:   eventType,
		SlotID:      slotID,
		ModelName:   modelName,
		Runtime:     runtime,
		GPUIndices:  gpuIndices,
		MemoryMB:    memoryMB,
		Description: description,
	}

	gmt.stats.SchedulingEvents = append(gmt.stats.SchedulingEvents, event)

	// Prune old events (keep last 10 minutes)
	cutoff := time.Now().Add(-10 * time.Minute)
	gmt.pruneOldData(cutoff)

	log.Debug().
		Str("event_type", eventType).
		Str("slot_id", slotID).
		Str("model_name", modelName).
		Str("runtime", runtime).
		Interface("gpu_indices", gpuIndices).
		Uint64("memory_mb", memoryMB).
		Str("description", description).
		Msg("GPU_MEMORY_TRACKER: Added scheduling event")
}

// GetStats returns the current GPU memory statistics
func (gmt *GPUMemoryTracker) GetStats() types.GPUMemoryStats {
	gmt.stats.mu.RLock()
	defer gmt.stats.mu.RUnlock()

	// Return a copy of the stats to avoid data races
	return types.GPUMemoryStats{
		TotalStabilizations:      gmt.stats.TotalStabilizations,
		SuccessfulStabilizations: gmt.stats.SuccessfulStabilizations,
		FailedStabilizations:     gmt.stats.FailedStabilizations,
		LastStabilization:        gmt.stats.LastStabilization,
		RecentEvents:             append([]types.GPUMemoryStabilizationEvent{}, gmt.stats.RecentEvents...),
		AverageWaitTimeSeconds:   gmt.stats.AverageWaitTimeSeconds,
		MaxWaitTimeSeconds:       gmt.stats.MaxWaitTimeSeconds,
		MinWaitTimeSeconds:       gmt.stats.MinWaitTimeSeconds,
		MemoryTimeSeries:         append([]types.GPUMemoryDataPoint{}, gmt.stats.MemoryTimeSeries...),
		SchedulingEvents:         append([]types.SchedulingEvent{}, gmt.stats.SchedulingEvents...),
	}
}
