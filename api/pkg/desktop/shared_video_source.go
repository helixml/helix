//go:build cgo

// Package desktop provides shared video source for multi-client streaming.
// A SharedVideoSource manages ONE GStreamer pipeline per session and broadcasts
// encoded H.264 frames to all connected WebSocket clients. This prevents resource
// contention when multiple pipewirezerocopysrc instances try to connect to the
// same PipeWire ScreenCast node.
//
// Build: 2026-01-23-gop-replay-vhs-effect
package desktop

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Client state machine constants
const (
	clientStateCatchingUp uint32 = 0 // Receiving GOP replay + buffering live frames
	clientStateLive       uint32 = 1 // Receiving live frames directly
	clientStateClosed     uint32 = 2 // Disconnected
)

// Catchup timeout - if client can't catch up in this time, disconnect
const catchupTimeout = 30 * time.Second

// SharedVideoSource manages a single GStreamer pipeline that broadcasts to multiple clients.
// All clients connected to the same session share this source, preventing the issue where
// multiple pipewirezerocopysrc instances compete for the same PipeWire node.
//
// Key features:
// - ONE pipeline per session (identified by PipeWire node ID)
// - Broadcasts encoded H.264 frames to all subscribers
// - Caches the last keyframe for mid-stream joins
// - Automatically stops when the last client disconnects
type SharedVideoSource struct {
	// Immutable after creation
	nodeID       uint32
	pipelineStr  string
	pipelineOpts GstPipelineOptions

	// Pipeline state
	pipeline  *GstPipeline
	running   atomic.Bool
	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error
	startMu   sync.Mutex // Protects startOnce/startErr

	// Client management
	clients   map[uint64]*sharedVideoClient
	clientsMu sync.RWMutex
	nextID    atomic.Uint64

	// GOP buffer for mid-stream joins
	// Stores all frames since the last keyframe so new clients can decode immediately
	// When a new keyframe arrives, the buffer is reset
	// New clients receive the entire GOP buffer to catch up to the live stream
	gopBuffer   []VideoFrame
	gopBufferMu sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// sharedVideoClient represents a connected client receiving video frames
type sharedVideoClient struct {
	id      uint64
	frameCh chan VideoFrame

	// State machine: catching_up -> live -> closed
	// - catching_up: Receiving GOP replay, broadcaster queues frames to pending
	// - live: Receiving live frames directly to frameCh
	// - closed: Disconnected, cleanup pending
	state atomic.Uint32

	// Pending buffer for frames that arrive during GOP replay
	// Broadcaster adds frames here while state == catching_up
	// Catchup goroutine drains this to frameCh after GOP replay
	pendingMu sync.Mutex
	pending   []VideoFrame
}

// SharedVideoSourceRegistry maintains a map of PipeWire node IDs to shared video sources.
// This ensures only ONE pipeline is created per PipeWire node, regardless of how many
// WebSocket clients connect.
type SharedVideoSourceRegistry struct {
	sources map[uint32]*SharedVideoSource // keyed by PipeWire node ID
	mu      sync.Mutex
}

var (
	sharedVideoRegistry     *SharedVideoSourceRegistry
	sharedVideoRegistryOnce sync.Once
)

// GetSharedVideoRegistry returns the singleton registry
func GetSharedVideoRegistry() *SharedVideoSourceRegistry {
	sharedVideoRegistryOnce.Do(func() {
		sharedVideoRegistry = &SharedVideoSourceRegistry{
			sources: make(map[uint32]*SharedVideoSource),
		}
	})
	return sharedVideoRegistry
}

// GetOrCreate returns an existing SharedVideoSource for the PipeWire node, or creates a new one.
// The pipelineStr and opts are only used when creating a new source.
func (r *SharedVideoSourceRegistry) GetOrCreate(nodeID uint32, pipelineStr string, opts GstPipelineOptions) *SharedVideoSource {
	r.mu.Lock()
	defer r.mu.Unlock()

	if source, exists := r.sources[nodeID]; exists {
		source.clientsMu.RLock()
		clientCount := len(source.clients)
		source.clientsMu.RUnlock()
		fmt.Printf("[SHARED_VIDEO] Reusing existing source for node %d (clients: %d)\n",
			nodeID, clientCount)
		return source
	}

	// Create new shared source
	ctx, cancel := context.WithCancel(context.Background())
	source := &SharedVideoSource{
		nodeID:       nodeID,
		pipelineStr:  pipelineStr,
		pipelineOpts: opts,
		clients:      make(map[uint64]*sharedVideoClient),
		ctx:          ctx,
		cancel:       cancel,
	}

	r.sources[nodeID] = source
	fmt.Printf("[SHARED_VIDEO] Created new source for node %d\n", nodeID)
	return source
}

// Remove removes a SharedVideoSource from the registry.
// Called when the last client disconnects.
func (r *SharedVideoSourceRegistry) Remove(nodeID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if source, exists := r.sources[nodeID]; exists {
		source.stop()
		delete(r.sources, nodeID)
		fmt.Printf("[SHARED_VIDEO] Removed source for node %d\n", nodeID)
	}
}

// SourceStats contains statistics for a single shared video source
type SourceStats struct {
	NodeID        uint32              `json:"node_id"`
	Running       bool                `json:"running"`
	ClientCount   int                 `json:"client_count"`
	FramesReceived uint64             `json:"frames_received"`
	FramesDropped  uint64             `json:"frames_dropped"`
	GOPBufferSize int                 `json:"gop_buffer_size"`
	Clients       []ClientBufferStats `json:"clients"`
}

// GetAllStats returns statistics for all active video sources
func (r *SharedVideoSourceRegistry) GetAllStats() []SourceStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats := make([]SourceStats, 0, len(r.sources))
	for nodeID, source := range r.sources {
		received, dropped := source.GetFrameStats()

		source.gopBufferMu.RLock()
		gopLen := len(source.gopBuffer)
		source.gopBufferMu.RUnlock()

		stats = append(stats, SourceStats{
			NodeID:         nodeID,
			Running:        source.IsRunning(),
			ClientCount:    source.GetClientCount(),
			FramesReceived: received,
			FramesDropped:  dropped,
			GOPBufferSize:  gopLen,
			Clients:        source.GetClientBufferStats(),
		})
	}
	return stats
}

