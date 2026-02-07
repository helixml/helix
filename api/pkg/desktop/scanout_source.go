package desktop

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
)

const (
	defaultQEMUAddr  = "10.0.2.2:15937"
	defaultDRMSocket = "/run/helix-drm.sock"
)

// ScanoutSource receives pre-encoded H.264 frames from QEMU's helix-frame-export
// via TCP. This bypasses PipeWire and GStreamer entirely - encoding is done by
// QEMU's VideoToolbox on the macOS host.
//
// Flow: Mutter → virtio-gpu page flip → QEMU resource_flush → VideoToolbox H.264
//       → TCP → ScanoutSource → VideoStreamer → WebSocket → browser
type ScanoutSource struct {
	logger    *slog.Logger
	qemuAddr  string
	drmSocket string

	mu        sync.Mutex
	conn      net.Conn
	scanoutID uint32
	running   bool
	cancel    context.CancelFunc

	// Frame delivery
	frameCh chan VideoFrame
	errorCh chan error
}

// NewScanoutSource creates a new scanout H.264 source.
func NewScanoutSource(logger *slog.Logger) *ScanoutSource {
	qemuAddr := defaultQEMUAddr
	drmSocket := defaultDRMSocket
	return &ScanoutSource{
		logger:    logger,
		qemuAddr:  qemuAddr,
		drmSocket: drmSocket,
		frameCh:   make(chan VideoFrame, 16),
		errorCh:   make(chan error, 1),
	}
}

// Start connects to QEMU, subscribes to the scanout, and begins receiving frames.
// If scanoutID is 0, it requests a DRM lease from helix-drm-manager to get one.
func (s *ScanoutSource) Start(ctx context.Context, scanoutID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	ctx, s.cancel = context.WithCancel(ctx)

	// Determine scanout ID:
	// 1. From explicit argument (if > 0)
	// 2. From HELIX_SCANOUT_ID env var
	// 3. From file written by mutter-lease-launcher ($XDG_RUNTIME_DIR/helix-scanout-id)
	// 4. Request a new DRM lease (last resort)
	if scanoutID == 0 {
		if envID := os.Getenv("HELIX_SCANOUT_ID"); envID != "" {
			if n, err := strconv.Atoi(envID); err == nil && n > 0 {
				scanoutID = uint32(n)
				s.logger.Info("Using scanout ID from env", "scanout_id", scanoutID)
			}
		}
	}
	if scanoutID == 0 {
		xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
		if xdgRuntime == "" {
			xdgRuntime = "/run/user/1000"
		}
		scanoutFile := xdgRuntime + "/helix-scanout-id"
		if data, err := os.ReadFile(scanoutFile); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && n > 0 {
				scanoutID = uint32(n)
				s.logger.Info("Using scanout ID from file", "scanout_id", scanoutID, "file", scanoutFile)
			}
		}
	}
	if scanoutID == 0 {
		// Last resort: request a DRM lease to get a scanout ID
		client := drmmanager.NewClient(s.drmSocket)
		lease, err := client.RequestLease(1920, 1080)
		if err != nil {
			return fmt.Errorf("request DRM lease: %w", err)
		}
		scanoutID = lease.ScanoutID
		s.logger.Info("DRM lease acquired for scanout ID",
			"scanout_id", scanoutID,
			"connector", lease.ConnectorName)
	}
	s.scanoutID = scanoutID

	// Connect to QEMU frame export server
	conn, err := net.DialTimeout("tcp", s.qemuAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to QEMU %s: %w", s.qemuAddr, err)
	}
	s.conn = conn

	// Subscribe to scanout
	if err := drmmanager.WriteSubscribe(conn, scanoutID); err != nil {
		conn.Close()
		return fmt.Errorf("subscribe to scanout %d: %w", scanoutID, err)
	}

	// Read subscribe response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	respScanout, success, err := drmmanager.ReadSubscribeResp(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("subscribe response: %w", err)
	}
	if !success {
		conn.Close()
		return fmt.Errorf("subscribe rejected for scanout %d", scanoutID)
	}
	conn.SetReadDeadline(time.Time{}) // Clear deadline

	s.logger.Info("subscribed to QEMU scanout",
		"scanout_id", respScanout,
		"qemu_addr", s.qemuAddr)

	s.running = true

	// Start frame reader goroutine
	go s.readFrames(ctx)

	return nil
}

// FrameCh returns the channel for receiving video frames.
func (s *ScanoutSource) FrameCh() <-chan VideoFrame {
	return s.frameCh
}

// Frames implements FrameSource interface.
func (s *ScanoutSource) Frames() <-chan VideoFrame {
	return s.frameCh
}

// ErrorCh returns the channel for receiving errors.
func (s *ScanoutSource) ErrorCh() <-chan error {
	return s.errorCh
}

// Errors implements FrameSource interface.
func (s *ScanoutSource) Errors() <-chan error {
	return s.errorCh
}

// Stop disconnects from QEMU and stops frame delivery.
func (s *ScanoutSource) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		s.conn.Close()
	}
}

// readFrames reads H.264 frames from the QEMU TCP connection and delivers them
// to the frame channel.
func (s *ScanoutSource) readFrames(ctx context.Context) {
	defer func() {
		close(s.frameCh)
		s.running = false
	}()

	frameCount := uint64(0)
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read frame from QEMU protocol
		scanoutID, nalData, isKeyframe, err := drmmanager.ReadFrameResponse(s.conn)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled
			}
			s.logger.Error("QEMU frame read error", "err", err)
			select {
			case s.errorCh <- err:
			default:
			}
			return
		}

		_ = scanoutID // We already know which scanout we're subscribed to

		frameCount++
		now := time.Now()

		// Convert QEMU PTS (nanoseconds) to microseconds for the VideoFrame format
		ptsUs := frameCount * 16666 // ~60fps timing in microseconds

		frame := VideoFrame{
			Data:       nalData,
			PTS:        ptsUs,
			IsKeyframe: isKeyframe,
			IsReplay:   false,
			Timestamp:  now,
		}

		select {
		case s.frameCh <- frame:
		default:
			// Channel full, drop frame
			if frameCount%100 == 0 {
				fps := float64(frameCount) / time.Since(startTime).Seconds()
				s.logger.Warn("scanout frame dropped (channel full)",
					"frame", frameCount, "fps", fmt.Sprintf("%.1f", fps))
			}
		}

		// Periodic stats
		if frameCount == 1 || frameCount%300 == 0 {
			fps := float64(frameCount) / time.Since(startTime).Seconds()
			s.logger.Info("scanout stream stats",
				"frames", frameCount,
				"fps", fmt.Sprintf("%.1f", fps),
				"last_size", len(nalData),
				"keyframe", isKeyframe)
		}
	}
}
