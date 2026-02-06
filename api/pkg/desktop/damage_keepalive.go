// Package desktop provides desktop integration for Helix sandboxes.
package desktop

import (
	"context"
	"time"
)

const (
	// Interval between cursor keepalive movements.
	// 500ms = 2 FPS minimum on static screens.
	damageKeepaliveInterval = 500 * time.Millisecond
)

// runDamageKeepalive periodically generates screen damage by injecting tiny cursor movements
// via the RemoteDesktop D-Bus API.
//
// PipeWire ScreenCast on GNOME is damage-based: it only produces frames when screen pixels
// change. On a completely static desktop, the pipeline stalls after an initial burst of frames.
//
// Previous approaches that FAILED on virtio-gpu (macOS ARM / UTM):
// - pipewiresrc keepalive-time=500: resends last buffer, but the PipeWire thread loop
//   gets spurious wakeups from other ScreenCast sessions sharing the same connection,
//   resetting the 500ms timer before it fires.
// - GNOME Shell D-Bus Eval (St.Widget toggle, color toggle, queue_redraw): Clutter actor
//   changes don't generate compositor-level damage on virtio-gpu headless mode.
//
// This approach works because:
// 1. The linked ScreenCast session uses cursor-mode=Embedded (cursor composited into frame)
// 2. NotifyPointerMotion via RemoteDesktop D-Bus moves the real cursor
// 3. Mutter's cursor_changed callback calls maybe_record_frame with the cursor embedded
// 4. This generates real compositor-level damage → PipeWire produces a new frame
//
// The cursor jitter (1px right/left alternating) is imperceptible, and user mouse events
// (absolute positioning from WebSocket) immediately override the keepalive position.
//
// Overhead: one D-Bus method call every 500ms = negligible.
func (s *Server) runDamageKeepalive(ctx context.Context) {
	s.logger.Info("[KEEPALIVE] Starting cursor-based damage keepalive for PipeWire ScreenCast")

	ticker := time.NewTicker(damageKeepaliveInterval)
	defer ticker.Stop()

	var toggle bool
	var failCount int
	var successCount int

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("[KEEPALIVE] Cursor damage keepalive stopped")
			return
		case <-ticker.C:
			// Need an active RemoteDesktop session for cursor movement
			if s.rdSessionPath == "" {
				continue
			}

			// Alternate between +1px and -1px horizontal movement.
			// Net movement over two ticks: 0px (invisible jitter).
			dx := float64(1)
			if toggle {
				dx = -1
			}
			toggle = !toggle

			rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
			err := rdSession.Call(remoteDesktopSessionIface+".NotifyPointerMotion", 0, dx, float64(0)).Err
			if err != nil {
				failCount++
				if failCount <= 3 || failCount%100 == 0 {
					s.logger.Warn("[KEEPALIVE] NotifyPointerMotion failed",
						"err", err, "failures", failCount)
				}
				continue
			}
			successCount++
			if successCount == 1 {
				s.logger.Info("[KEEPALIVE] Cursor damage keepalive active (first success)")
			}
		}
	}
}
