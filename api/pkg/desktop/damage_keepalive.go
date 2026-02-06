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
// 2. NotifyPointerMotionAbsolute via RemoteDesktop D-Bus positions the cursor on the linked stream
// 3. Mutter's cursor_changed callback calls maybe_record_frame with the cursor embedded
// 4. This generates real compositor-level damage → PipeWire produces a new frame
//
// We use NotifyPointerMotionAbsolute (not Relative) because:
// - Absolute positioning requires a stream object path, which explicitly associates the
//   cursor movement with a specific monitor/ScreenCast session
// - On Mutter 49+ headless mode, relative motion (NotifyPointerMotionRelative) succeeds
//   but doesn't generate compositor-level damage because the cursor position isn't
//   associated with any specific output
// - Absolute positioning on the linked stream ensures Mutter knows which output's
//   cursor changed, triggering the damage path
//
// The cursor sweeps through unique positions in a small area (10x10 pixels around 100,100).
// Each tick moves to a new position, ensuring Mutter 49 can't coalesce repeated identical
// movements. User mouse events immediately override the keepalive position.
//
// Overhead: one D-Bus method call every 500ms = negligible.
func (s *Server) runDamageKeepalive(ctx context.Context) {
	s.logger.Info("[KEEPALIVE] Starting cursor-based damage keepalive for PipeWire ScreenCast")

	ticker := time.NewTicker(damageKeepaliveInterval)
	defer ticker.Stop()

	var counter int
	var failCount int
	var successCount int

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("[KEEPALIVE] Cursor damage keepalive stopped")
			return
		case <-ticker.C:
			// Need an active RemoteDesktop session and linked ScreenCast stream
			if s.rdSessionPath == "" || s.scStreamPath == "" {
				continue
			}

			// Cycle through unique positions in a 10x10 grid around (100,100).
			// Every tick has a different (x,y) so Mutter 49 can't coalesce
			// repeated identical cursor positions.
			pos := counter % 100
			x := float64(100 + pos%10)
			y := float64(100 + pos/10)
			counter++

			rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)

			// Use NotifyPointerMotionAbsolute with the linked stream path.
			// This explicitly ties cursor movement to the ScreenCast stream's output,
			// which ensures Mutter generates compositor-level damage on the correct monitor.
			// Note: Mutter 49 expects stream as string type "s", not ObjectPath "o".
			err := rdSession.Call(
				remoteDesktopSessionIface+".NotifyPointerMotionAbsolute", 0,
				string(s.scStreamPath), x, y,
			).Err
			if err != nil {
				failCount++
				if failCount <= 3 || failCount%100 == 0 {
					s.logger.Warn("[KEEPALIVE] NotifyPointerMotionAbsolute failed",
						"err", err, "failures", failCount, "stream", s.scStreamPath)
				}
				continue
			}
			successCount++
			if successCount == 1 {
				s.logger.Info("[KEEPALIVE] Cursor damage keepalive active (first success)",
					"method", "NotifyPointerMotionAbsolute", "stream", s.scStreamPath)
			}
		}
	}
}