// Subscribe registers a new client to receive video frames.
// Returns a channel for receiving frames and a client ID for unsubscribing.
// If this is the first client, the pipeline is started.
//
// Non-blocking design:
// - Client starts in "catching_up" state
// - Broadcaster queues frames to client.pending instead of channel
// - Catchup goroutine runs in background: replays GOP, drains pending, transitions to live
// - One slow client does NOT block other clients
func (s *SharedVideoSource) Subscribe() (<-chan VideoFrame, uint64, error) {
	clientID := s.nextID.Add(1)

	// Create client with buffered channel sized to GOP
	// Buffer equals GOP size so client can buffer a full keyframe interval
	// If buffer fills, client is disconnected (not frame-dropped) to prevent decoder corruption
	// Client starts in catching_up state (0 is the zero value for atomic.Uint32)
	bufferSize := getDefaultGOPSize()
	client := &sharedVideoClient{
		id:      clientID,
		frameCh: make(chan VideoFrame, bufferSize),
		// state is 0 (clientStateCatchingUp) by default
		// pending starts nil, will be populated by broadcaster
	}

	// Check current client count
	s.clientsMu.RLock()
	existingClients := len(s.clients)
	s.clientsMu.RUnlock()

	// Start pipeline on first client
	if existingClients == 0 {
		// First client - add to map, start pipeline, no catchup needed (no GOP yet)
		// Set state to live directly since there's nothing to catch up on
		client.state.Store(clientStateLive)

		s.clientsMu.Lock()
		s.clients[clientID] = client
		s.clientsMu.Unlock()

		fmt.Printf("[SHARED_VIDEO] Client %d subscribed to node %d (first client, starting pipeline)\n", clientID, s.nodeID)

		if err := s.start(); err != nil {
			s.clientsMu.Lock()
			delete(s.clients, clientID)
			s.clientsMu.Unlock()
			close(client.frameCh)
			return nil, 0, fmt.Errorf("start pipeline: %w", err)
		}
	} else {
		// Subsequent client - needs catchup
		// Check if pipeline had a start error
		s.startMu.Lock()
		err := s.startErr
		s.startMu.Unlock()
		if err != nil {
			close(client.frameCh)
			return nil, 0, fmt.Errorf("pipeline error: %w", err)
		}

		// Add client to map with state=catching_up
		// Broadcaster will start queuing frames to pending buffer immediately
		s.clientsMu.Lock()
		s.clients[clientID] = client
		clientCount := len(s.clients)
		s.clientsMu.Unlock()

		fmt.Printf("[SHARED_VIDEO] Client %d subscribed to node %d (total: %d, starting catchup)\n",
			clientID, s.nodeID, clientCount)

		// Start catchup goroutine in background
		// This will: 1) copy GOP buffer, 2) send to channel, 3) drain pending, 4) transition to live
		// Non-blocking - broadcaster continues sending frames (they go to pending buffer)
		go s.runCatchup(client)
	}

	return client.frameCh, clientID, nil
}

