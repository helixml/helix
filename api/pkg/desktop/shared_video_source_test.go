//go:build cgo && linux

package desktop

import (
	"testing"
	"time"
)

func newTestVideoRegistry() *SharedVideoSourceRegistry {
	return &SharedVideoSourceRegistry{
		sources:         make(map[uint32]*SharedVideoSource),
		pendingStops:    make(map[uint32]*pendingStop),
		gracePeriod:     time.Minute,
		failureCounts:   make(map[uint32]int),
		failureCooldown: make(map[uint32]time.Time),
		breakerTrips:    make(map[uint32]int),
		retainedGOPs:    make(map[uint32][]VideoFrame),
	}
}

// A reconnect during a slow pipeline start must attach to the in-flight start.
// Before, a source whose pipeline was still being constructed (running=false)
// was "dead": the reconnect evicted it, stopped it mid-start and queued a
// second pipeline behind the first — the 2026-10-01 incident, ~95s to video.
func TestGetOrCreateReusesSourceThatIsStillStarting(t *testing.T) {
	r := newTestVideoRegistry()
	first := r.GetOrCreate(48, "fakesrc ! appsink name=videosink", GstPipelineOptions{})
	if first.running.Load() {
		t.Fatal("precondition: a fresh source is not running yet")
	}

	again := r.GetOrCreate(48, "fakesrc ! appsink name=videosink", GstPipelineOptions{})
	if again != first {
		t.Fatal("a source whose start has not finished must be reused, not evicted")
	}
	if first.stopped.Load() {
		t.Fatal("the in-flight source must not be stopped")
	}
	if got := r.GetExisting(48); got != first {
		t.Fatal("GetExisting must also treat a starting source as alive")
	}
}

// Dead sources are still replaced: a failed start, or a stopped source.
func TestGetOrCreateReplacesDeadSources(t *testing.T) {
	t.Run("start failed", func(t *testing.T) {
		r := newTestVideoRegistry()
		first := r.GetOrCreate(1, "x", GstPipelineOptions{})
		first.startDone.Store(true) // start finished without running
		if r.GetOrCreate(1, "x", GstPipelineOptions{}) == first {
			t.Fatal("a source whose start failed must be replaced")
		}
	})
	t.Run("stopped", func(t *testing.T) {
		r := newTestVideoRegistry()
		first := r.GetOrCreate(2, "x", GstPipelineOptions{})
		first.stop()
		if r.GetOrCreate(2, "x", GstPipelineOptions{}) == first {
			t.Fatal("a stopped source must be replaced")
		}
	})
}

// A source stopped before its first Subscribe must refuse to build a pipeline
// nobody would ever stop.
func TestStartAfterStopDoesNotBuildPipeline(t *testing.T) {
	r := newTestVideoRegistry()
	src := r.GetOrCreate(3, "videotestsrc ! appsink name=videosink", GstPipelineOptions{})
	src.stop()
	if err := src.start(); err == nil {
		t.Fatal("start after stop must fail")
	}
	if src.pipeline != nil {
		t.Fatal("no pipeline may be created after stop")
	}
}

// A client that subscribed after the stop was scheduled must keep its source.
func TestGracePeriodStopSparesSourceWithClients(t *testing.T) {
	r := newTestVideoRegistry()
	src := r.GetOrCreate(4, "x", GstPipelineOptions{})
	r.ScheduleStop(4) // last client left
	if _, pending := r.pendingStops[4]; !pending {
		t.Fatal("precondition: stop scheduled")
	}
	// A viewer that already held the pointer subscribes now.
	src.clientsMu.Lock()
	src.clients[99] = &sharedVideoClient{id: 99, frameCh: make(chan VideoFrame, 1), errorCh: make(chan error, 1)}
	src.clientsMu.Unlock()

	r.doStop(r.pendingStops[4])

	if src.stopped.Load() {
		t.Fatal("source with an attached client was stopped")
	}
	if r.sources[4] != src {
		t.Fatal("source must be active again")
	}
}

