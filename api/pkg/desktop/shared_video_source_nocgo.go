//go:build !cgo

// Package desktop provides shared video source stubs for non-CGO builds.
// The actual implementation is in shared_video_source.go (CGO build only).
package desktop

import (
	"fmt"
)

// SharedVideoSource is a stub for non-CGO builds.
// Video streaming requires CGO for GStreamer bindings.
type SharedVideoSource struct {
	nodeID uint32
}

// SharedVideoSourceRegistry is a stub for non-CGO builds.
type SharedVideoSourceRegistry struct{}

var sharedVideoRegistry *SharedVideoSourceRegistry

// GetSharedVideoRegistry returns the singleton registry (stub).
func GetSharedVideoRegistry() *SharedVideoSourceRegistry {
	if sharedVideoRegistry == nil {
		sharedVideoRegistry = &SharedVideoSourceRegistry{}
	}
	return sharedVideoRegistry
}

// GetOrCreate returns nil for non-CGO builds.
func (r *SharedVideoSourceRegistry) GetOrCreate(nodeID uint32, pipelineStr string, opts GstPipelineOptions) *SharedVideoSource {
	fmt.Printf("[SHARED_VIDEO] GetOrCreate stub called (no CGO)\n")
	return nil
}

// ScheduleStop is a no-op for non-CGO builds.
func (r *SharedVideoSourceRegistry) ScheduleStop(nodeID uint32) {}

// Remove is a no-op for non-CGO builds.
func (r *SharedVideoSourceRegistry) Remove(nodeID uint32) {}

// Shutdown is a no-op for non-CGO builds.
func (r *SharedVideoSourceRegistry) Shutdown() {}

// SourceStats contains statistics for a single shared video source.
type SourceStats struct {
	NodeID         uint32              `json:"node_id"`
	Running        bool                `json:"running"`
	PendingStop    bool                `json:"pending_stop"`
	ClientCount    int                 `json:"client_count"`
	FramesReceived uint64              `json:"frames_received"`
	FramesDropped  uint64              `json:"frames_dropped"`
	GOPBufferSize  int                 `json:"gop_buffer_size"`
	Clients        []ClientBufferStats `json:"clients"`
}

// RegistryStats contains overall registry metrics.
type RegistryStats struct {
	ActiveSources  int   `json:"active_sources"`
	PendingStops   int   `json:"pending_stops"`
	CancelledStops int64 `json:"cancelled_stops"`
	CompletedStops int64 `json:"completed_stops"`
	GracePeriodMs  int64 `json:"grace_period_ms"`
}

// GetRegistryStats returns empty stats for non-CGO builds.
func (r *SharedVideoSourceRegistry) GetRegistryStats() RegistryStats {
	return RegistryStats{}
}

// GetAllStats returns empty stats for non-CGO builds.
func (r *SharedVideoSourceRegistry) GetAllStats() []SourceStats {
	return nil
}

// Subscribe returns an error for non-CGO builds.
func (s *SharedVideoSource) Subscribe() (<-chan VideoFrame, uint64, error) {
	return nil, 0, fmt.Errorf("video streaming requires CGO")
}

// Unsubscribe is a no-op for non-CGO builds.
func (s *SharedVideoSource) Unsubscribe(clientID uint64) {}

// IsRunning returns false for non-CGO builds.
func (s *SharedVideoSource) IsRunning() bool {
	return false
}

// GetClientCount returns 0 for non-CGO builds.
func (s *SharedVideoSource) GetClientCount() int {
	return 0
}

// GetFrameStats returns 0 for non-CGO builds.
func (s *SharedVideoSource) GetFrameStats() (received, dropped uint64) {
	return 0, 0
}

// ClientBufferStats contains buffer statistics for a single client.
type ClientBufferStats struct {
	ClientID   uint64 `json:"client_id"`
	BufferUsed int    `json:"buffer_used"`
	BufferSize int    `json:"buffer_size"`
	BufferPct  int    `json:"buffer_pct"`
}

// GetClientBufferStats returns empty stats for non-CGO builds.
func (s *SharedVideoSource) GetClientBufferStats() []ClientBufferStats {
	return nil
}