// disconnectClient forcefully disconnects a slow client by closing their channel.
// This is called by broadcastFrames when a client's buffer is full or pending overflow.
// Uses CAS to ensure only one goroutine closes the channel (prevents double-close panic).
// The client's WebSocket handler will see the closed channel and clean up.
func (s *SharedVideoSource) disconnectClient(clientID uint64) {
	s.clientsMu.Lock()
	client, exists := s.clients[clientID]
	if !exists {
		s.clientsMu.Unlock()
		return
	}

	// Try CAS from catching_up → closed
	if client.state.CompareAndSwap(clientStateCatchingUp, clientStateClosed) {
		close(client.frameCh)
		client.pendingMu.Lock()
		client.pending = nil
		client.pendingMu.Unlock()
		s.clientsMu.Unlock()
		return
	}

	// Try CAS from live → closed
	if client.state.CompareAndSwap(clientStateLive, clientStateClosed) {
		close(client.frameCh)
		s.clientsMu.Unlock()
		return
	}

	// Already closed - nothing to do
	s.clientsMu.Unlock()
}

// Unsubscribe removes a client from the video source.
// If this is the last client, the pipeline is stopped and the source is removed from the registry.
// Uses CAS to ensure channel is closed exactly once.
func (s *SharedVideoSource) Unsubscribe(clientID uint64) {
	s.clientsMu.Lock()
	client, exists := s.clients[clientID]
	if exists {
		delete(s.clients, clientID)

		// Use CAS to close channel - only the winner closes it
		// Try from catching_up first, then live
		if client.state.CompareAndSwap(clientStateCatchingUp, clientStateClosed) {
			close(client.frameCh)
			client.pendingMu.Lock()
			client.pending = nil
			client.pendingMu.Unlock()
		} else if client.state.CompareAndSwap(clientStateLive, clientStateClosed) {
			close(client.frameCh)
		}
		// If already closed, CAS fails and we don't double-close
	}
	remaining := len(s.clients)
	s.clientsMu.Unlock()

	if exists {
		fmt.Printf("[SHARED_VIDEO] Client %d unsubscribed from node %d (remaining: %d)\n",
			clientID, s.nodeID, remaining)
	}

	// If no more clients, stop the pipeline and remove from registry
	if remaining == 0 {
		GetSharedVideoRegistry().Remove(s.nodeID)
	}
}