// fakeScanout is an external FrameSource standing in for QEMU's scanout feed.
type fakeScanout struct {
	frames chan VideoFrame
	errs   chan error
}

func newFakeScanout() *fakeScanout {
	return &fakeScanout{frames: make(chan VideoFrame, 64), errs: make(chan error, 1)}
}
func (f *fakeScanout) Frames() <-chan VideoFrame { return f.frames }
func (f *fakeScanout) Errors() <-chan error      { return f.errs }
func (f *fakeScanout) Stop()                     {}

func frameWithPTS(pts uint64, key bool) VideoFrame {
	return VideoFrame{Data: []byte{0, 0, 0, 1, byte(pts)}, PTS: pts, IsKeyframe: key, Timestamp: time.Now()}
}

func receive(t *testing.T, ch <-chan VideoFrame, n int) []uint64 {
	t.Helper()
	var got []uint64
	for len(got) < n {
		select {
		case f := <-ch:
			got = append(got, f.PTS)
		case <-time.After(2 * time.Second):
			t.Fatalf("received %v, want %d frames", got, n)
		}
	}
	return got
}

// waitGOP waits until the source has cached n frames, so the broadcaster has
// processed everything the test sent before the source is stopped.
func waitGOP(t *testing.T, s *SharedVideoSource, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.gopBufferMu.RLock()
		l := len(s.gopBuffer)
		s.gopBufferMu.RUnlock()
		if l == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("GOP never reached %d frames", n)
}

// QEMU's scanout encoder pauses while nobody is subscribed and resumes mid-GOP.
// The next scanout source on the node must replay the previous GOP before the
// resumed delta frames, or the viewer has nothing decodable until the
// encoder's next keyframe (~30s on an idle Helix for Mac desktop).
func TestScanoutSourceReplaysPreviousGOPAfterRestart(t *testing.T) {
	r := newTestVideoRegistry()

	first := newFakeScanout()
	src := r.GetOrCreateWithSource(7, first)
	ch, _, id, err := src.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	first.frames <- frameWithPTS(1, true)
	first.frames <- frameWithPTS(2, false)
	first.frames <- frameWithPTS(3, false)
	receive(t, ch, 3)
	waitGOP(t, src, 3)

	src.Unsubscribe(id)
	r.ScheduleStop(7)           // Unsubscribe schedules on the global registry
	r.doStop(r.pendingStops[7]) // grace period expires

	second := newFakeScanout()
	next := r.GetOrCreateWithSource(7, second)
	if next == src {
		t.Fatal("precondition: a new source after the stop")
	}
	ch2, _, _, err := next.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	second.frames <- frameWithPTS(4, false) // encoder resumes mid-GOP

	got := receive(t, ch2, 4)
	want := []uint64{1, 2, 3, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frames = %v, want %v", got, want)
		}
	}
}

// A GOP with skipped frames can't be continued, and GStreamer sources start on
// a fresh keyframe, so neither is retained.
func TestScanoutGOPRetainedOnlyWhenComplete(t *testing.T) {
	r := newTestVideoRegistry()
	src := r.GetOrCreateWithSource(8, newFakeScanout())
	src.gopBuffer = []VideoFrame{frameWithPTS(1, true), frameWithPTS(2, false)}

	src.gopComplete = false
	r.retainScanoutGOP(8, src)
	if _, ok := r.retainedGOPs[8]; ok {
		t.Fatal("an incomplete GOP must not be retained")
	}

	src.gopComplete = true
	src.gopBuffer = []VideoFrame{frameWithPTS(2, false)}
	r.retainScanoutGOP(8, src)
	if _, ok := r.retainedGOPs[8]; ok {
		t.Fatal("a GOP not starting at a keyframe must not be retained")
	}

	gst := r.GetOrCreate(9, "x", GstPipelineOptions{})
	gst.gopBuffer = []VideoFrame{frameWithPTS(1, true)}
	gst.gopComplete = true
	r.retainScanoutGOP(9, gst)
	if _, ok := r.retainedGOPs[9]; ok {
		t.Fatal("GStreamer sources must not retain a GOP")
	}
}
