package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// createFIFO creates a named pipe (FIFO) at the given path
func createFIFO(path string) error {
	return syscall.Mkfifo(path, 0644)
}

// VideoForwarder captures video and forwards frames via shared memory.
// Supports two capture modes:
// - GNOME: Uses pipewiresrc to capture from Mutter's ScreenCast PipeWire node
// - Sway: Uses wf-recorder (wlr-screencopy) since pipewiresrc has issues with xdg-desktop-portal-wlr
type VideoForwarder struct {
	nodeID        uint32
	shmSocketPath string
	useSway       bool // If true, use wf-recorder instead of pipewiresrc
	cmd           *exec.Cmd
	running       bool
	cancelMonitor context.CancelFunc // Cancels the monitor goroutine
	mu            sync.Mutex
	logger        interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
		Debug(msg string, args ...any)
	}
}

// NewVideoForwarder creates a new video forwarder for GNOME (uses pipewiresrc).
func NewVideoForwarder(nodeID uint32, shmSocketPath string, logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}) *VideoForwarder {
	return &VideoForwarder{
		nodeID:        nodeID,
		shmSocketPath: shmSocketPath,
		useSway:       false,
		logger:        logger,
	}
}

// NewVideoForwarderForSway creates a video forwarder for Sway (uses wf-recorder).
// wf-recorder uses wlr-screencopy protocol directly, bypassing the problematic
// xdg-desktop-portal-wlr + pipewiresrc path that hangs during format negotiation.
func NewVideoForwarderForSway(shmSocketPath string, logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}) *VideoForwarder {
	return &VideoForwarder{
		nodeID:        0, // Not used for Sway
		shmSocketPath: shmSocketPath,
		useSway:       true,
		logger:        logger,
	}
}

// Start begins the video forwarding process.
// It spawns gst-launch to capture from PipeWire and output to shmsink.
// This method blocks until the socket is created (or timeout/error).
func (v *VideoForwarder) Start(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return nil
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(v.shmSocketPath), 0755); err != nil {
		return fmt.Errorf("create shm socket dir: %w", err)
	}

	// Remove existing socket if present
	os.Remove(v.shmSocketPath)

	if v.useSway {
		// Sway: Use wf-recorder (wlr-screencopy) to output H.264 directly to a FIFO
		// wf-recorder uses the native wlroots screen capture protocol which is more reliable
		// than xdg-desktop-portal-wlr + pipewiresrc (which hangs during format negotiation)
		//
		// Output format: raw H.264 Annex-B stream (NAL units with start codes)
		// ws_stream.go reads from this FIFO and RTP-packetizes (no additional encoding needed)
		//
		// Note: We use nvenc for GPU-accelerated encoding if available
		// The -m h264 flag outputs raw H.264 without container (Annex-B format)
		fifoPath := v.shmSocketPath // Reuse socket path for FIFO

		// Create FIFO (named pipe) for H.264 stream
		os.Remove(fifoPath)
		if err := createFIFO(fifoPath); err != nil {
			return fmt.Errorf("create FIFO: %w", err)
		}

		// wf-recorder command:
		// -y: overwrite without prompting
		// -c h264_nvenc: use NVIDIA encoder (falls back to libx264 if unavailable)
		// -x yuv420p: pixel format for encoding
		// -m h264: output raw H.264 annex-b format (no container)
		// --no-damage: capture every frame, not just on damage (for smoother streaming)
		v.logger.Info("starting video forwarder (Sway/wf-recorder)",
			"fifo", fifoPath)

		v.cmd = exec.CommandContext(ctx, "wf-recorder",
			"-y",
			"-c", "h264_nvenc",
			"-x", "yuv420p",
			"-m", "h264",
			"--no-damage",
			"-f", fifoPath,
		)
	} else {
		// GNOME: Use pipewiresrc to capture from Mutter's ScreenCast PipeWire node
		// This works reliably because GNOME's ScreenCast creates a proper PipeWire stream
		pipelineDef := fmt.Sprintf(
			"pipewiresrc path=%d do-timestamp=true ! "+
				"video/x-raw,format=BGRx ! "+
				"shmsink socket-path=%s wait-for-connection=true sync=false",
			v.nodeID, v.shmSocketPath,
		)

		v.logger.Info("starting video forwarder (GNOME/pipewiresrc)",
			"node_id", v.nodeID,
			"shm_socket", v.shmSocketPath,
			"pipeline", pipelineDef)

		v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q", "-e",
			"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
			"!", "video/x-raw,format=BGRx",
			"!", "shmsink", fmt.Sprintf("socket-path=%s", v.shmSocketPath),
			"wait-for-connection=true", "sync=false",
		)
	}

	// Inherit environment for PipeWire/Wayland access
	v.cmd.Env = os.Environ()

	// Start the process
	if err := v.cmd.Start(); err != nil {
		return fmt.Errorf("start video forwarder: %w", err)
	}

	v.running = true

	// Create cancelable context for the monitor goroutine
	// This allows Stop() to cleanly shut down the monitor
	monitorCtx, cancelMonitor := context.WithCancel(context.Background())
	v.cancelMonitor = cancelMonitor

	// Monitor the process
	go v.monitor(monitorCtx)

	v.logger.Info("video forwarder started, waiting for socket creation", "pid", v.cmd.Process.Pid)

	// Wait for the socket to be created before returning.
	// Consumers will fail if they try to connect before the socket exists.
	if err := v.waitForSocket(ctx, 10*time.Second); err != nil {
		v.logger.Error("socket creation timeout, stopping forwarder", "err", err)
		v.running = false
		if v.cmd.Process != nil {
			v.cmd.Process.Kill()
		}
		return err
	}

	v.logger.Info("video forwarder ready, socket created", "socket", v.shmSocketPath)
	return nil
}

