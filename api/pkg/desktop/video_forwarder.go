package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// VideoForwarder captures video from PipeWire and forwards frames to Wolf via shared memory.
// This solves the cross-container PipeWire authorization issue by running the capture
// inside the container where PipeWire is accessible.
type VideoForwarder struct {
	nodeID        uint32
	shmSocketPath string
	cmd           *exec.Cmd
	running       bool
	mu            sync.Mutex
	logger        interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
		Debug(msg string, args ...any)
	}
}

// NewVideoForwarder creates a new video forwarder.
func NewVideoForwarder(nodeID uint32, shmSocketPath string, logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}) *VideoForwarder {
	return &VideoForwarder{
		nodeID:        nodeID,
		shmSocketPath: shmSocketPath,
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

	// Build GStreamer pipeline:
	// pipewiresrc path=<node_id> - capture from GNOME ScreenCast
	// videoconvert - ensure format compatibility
	// shmsink - output to shared memory socket
	//
	// Note: We use raw video (BGRx) because Wolf's NVENC pipeline expects raw input.
	// The shmsink creates a Unix socket that Wolf's shmsrc can connect to.
	pipelineDef := fmt.Sprintf(
		"pipewiresrc path=%d do-timestamp=true ! "+
			"video/x-raw,format=BGRx ! "+
			"shmsink socket-path=%s wait-for-connection=true sync=false",
		v.nodeID, v.shmSocketPath,
	)

	v.logger.Info("starting video forwarder",
		"node_id", v.nodeID,
		"shm_socket", v.shmSocketPath,
		"pipeline", pipelineDef)

	v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q", "-e",
		"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
		"!", "video/x-raw,format=BGRx",
		"!", "shmsink", fmt.Sprintf("socket-path=%s", v.shmSocketPath),
		"wait-for-connection=true", "sync=false",
	)

	// Inherit environment for PipeWire access
	v.cmd.Env = os.Environ()

	// Start the process
	if err := v.cmd.Start(); err != nil {
		return fmt.Errorf("start gst-launch: %w", err)
	}

	v.running = true

	// Monitor the process
	go v.monitor(ctx)

	v.logger.Info("video forwarder started, waiting for socket creation", "pid", v.cmd.Process.Pid)

	// Wait for the socket to be created before returning.
	// Wolf's shmsrc will fail if it tries to connect before the socket exists.
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

		v.cmd = exec.CommandContext(ctx, "gst-launch-1.0", "-q", "-e",
			"pipewiresrc", fmt.Sprintf("path=%d", v.nodeID), "do-timestamp=true",
			"!", "video/x-raw,format=BGRx",
			"!", "shmsink", fmt.Sprintf("socket-path=%s", v.shmSocketPath),
			"wait-for-connection=true", "sync=false",
		)
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

	if v.cmd != nil && v.cmd.Process != nil {
		v.logger.Info("stopping video forwarder", "pid", v.cmd.Process.Pid)
		v.cmd.Process.Kill()
		v.cmd.Wait()
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

// ShmSocketPath returns the path to the shared memory socket.
func (v *VideoForwarder) ShmSocketPath() string {
	return v.shmSocketPath
}
