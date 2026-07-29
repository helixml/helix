//go:build cgo && linux

package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This is the regression test for the desktop-bridge GPU/fd leak: ~28 MiB of GPU
// memory and ~4.7 /dev/nvidia0 fds were leaked per GStreamer pipeline
// create/destroy cycle, which exhausted a shared 16 GB GPU over 45 hours.
//
// The leak is entirely below the Go layer — activePipelineCount reached 0 and
// every pipeline logged a clean teardown while 1564 nvidia fds were still held —
// so a test that asserts on Go-side counters would have passed throughout the
// incident. The only meaningful assertion is on the process's real OS/GPU
// resources, which is what this test makes.
//
// The test needs a real NVIDIA GPU and a registered nvh264enc. It skips (does not
// fail) when either is missing so non-GPU CI hosts are unaffected.

// leakTestPipeline is a self-contained GPU pipeline: it allocates a CUDA context
// and an NVENC session — the resources that leaked — without needing a PipeWire
// ScreenCast session, so the test does not depend on a live desktop.
const leakTestPipeline = "videotestsrc is-live=true ! video/x-raw,width=1920,height=1080,framerate=30/1 ! " +
	"cudaupload ! nvh264enc ! h264parse ! appsink name=videosink"

// leakTestCycles is the number of create/destroy cycles per sub-test. At the
// 4.66 fds/cycle observed in production this produces a ~93 fd delta on leaking
// code, far outside any plausible noise band.
const leakTestCycles = 20

// leakTestWarmupCycles are discarded before the flatness window opens. The CUDA
// driver opens a handful of persistent /dev/nvidia0 fds and allocates a primary
// context the first time it is used in a process; those are one-off, not a leak.
const leakTestWarmupCycles = 3

// maxLeakedFDs is the tolerance for the flatness assertion across the measured
// window. Anything above this is a leak: production leaked ~4.66 fds per single
// cycle.
const maxLeakedFDs = 2

// maxLeakedGPUMiB is the tolerance for GPU memory flatness across the measured
// window. Production leaked ~28 MiB per single cycle.
const maxLeakedGPUMiB = 32

func requireGPU(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/nvidia0"); err != nil {
		t.Skip("skipping GPU leak test: /dev/nvidia0 not present")
	}
	InitGStreamer()
	if !CheckGstElement("nvh264enc") {
		t.Skip("skipping GPU leak test: nvh264enc is not registered")
	}
	if !CheckGstElement("cudaupload") {
		t.Skip("skipping GPU leak test: cudaupload is not registered")
	}
}

// countNvidiaFDs returns how many of this process's open file descriptors point
// at /dev/nvidia0. This is the in-process equivalent of the
// `ls -l /proc/<pid>/fd | grep -c nvidia0` measurement taken on the leaking
// production process.
func countNvidiaFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	n := 0
	for _, e := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", e.Name()))
		if err != nil {
			// The fd can vanish between ReadDir and Readlink (including the fd
			// ReadDir itself used). Not an error for a counting pass.
			continue
		}
		if target == "/dev/nvidia0" {
			n++
		}
	}
	return n
}

// computeApps returns the GPU memory in MiB used by each process with a CUDA
// context on the GPU, keyed by the pid nvidia-smi reports.
//
// NOTE: nvidia-smi reports *host* pids. Inside a container this process's own pid
// is namespaced and will not match, so callers must identify their own entry by
// elimination (see gpuMiBTracker) rather than by looking up os.Getpid().
func computeApps() (map[int]int, error) {
	out, err := exec.Command("nvidia-smi",
		"--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, err
	}
	apps := map[int]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		mib, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			continue
		}
		apps[pid] = mib
	}
	return apps, nil
}

// gpuMiBTracker measures this process's GPU memory across the test.
//
// Because nvidia-smi reports host pids that cannot be matched against our
// namespaced pid, the tracker snapshots the compute-app pid set before any CUDA
// context exists and then identifies our own entry as the pid that appears
// afterwards. If that identification is ambiguous (another tenant on the shared
// GPU happened to start at the same moment) the tracker degrades to reporting
// only, and the fd assertion carries the test.
type gpuMiBTracker struct {
	baseline map[int]int
	pid      int
	resolved bool
}

func newGPUMiBTracker(t *testing.T) *gpuMiBTracker {
	t.Helper()
	apps, err := computeApps()
	if err != nil {
		t.Logf("nvidia-smi unavailable, GPU memory will not be tracked: %v", err)
		return &gpuMiBTracker{}
	}
	return &gpuMiBTracker{baseline: apps}
}

