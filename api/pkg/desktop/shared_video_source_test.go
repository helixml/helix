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