// waitForSocket polls for the socket file to exist.
// This is called without holding the mutex (must release before calling).
func (v *VideoForwarder) waitForSocket(ctx context.Context, timeout time.Duration) error {
	// Release the mutex while waiting so monitor() can run
	v.mu.Unlock()
	defer v.mu.Lock()

	deadline := time.Now().Add(timeout)
	pollInterval := 50 * time.Millisecond

	for time.Now().Before(deadline) {
		// Check if socket exists
		if _, err := os.Stat(v.shmSocketPath); err == nil {
			return nil
		}

		// Check if context was cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		// Increase poll interval gradually (up to 200ms)
		if pollInterval < 200*time.Millisecond {
			pollInterval += 25 * time.Millisecond
		}
	}

	return fmt.Errorf("socket not created within %v", timeout)
}

// monitor watches the gst-launch process and restarts on failure.
func (v *VideoForwarder) monitor(ctx context.Context) {
	for {
		if v.cmd == nil || v.cmd.Process == nil {
			return
		}

		err := v.cmd.Wait()

		v.mu.Lock()
		running := v.running
		v.mu.Unlock()

		if !running {
			v.logger.Debug("video forwarder stopped")
			return
		}

		v.logger.Warn("video forwarder process exited", "err", err)

		// Wait before restarting
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		v.logger.Info("restarting video forwarder")

		// Restart the process
		v.mu.Lock()
		if !v.running {
			v.mu.Unlock()
			return
		}

		os.Remove(v.shmSocketPath)

		if v.useSway {
			// Sway: Recreate FIFO and restart wf-recorder
			fifoPath := v.shmSocketPath
			os.Remove(fifoPath)
			if err := createFIFO(fifoPath); err != nil {
				v.logger.Error("failed to recreate FIFO", "err", err)
				v.running = false
				v.mu.Unlock()
				return
			}
			v.cmd = exec.CommandContext(ctx, "wf-recorder",
				"-y",
				"-c", "h264_nvenc",
				"-x", "yuv420p",
				"-m", "h264",
				"--no-damage",
				"-f", fifoPath,
			)
		} else {
			// GNOME: Use pipewiresrc pipeline
			v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q", "-e",
				"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
				"!", "video/x-raw,format=BGRx",
				"!", "shmsink", fmt.Sprintf("socket-path=%s", v.shmSocketPath),
				"wait-for-connection=true", "sync=false",
			)
		}
		v.cmd.Env = os.Environ()

		if err := v.cmd.Start(); err != nil {
			v.logger.Error("failed to restart video forwarder", "err", err)
			v.running = false
			v.mu.Unlock()
			return
		}
		v.logger.Info("video forwarder restarted", "pid", v.cmd.Process.Pid)
		v.mu.Unlock()
	}
}

// Stop terminates the video forwarder.
func (v *VideoForwarder) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.running {
		return
	}

	v.running = false

	// Cancel the monitor goroutine first to prevent it from restarting
	if v.cancelMonitor != nil {
		v.cancelMonitor()
		v.cancelMonitor = nil
	}

	if v.cmd != nil && v.cmd.Process != nil {
		v.logger.Info("stopping video forwarder", "pid", v.cmd.Process.Pid)
		v.cmd.Process.Kill()
		// Don't call Wait() here - the monitor goroutine will reap the process
		// and exit cleanly since running is now false
	}

	// Clean up socket
	os.Remove(v.shmSocketPath)
}

// IsRunning returns whether the forwarder is active.
func (v *VideoForwarder) IsRunning() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.running
}

// Restart stops and restarts the video forwarder.
// This is needed when WebSocket clients reconnect because shmsink/shmsrc
// can get into a bad state after client disconnection.
// Note: Uses background context so the forwarder isn't tied to any single WebSocket connection.
func (v *VideoForwarder) Restart(ctx context.Context) error {
	v.logger.Info("restarting video forwarder for new connection")
	v.Stop()
	// Use background context - the forwarder should live beyond individual connections.
	// We ignore the passed ctx to avoid killing the forwarder when the WebSocket closes.
	return v.Start(context.Background())
}

// ShmSocketPath returns the path to the shared memory socket.
func (v *VideoForwarder) ShmSocketPath() string {
	return v.shmSocketPath
}