// runCatchup runs the GOP replay and pending buffer drain for a catching-up client.
// This runs in a separate goroutine to avoid blocking the broadcaster.
// Guaranteed to terminate within catchupTimeout (30 seconds) or on client disconnect.
//
// State machine transitions (all use CAS):
// - catching_up → live: on successful catchup (pending drained)
// - catching_up → closed: on timeout or external disconnect
func (s *SharedVideoSource) runCatchup(client *sharedVideoClient) {
	timeout := time.After(catchupTimeout)
	startTime := time.Now()

	// Phase 1: Get GOP buffer snapshot
	// We hold gopBufferMu briefly to copy, then release
	s.gopBufferMu.RLock()
	gopCopy := make([]VideoFrame, len(s.gopBuffer))
	copy(gopCopy, s.gopBuffer)
	s.gopBufferMu.RUnlock()

	fmt.Printf("[SHARED_VIDEO] Client %d catchup started: %d GOP frames to replay\n",
		client.id, len(gopCopy))

	// Phase 2: Send GOP frames to client channel (marked as replay for decoder warmup)
	for i, frame := range gopCopy {
		// Check if client was closed externally
		if client.state.Load() == clientStateClosed {
			fmt.Printf("[SHARED_VIDEO] Client %d catchup aborted: client closed\n", client.id)
			return
		}

		// Mark as replay frame so frontend can show VHS/fast-forward effect
		replayFrame := frame
		replayFrame.IsReplay = true

		select {
		case client.frameCh <- replayFrame:
			// Frame sent successfully
		case <-timeout:
			fmt.Printf("[SHARED_VIDEO] Client %d catchup timeout at GOP frame %d/%d\n",
				client.id, i, len(gopCopy))
			s.disconnectClient(client.id)
			return
		}
	}

	// Phase 3: Drain pending buffer until empty, then transition to live
	// CRITICAL: We must hold pendingMu while checking empty AND doing CAS
	// This ensures no frame is added to pending after we check but before we go live
	drainedCount := 0
	for {
		// Check if client was closed externally
		if client.state.Load() == clientStateClosed {
			fmt.Printf("[SHARED_VIDEO] Client %d catchup aborted: client closed during drain\n", client.id)
			return
		}

		// Lock pending, check if empty, transition if so
		client.pendingMu.Lock()
		if len(client.pending) == 0 {
			// Pending is empty - transition to live while holding lock
			// This prevents broadcaster from adding to pending after our check
			if client.state.CompareAndSwap(clientStateCatchingUp, clientStateLive) {
				client.pending = nil // Release pending buffer memory
				client.pendingMu.Unlock()
				elapsed := time.Since(startTime)
				fmt.Printf("[SHARED_VIDEO] Client %d catchup complete: %d GOP + %d pending frames in %v\n",
					client.id, len(gopCopy), drainedCount, elapsed)
				return
			}
			// CAS failed - client was closed by someone else
			client.pendingMu.Unlock()
			fmt.Printf("[SHARED_VIDEO] Client %d catchup: CAS to live failed (closed externally)\n", client.id)
			return
		}

		// Take first pending frame
		frame := client.pending[0]
		client.pending = client.pending[1:]
		client.pendingMu.Unlock()

		drainedCount++

		// Send to channel with timeout
		select {
		case client.frameCh <- frame:
			// Frame sent successfully
		case <-timeout:
			fmt.Printf("[SHARED_VIDEO] Client %d catchup timeout draining pending (drained %d)\n",
				client.id, drainedCount)
			s.disconnectClient(client.id)
			return
		}
	}
}

// start initializes and starts the GStreamer pipeline
func (s *SharedVideoSource) start() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	var startErr error
	s.startOnce.Do(func() {
		fmt.Printf("[SHARED_VIDEO] Starting pipeline for node %d\n", s.nodeID)
		fmt.Printf("[SHARED_VIDEO] Pipeline: %s\n", s.pipelineStr)

		// Create GStreamer pipeline
		var err error
		s.pipeline, err = NewGstPipelineWithOptions(s.pipelineStr, s.pipelineOpts)
		if err != nil {
			startErr = fmt.Errorf("create pipeline: %w", err)
			return
		}

		// Start the pipeline
		if err = s.pipeline.Start(s.ctx); err != nil {
			startErr = fmt.Errorf("start pipeline: %w", err)
			return
		}

		s.running.Store(true)

		// Start broadcaster goroutine
		s.wg.Add(1)
		go s.broadcastFrames()
	})

	s.startErr = startErr
	return startErr
}

