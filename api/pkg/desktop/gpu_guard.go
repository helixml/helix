package desktop

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// GPU resource guard.
//
// On 2026-07-28 a single desktop-bridge accumulated 9.3 GB of GPU memory and
// 1564 open /dev/nvidia0 fds over 45 hours, starved every other tenant on a
// shared 16 GB card, and broke an unrelated user's desktop. Nothing noticed.
// Go-side accounting said zero pipelines were active the whole time, so no
// counter-based check would have caught it.
//
// This guard watches the only numbers that cannot lie — the process's own open
// file descriptors — and makes the condition loud, then refuses to make it
// worse. It is a backstop, not a fix: leaks are bugs and get fixed at the source.
const (
	// gpuGuardWarnFDs is where we start shouting. A healthy bridge holds ~52
	// /dev/nvidia0 fds with an active stream; the leaker reached 1564.
	gpuGuardWarnFDs = 200

	// gpuGuardMaxFDs is where we stop instantiating pipelines. Past this point a
	// new pipeline is far more likely to deepen an existing leak than to serve a
	// working stream, and the failure it causes lands on other tenants.
	gpuGuardMaxFDs = 400

	// gpuGuardInterval is how often the check runs. It reads one directory, so
	// the cost is negligible and there is no per-frame component.
	gpuGuardInterval = 30 * time.Second
)

// lastNvidiaFDCount caches the most recent measurement so the admission check in
// the pipeline path does not have to walk /proc on every subscribe.
var lastNvidiaFDCount atomic.Int64

// activePipelineCount tracks how many GStreamer pipelines are currently running.
// Declared here rather than in gst_pipeline.go so it is available in both the
// cgo and no-cgo builds.
//
// Worth remembering what this number is worth: throughout the 45-hour leak it
// read 0 while the process held 1564 /dev/nvidia0 fds. It is useful log context
// and nothing more — never treat it as evidence that resources were released.
var activePipelineCount atomic.Int32

// ActivePipelineCount returns the number of GStreamer pipelines currently running.
func ActivePipelineCount() int32 { return activePipelineCount.Load() }

// countProcessNvidiaFDs returns how many of this process's file descriptors point
// at /dev/nvidia0. Returns -1 when /proc is unreadable (non-Linux, restricted
// sandbox), in which case the guard stays out of the way.
func countProcessNvidiaFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	n := 0
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", e.Name()))
		if err != nil {
			// The fd can disappear between listing and reading the link — including
			// the one ReadDir itself used. Not an error for a counting pass.
			continue
		}
		if target == "/dev/nvidia0" {
			n++
		}
	}
	return n
}

// StartGPUResourceGuard begins periodic GPU resource monitoring. Safe to call
// once at startup; it runs until the process exits.
func StartGPUResourceGuard(logger *slog.Logger) {
	if _, err := os.Stat("/dev/nvidia0"); err != nil {
		return // No NVIDIA device — nothing to guard.
	}
	go func() {
		ticker := time.NewTicker(gpuGuardInterval)
		defer ticker.Stop()
		for range ticker.C {
			n := countProcessNvidiaFDs()
			if n < 0 {
				continue
			}
			lastNvidiaFDCount.Store(int64(n))
			switch {
			case n >= gpuGuardMaxFDs:
				logger.Error("GPU resource guard: /dev/nvidia0 fd count is over the hard limit, "+
					"refusing new pipelines — this process is leaking GPU resources",
					"nvidia_fds", n, "limit", gpuGuardMaxFDs,
					"active_pipelines", ActivePipelineCount())
			case n >= gpuGuardWarnFDs:
				logger.Warn("GPU resource guard: /dev/nvidia0 fd count is unusually high",
					"nvidia_fds", n, "warn_at", gpuGuardWarnFDs, "hard_limit", gpuGuardMaxFDs,
					"active_pipelines", ActivePipelineCount())
			}
		}
	}()
}

// CheckGPUResourceBudget reports an error when this process is holding so many
// GPU file descriptors that creating another pipeline would endanger other
// tenants on the same card. Called before pipeline instantiation.
func CheckGPUResourceBudget() error {
	// Use the cached value if the guard is running; otherwise measure directly so
	// the check still works in tests and short-lived processes.
	n := int(lastNvidiaFDCount.Load())
	if n == 0 {
		n = countProcessNvidiaFDs()
		if n < 0 {
			return nil
		}
		lastNvidiaFDCount.Store(int64(n))
	}
	if n >= gpuGuardMaxFDs {
		return fmt.Errorf("refusing to create another video pipeline: this desktop is holding %d "+
			"/dev/nvidia0 file descriptors (limit %d), which indicates leaked GPU resources. "+
			"Restart the desktop session to recover", n, gpuGuardMaxFDs)
	}
	return nil
}
