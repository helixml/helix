// Package desktop provides desktop integration for Helix sandboxes.
package desktop

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	gnomeShellBus   = "org.gnome.Shell"
	gnomeShellPath  = "/org/gnome/Shell"
	gnomeShellIface = "org.gnome.Shell"

	// Interval between damage keepalive pings.
	// 500ms = 2 FPS minimum on static screens.
	damageKeepaliveInterval = 500 * time.Millisecond

	// GNOME Shell JavaScript to create a 1x1 St.Widget and toggle its visibility.
	// This generates a minimal damage event (1 pixel) that triggers PipeWire
	// ScreenCast to produce a new buffer, preventing pipeline stall on static screens.
	//
	// On virtio-gpu (macOS ARM / UTM), PipeWire ScreenCast is strictly damage-based:
	// no screen changes = no frames. The pipewiresrc keepalive-time property doesn't
	// force frame production. This workaround ensures at least 2 FPS on static screens.
	//
	// Uses St.Widget (GNOME Shell Toolkit) with CSS background instead of Clutter.Color,
	// which is not a constructor in GNOME Shell 45+.
	damageKeepaliveJS = `(function(){` +
		`if(!global._hk){` +
		`global._hk=new imports.gi.St.Widget({width:1,height:1,x:0,y:0,` +
		`style:'background:#000'});` +
		`global.stage.add_child(global._hk)}` +
		`global._hk.visible=!global._hk.visible` +
		`})()`

	// Cleanup script to remove the keepalive actor on shutdown.
	damageKeepaliveCleanupJS = `if(global._hk){global._hk.destroy();delete global._hk}`
)

// runDamageKeepalive periodically generates minimal screen damage via GNOME Shell D-Bus Eval.
//
// PipeWire ScreenCast on GNOME is damage-based: it only produces frames when screen pixels
// change. On a completely static desktop, the pipeline receives 2 initial frames and then
// stalls permanently. The pipewiresrc keepalive-time=500 property is supposed to handle this,
// but doesn't work on virtio-gpu (macOS ARM / UTM).
//
// This workaround creates a 1x1 black pixel actor at position (0,0) on the GNOME Shell stage
// and toggles its opacity between 0 and 1 every 500ms. This generates a 1-pixel damage rect,
// which is enough for ScreenCast to produce a new PipeWire buffer.
//
// Overhead: one D-Bus method call every 500ms + 1 pixel compositing = negligible.
func (s *Server) runDamageKeepalive(ctx context.Context) {
	s.logger.Info("[KEEPALIVE] Starting damage keepalive for PipeWire ScreenCast")

	ticker := time.NewTicker(damageKeepaliveInterval)
	defer ticker.Stop()

	shell := s.conn.Object(gnomeShellBus, dbus.ObjectPath(gnomeShellPath))
	var failCount int
	var successCount int

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("[KEEPALIVE] Damage keepalive stopped")
			// Clean up the actor
			shell.Call(gnomeShellIface+".Eval", 0, damageKeepaliveCleanupJS)
			return
		case <-ticker.C:
			// Call GNOME Shell Eval to toggle the keepalive actor
			var success bool
			var result string
			err := shell.Call(gnomeShellIface+".Eval", 0, damageKeepaliveJS).Store(&success, &result)
			if err != nil {
				failCount++
				if failCount <= 3 || failCount%100 == 0 {
					s.logger.Warn("[KEEPALIVE] D-Bus Eval failed",
						"err", err, "failures", failCount)
				}
				continue
			}
			if !success {
				failCount++
				if failCount <= 3 || failCount%100 == 0 {
					s.logger.Warn("[KEEPALIVE] Shell Eval returned failure",
						"result", result, "failures", failCount)
				}
				continue
			}
			successCount++
			if successCount == 1 {
				s.logger.Info("[KEEPALIVE] Damage keepalive active (first success)")
			}
		}
	}
}