// broadcastFrames reads frames from the pipeline and sends to all subscribed clients
func (s *SharedVideoSource) broadcastFrames() {
	defer s.wg.Done()

	frameCh := s.pipeline.Frames()
	var frameCount uint64
	var keyframeCount uint64

	for {
		select {
		case <-s.ctx.Done():
			fmt.Printf("[SHARED_VIDEO] Broadcast stopped (context cancelled) for node %d\n", s.nodeID)
			return
		case frame, ok := <-frameCh:
			if !ok {
				// Pipeline stopped
				fmt.Printf("[SHARED_VIDEO] Pipeline channel closed for node %d\n", s.nodeID)
				return
			}

			frameCount++

			// Maintain GOP buffer for mid-stream joins
			// On keyframe: reset buffer and start fresh GOP
			// On P-frame: append to current GOP
			s.gopBufferMu.Lock()
			if frame.IsKeyframe {
				keyframeCount++
				// Clear old GOP buffer explicitly to help GC, then start new GOP with keyframe
				oldLen := len(s.gopBuffer)
				s.gopBuffer = nil // Release old slice for GC
				s.gopBuffer = []VideoFrame{frame}
				if keyframeCount <= 3 || keyframeCount%100 == 0 {
					fmt.Printf("[SHARED_VIDEO] New GOP started (keyframe #%d, %d bytes, freed %d frames) for node %d\n",
						keyframeCount, len(frame.Data), oldLen, s.nodeID)
				}
			} else {
				// Append P-frame to current GOP
				// Limit matches GOP size from config (default 1800 = 30s at 60fps)
				// The buffer is cleared on each keyframe anyway
				maxGOPFrames := getDefaultGOPSize()
				if len(s.gopBuffer) < maxGOPFrames {
					s.gopBuffer = append(s.gopBuffer, frame)
				}
			}
			s.gopBufferMu.Unlock()

			// Broadcast to all clients based on their state
			// - catching_up: queue to pending buffer (MUST check state while holding pendingMu!)
			// - live: send directly to channel
			// - closed: skip
			var slowClients []uint64
			var pendingOverflow []uint64
			maxPendingSize := getDefaultGOPSize() * 2 // 2x GOP = overflow threshold

			s.clientsMu.RLock()
			clientCount := len(s.clients)
			for _, client := range s.clients {
				// Quick check for closed - can skip entirely
				if client.state.Load() == clientStateClosed {
					continue
				}

				// CRITICAL: For catching_up clients, we MUST check state while holding pendingMu
				// to avoid race with catchup goroutine's CAS to live.
				// Race scenario without this:
				// 1. Broadcaster loads state=catching_up
				// 2. Catchup: lock pending, CAS to live, pending=nil, unlock
				// 3. Broadcaster: lock pending, append to nil (creates new slice), unlock
				// Result: frame stuck in pending forever, catchup has exited = lost frame
				handled := false
				client.pendingMu.Lock()
				if client.state.Load() == clientStateCatchingUp {
					// Still catching up - add to pending buffer
					if len(client.pending) < maxPendingSize {
						client.pending = append(client.pending, frame)
					} else {
						// Pending buffer overflow - client too slow even during catchup
						pendingOverflow = append(pendingOverflow, client.id)
					}
					handled = true
				}
				client.pendingMu.Unlock()

				if handled {
					continue
				}

				// Client is live (or transitioned while we held pendingMu)
				// Send directly to channel
				if client.state.Load() == clientStateLive {
					func() {
						defer func() {
							if r := recover(); r != nil {
								// Channel was closed between state check and send
							}
						}()
						select {
						case client.frameCh <- frame:
							// Frame sent successfully
						default:
							// Buffer full - client is too slow, mark for disconnection
							slowClients = append(slowClients, client.id)
						}
					}()
				}
				// If state is closed, we skip (handled by initial check or transition)
			}
			s.clientsMu.RUnlock()

			// Disconnect slow/overflow clients (outside of RLock to avoid deadlock)
			for _, clientID := range slowClients {
				fmt.Printf("[SHARED_VIDEO] Disconnecting slow client %d (channel buffer full)\n", clientID)
				s.disconnectClient(clientID)
			}
			for _, clientID := range pendingOverflow {
				fmt.Printf("[SHARED_VIDEO] Disconnecting client %d (pending buffer overflow, max %d frames)\n",
					clientID, maxPendingSize)
				s.disconnectClient(clientID)
			}

			// Log periodically
			if frameCount == 1 || frameCount%300 == 0 {
				fmt.Printf("[SHARED_VIDEO] Broadcast frame %d to %d clients (node %d)\n",
					frameCount, clientCount, s.nodeID)
			}
		}
	}
}