// resolve finds our own pid in the compute-app list. Called after the first
// pipeline has created a CUDA context.
func (g *gpuMiBTracker) resolve(t *testing.T) {
	t.Helper()
	if g.resolved || g.baseline == nil {
		return
	}
	apps, err := computeApps()
	if err != nil {
		return
	}
	var candidates []int
	for pid := range apps {
		if _, existed := g.baseline[pid]; !existed {
			candidates = append(candidates, pid)
		}
	}
	if len(candidates) != 1 {
		t.Logf("could not unambiguously identify our own GPU compute-app entry (%d candidates); "+
			"GPU memory will be reported but not asserted", len(candidates))
		g.baseline = nil
		return
	}
	g.pid = candidates[0]
	g.resolved = true
	t.Logf("tracking GPU memory for host pid %d", g.pid)
}

// mib returns our GPU memory in MiB, or -1 when it could not be determined.
func (g *gpuMiBTracker) mib() int {
	if !g.resolved {
		return -1
	}
	apps, err := computeApps()
	if err != nil {
		return -1
	}
	mib, ok := apps[g.pid]
	if !ok {
		// Our CUDA context is fully gone — that is the ideal outcome, not a
		// measurement failure.
		return 0
	}
	return mib
}

// runLeakCycles runs `cycles` create/destroy cycles of `cycle` and asserts that
// neither /dev/nvidia0 fds nor GPU memory grow after the warm-up window.
func runLeakCycles(t *testing.T, cycle func(t *testing.T, i int)) {
	t.Helper()

	gpu := newGPUMiBTracker(t)

	var (
		windowFDs int
		windowMiB int
		haveFDs   bool
	)

	for i := 0; i < leakTestCycles; i++ {
		cycle(t, i)

		if i == 0 {
			gpu.resolve(t)
		}

		fds := countNvidiaFDs(t)
		mib := gpu.mib()
		t.Logf("cycle %2d: /dev/nvidia0 fds=%d gpu=%d MiB", i+1, fds, mib)

		if i == leakTestWarmupCycles-1 {
			// Warm-up over: this is the reference point for flatness.
			windowFDs, windowMiB, haveFDs = fds, mib, true
			continue
		}
		if !haveFDs {
			continue
		}
		if delta := fds - windowFDs; delta > maxLeakedFDs {
			t.Fatalf("leaked %d /dev/nvidia0 fds between cycle %d and cycle %d (%d -> %d, tolerance %d)",
				delta, leakTestWarmupCycles, i+1, windowFDs, fds, maxLeakedFDs)
		}
		if mib >= 0 && windowMiB >= 0 {
			if delta := mib - windowMiB; delta > maxLeakedGPUMiB {
				t.Fatalf("leaked %d MiB of GPU memory between cycle %d and cycle %d (%d -> %d MiB, tolerance %d)",
					delta, leakTestWarmupCycles, i+1, windowMiB, mib, maxLeakedGPUMiB)
			}
		}
	}
}

// TestPipelineCreateStopDoesNotLeak covers the "never started" shape: a pipeline
// is parsed (which is where nvh264enc creates its CUDA context) and then stopped
// without ever reaching PLAYING. 213 of the 324 production pipelines took this
// path — every WebSocket connect that failed to start capture.
func TestPipelineCreateStopDoesNotLeak(t *testing.T) {
	requireGPU(t)

	runLeakCycles(t, func(t *testing.T, i int) {
		p, err := NewGstPipeline(leakTestPipeline)
		if err != nil {
			t.Fatalf("cycle %d: create pipeline: %v", i+1, err)
		}
		p.Stop()
	})
}

// TestPipelineStartStopDoesNotLeak covers the "started" shape: the pipeline
// reaches PLAYING, delivers frames, and is then stopped. 111 of the 324
// production pipelines took this path.
func TestPipelineStartStopDoesNotLeak(t *testing.T) {
	requireGPU(t)

	runLeakCycles(t, func(t *testing.T, i int) {
		p, err := NewGstPipeline(leakTestPipeline)
		if err != nil {
			t.Fatalf("cycle %d: create pipeline: %v", i+1, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := p.Start(ctx); err != nil {
			p.Stop()
			t.Fatalf("cycle %d: start pipeline: %v", i+1, err)
		}

		if err := consumeFrames(p, 3, 15*time.Second); err != nil {
			p.Stop()
			t.Fatalf("cycle %d: %v", i+1, err)
		}

		p.Stop()
	})
}

// consumeFrames waits for n frames so the cycle exercises a pipeline that really
// encoded on the GPU, not just one that was configured.
func consumeFrames(p *GstPipeline, n int, timeout time.Duration) error {
	deadline := time.After(timeout)
	got := 0
	for got < n {
		select {
		case _, ok := <-p.Frames():
			if !ok {
				return fmt.Errorf("frame channel closed after %d/%d frames", got, n)
			}
			got++
		case err := <-p.Errors():
			return fmt.Errorf("pipeline error after %d/%d frames: %w", got, n, err)
		case <-deadline:
			return fmt.Errorf("timed out waiting for frames (%d/%d)", got, n)
		}
	}
	return nil
}