// stop stops the shared video source
// Uses CAS to ensure each channel is closed exactly once.
func (s *SharedVideoSource) stop() {
	s.stopOnce.Do(func() {
		fmt.Printf("[SHARED_VIDEO] Stopping source for node %d\n", s.nodeID)
		s.running.Store(false)
		s.cancel()

		if s.pipeline != nil {
			s.pipeline.Stop()
		}

		// Close all client channels using CAS
		s.clientsMu.Lock()
		for _, client := range s.clients {
			// Try CAS from catching_up → closed
			if client.state.CompareAndSwap(clientStateCatchingUp, clientStateClosed) {
				close(client.frameCh)
				client.pendingMu.Lock()
				client.pending = nil
				client.pendingMu.Unlock()
				continue
			}
			// Try CAS from live → closed
			if client.state.CompareAndSwap(clientStateLive, clientStateClosed) {
				close(client.frameCh)
			}
			// If already closed, CAS fails - no double-close
		}
		s.clients = make(map[uint64]*sharedVideoClient)
		s.clientsMu.Unlock()

		s.wg.Wait()
	})
}

// IsRunning returns whether the pipeline is running
func (s *SharedVideoSource) IsRunning() bool {
	return s.running.Load()
}

// GetFrameStats returns pipeline frame statistics
func (s *SharedVideoSource) GetFrameStats() (received, dropped uint64) {
	if s.pipeline != nil {
		return s.pipeline.GetFrameStats()
	}
	return 0, 0
}

// GetClientCount returns the number of connected clients
func (s *SharedVideoSource) GetClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}

// ClientBufferStats contains buffer statistics for a single client
type ClientBufferStats struct {
	ClientID   uint64 `json:"client_id"`
	BufferUsed int    `json:"buffer_used"`   // Current frames in buffer
	BufferSize int    `json:"buffer_size"`   // Max buffer capacity (GOP size)
	BufferPct  int    `json:"buffer_pct"`    // Percentage full (0-100)
}

// GetClientBufferStats returns buffer statistics for all connected clients
func (s *SharedVideoSource) GetClientBufferStats() []ClientBufferStats {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	bufferSize := getDefaultGOPSize()
	stats := make([]ClientBufferStats, 0, len(s.clients))

	for _, client := range s.clients {
		if client.state.Load() == clientStateClosed {
			continue
		}
		used := len(client.frameCh)
		pct := 0
		if bufferSize > 0 {
			pct = (used * 100) / bufferSize
		}
		stats = append(stats, ClientBufferStats{
			ClientID:   client.id,
			BufferUsed: used,
			BufferSize: bufferSize,
			BufferPct:  pct,
		})
	}
	return stats
}
