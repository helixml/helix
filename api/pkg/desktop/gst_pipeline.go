//go:build cgo && linux

// Package desktop provides GStreamer pipeline management using go-gst bindings.
// This replaces the UDP-based gst-launch subprocess approach with native Go bindings
// for in-order frame delivery from appsink.
package desktop

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
)

// ============================================================================
// go-gst object ownership
// ============================================================================
//
// Every go-gst constructor hands back a Go wrapper that owns a C reference and
// releases it from a runtime finalizer:
//
//	glib.Take / TransferNone  ref_sinks a floating object (or refs a non-floating
//	                          one) => net refcount 1, owned by the Go wrapper
//	glib.TransferFull         adopts the caller's ref, owned by the Go wrapper
//
// Relying on those finalizers is what leaked the GPU. Go's GC sizes an object by
// its Go footprint — a few hundred bytes — while the C object behind it pins a
// CUDA context, an NVENC session and a handful of /dev/nvidia0 fds. There is
// never enough Go heap pressure to make the collector care, so the finalizers
// effectively never run and the GPU resources accumulate forever.
//
// So: this package owns every go-gst object it acquires and releases all of them
// explicitly. The two helpers below are the only correct way to do that. Both
// disarm the finalizer *before* dropping the ref — otherwise the collector runs
// g_object_unref a second time on memory we already freed, which is a
// use-after-free (observed as "g_object_unref: assertion 'G_IS_OBJECT (object)'
// failed" followed by SIGSEGV inside runtime.runFinalizers).
//
// The finalizer for GstObject-derived types (pipeline, element, bus, clock) lives
// on the embedded *glib.Object, so that exact pointer has to reach SetFinalizer;
// passing the outer *gst.Pipeline silently does nothing. GstSample and GstBuffer
// are not GObjects and carry their finalizer on the go-gst wrapper itself.

// releaseGObject drops our reference to a GstObject-derived wrapper immediately.
// obj must be the embedded *glib.Object — see gobjectOf.
func releaseGObject(obj *glib.Object) {
	if obj == nil {
		return
	}
	runtime.SetFinalizer(obj, nil)
	obj.Unref()
}

// gobjectOf digs the embedded *glib.Object out of a gst.Object. Every
// GstObject-derived go-gst type embeds one of these, and it is where go-glib
// installs the finalizer.
func gobjectOf(o *gst.Object) *glib.Object {
	if o == nil || o.InitiallyUnowned == nil {
		return nil
	}
	return o.InitiallyUnowned.Object
}

// releaseSample drops our reference to a GstSample immediately. Samples come out
// of appsink at the frame rate, each one holding a GstBuffer and its GstMemory —
// on the zero-copy path that memory is CUDA memory backed by a DMA-BUF fd, so
// leaving these to the GC is what kept whole CUDA contexts alive after their
// pipeline was gone.
func releaseSample(s *gst.Sample) {
	if s == nil {
		return
	}
	runtime.SetFinalizer(s, nil)
	s.Unref()
}

// releaseBuffer drops our reference to a GstBuffer immediately.
// Sample.GetBuffer() is documented as "unsafe none" but go-gst takes a ref and
// arms a finalizer anyway, so the caller owns a reference whether it wants one or
// not.
func releaseBuffer(b *gst.Buffer) {
	if b == nil {
		return
	}
	runtime.SetFinalizer(b, nil)
	b.Unref()
}

// gstInitOnce ensures GStreamer is initialized only once
var gstInitOnce sync.Once

// pipelineCreateMu serializes pipeline creation to prevent race conditions
// when multiple clients connect simultaneously to the same PipeWire node.
// The pipewirezerocopysrc element's CUDA context and PipeWire stream setup
// can fail if multiple instances try to initialize concurrently.
// NOTE: With SharedVideoSource, only ONE pipeline is created per PipeWire node,
// so this mutex mainly protects against the rare case of concurrent session starts.
var pipelineCreateMu sync.Mutex

// InitGStreamer initializes the GStreamer library. Safe to call multiple times.
func InitGStreamer() {
	gstInitOnce.Do(func() {
		gst.Init(nil)
	})
}

// VideoFrame represents a video frame from the GStreamer pipeline
type VideoFrame struct {
	Data       []byte    // H.264 NAL units (Annex B format with start codes)
	PTS        uint64    // Presentation timestamp in microseconds
	IsKeyframe bool      // True if this is an IDR frame
	IsReplay   bool      // True if this is a GOP replay frame (decoder warmup, don't display)
	Timestamp  time.Time // Wall clock time when frame was received
}

// GstPipelineOptions configures the GStreamer pipeline
type GstPipelineOptions struct {
	// UseRealtimeClock forces the pipeline to use a realtime (wall clock) based clock.
	// When enabled, do-timestamp=true on source elements will produce PTS values
	// that are relative to pipeline start but based on wall clock time.
	// This is useful for latency measurement when comparing PTS to time.Now().
	UseRealtimeClock bool
}

// GstPipeline wraps a GStreamer pipeline with appsink for video capture
type GstPipeline struct {
	pipeline *gst.Pipeline
	appsink  *app.Sink
	// appsinkElem is the *gst.Element returned by GetElementByName. That call is
	// transfer-full, so we own a reference on the appsink that has to be released
	// explicitly; app.SinkFromElement is only a cast and does not take another.
	appsinkElem *gst.Element
	// bus is the pipeline bus, acquired once in Start(). GetPipelineBus is
	// transfer-full and caches the wrapper on the Pipeline, so we own exactly one
	// reference no matter how often it is called.
	bus *gst.Bus
	// busDone is closed by watchBus when it returns, so Stop() can be sure nobody
	// is still polling the bus before releasing it.
	busDone       chan struct{}
	frameCh       chan VideoFrame
	errorCh       chan error // Channel for pipeline errors (GPU OOM, encoder failures, etc.)
	running       atomic.Bool
	stopOnce      sync.Once
	pipelineID    string     // For logging
	realtimeClock *gst.Clock // Custom clock, if one was forced onto the pipeline

	// baseTimeNs is the pipeline's base_time in nanoseconds since epoch (only valid with realtime clock).
	// Used to convert PTS (running time) to wall clock: captureTime = baseTimeNs + PTS
	baseTimeNs uint64
	// useRealtimeClock indicates if the pipeline is using a realtime clock for latency calculation
	useRealtimeClock bool

	// Frame drop tracking for diagnostics
	framesReceived atomic.Uint64 // Frames received from appsink
	framesDropped  atomic.Uint64 // Frames dropped due to full channel

	// Encoder-output cadence measurement (appsink callback, BEFORE the Go
	// channels) — isolates encoder jitter from Go-side channel/scheduling jitter.
	// Only touched in onNewSample (single GStreamer streaming thread), no lock.
	appsinkLastSample time.Time
	appsinkIntervalUs []int64
	appsinkLastLog    time.Time
}

// NewGstPipeline creates a new GStreamer pipeline from a pipeline string.
// The pipeline string must end with an appsink element named "videosink".
//
// Example pipeline:
//
//	pipewiresrc path=47 ! nvh264enc ! h264parse ! appsink name=videosink
func NewGstPipeline(pipelineStr string) (*GstPipeline, error) {
	return NewGstPipelineWithOptions(pipelineStr, GstPipelineOptions{})
}

// NewGstPipelineWithOptions creates a new GStreamer pipeline with custom options.
// The pipeline string must end with an appsink element named "videosink".
func NewGstPipelineWithOptions(pipelineStr string, opts GstPipelineOptions) (*GstPipeline, error) {
	InitGStreamer()

	// Serialize pipeline creation to prevent race conditions when multiple
	// clients connect simultaneously. This is especially important for
	// pipewirezerocopysrc which initializes CUDA context and PipeWire streams.
	pipelineCreateMu.Lock()
	defer pipelineCreateMu.Unlock()

	// Parse the pipeline string
	pipeline, err := gst.NewPipelineFromString(pipelineStr)
	if err != nil {
		// GStreamer gives misleading errors when GPU encoder plugins fail to load.
		// For example, if CUDA context creation fails (BAR1 memory exhaustion from
		// too many GPU processes), nvh264enc can't be instantiated and GStreamer
		// reports "no property X in element Y" instead of the actual CUDA error.
		// Detect this and provide an actionable error message.
		if diagErr := diagnoseGPUEncoderFailure(pipelineStr, err); diagErr != nil {
			return nil, diagErr
		}
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	// Get the appsink element
	elem, err := pipeline.GetElementByName("videosink")
	if err != nil {
		pipeline.SetState(gst.StateNull)
		releaseGObject(gobjectOf(pipeline.Object))
		return nil, fmt.Errorf("failed to get videosink element: %w", err)
	}

	appsink := app.SinkFromElement(elem)
	if appsink == nil {
		releaseGObject(gobjectOf(elem.Object))
		pipeline.SetState(gst.StateNull)
		releaseGObject(gobjectOf(pipeline.Object))
		return nil, fmt.Errorf("videosink element is not an appsink")
	}

	g := &GstPipeline{
		pipeline:    pipeline,
		appsink:     appsink,
		appsinkElem: elem,
		frameCh:     make(chan VideoFrame, 8), // Buffer a few frames
		errorCh:     make(chan error, 1),      // Buffer 1 error (only care about first fatal error)
		pipelineID:  fmt.Sprintf("gst-%p", pipeline),
	}

	// Force the pipeline to use a realtime clock if requested.
	// This makes do-timestamp=true use wall clock time instead of monotonic time,
	// enabling accurate latency measurement by comparing PTS to time.Now().
	if opts.UseRealtimeClock {
		clock, err := gst.NewSystemClock(gst.ClockTypeRealtime)
		if err != nil {
			g.Stop()
			return nil, fmt.Errorf("failed to create realtime clock: %w", err)
		}
		pipeline.ForceClock(clock.Clock)
		g.realtimeClock = clock.Clock // Keep reference to prevent GC
		g.useRealtimeClock = true
		fmt.Printf("[GST_PIPELINE] Using realtime clock for wall clock timestamps\n")
	}

	return g, nil
}

// Start begins the pipeline and frame delivery.
// Frames can be received from the Frames() channel.
func (g *GstPipeline) Start(ctx context.Context) error {
	if g.running.Load() {
		return nil
	}

	// Serialize pipeline start to prevent race conditions when multiple
	// clients connect simultaneously. pipewirezerocopysrc initializes
	// CUDA context and PipeWire streams during state transition to PLAYING.
	pipelineCreateMu.Lock()
	defer pipelineCreateMu.Unlock()

	// Track how many pipelines are being started for logging
	currentCount := activePipelineCount.Load()
	fmt.Printf("[GST_PIPELINE] Starting pipeline %s (active pipelines: %d)\n", g.pipelineID, currentCount)

	// Configure appsink properties
	g.appsink.SetProperty("emit-signals", true)
	g.appsink.SetProperty("max-buffers", uint(2))
	g.appsink.SetProperty("drop", true)
	g.appsink.SetProperty("sync", false)

	// Set up the new-sample callback
	g.appsink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: g.onNewSample,
	})

	// Start the pipeline
	if err := g.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to playing: %w", err)
	}

	// Capture base_time when using realtime clock for PTS→wall clock conversion
	// base_time is the clock time (nanoseconds since epoch for realtime clock) when pipeline started
	if g.useRealtimeClock {
		baseTime := g.pipeline.GetBaseTime()
		g.baseTimeNs = uint64(baseTime)
		fmt.Printf("[GST_PIPELINE] Captured base_time: %d ns (epoch: %s)\n",
			g.baseTimeNs, time.Unix(0, int64(g.baseTimeNs)).Format(time.RFC3339Nano))
	}

	g.running.Store(true)
	newCount := activePipelineCount.Add(1)
	fmt.Printf("[GST_PIPELINE] Pipeline %s started (active pipelines: %d)\n", g.pipelineID, newCount)

	// Acquire the bus here rather than inside watchBus so its lifetime is owned by
	// the pipeline, not by whichever goroutine happened to touch it first.
	// GetPipelineBus is transfer-full: this reference is ours to release in Stop().
	g.bus = g.pipeline.GetPipelineBus()

	// Monitor for EOS and errors. busDone lets Stop() wait for this goroutine to
	// stop touching the bus before the bus is released.
	g.busDone = make(chan struct{})
	go g.watchBus(ctx)

	return nil
}

// onNewSample is called when appsink has a new sample available
func (g *GstPipeline) onNewSample(sink *app.Sink) gst.FlowReturn {
	if !g.running.Load() {
		return gst.FlowEOS
	}

	// Encoder-output interval (appsink callback, before any Go channel). This is
	// the cadence the encoder produces frames at — compare to B.create (encoder
	// input) to isolate encoder jitter, and to the Go send loop to isolate
	// Go-channel jitter.
	{
		now := time.Now()
		if !g.appsinkLastSample.IsZero() {
			g.appsinkIntervalUs = append(g.appsinkIntervalUs, now.Sub(g.appsinkLastSample).Microseconds())
		}
		g.appsinkLastSample = now
		if g.appsinkLastLog.IsZero() {
			g.appsinkLastLog = now
		}
		if now.Sub(g.appsinkLastLog) >= 5*time.Second && len(g.appsinkIntervalUs) > 0 {
			p50, p95, p99, mx, burst := percentilesMsFromUs(g.appsinkIntervalUs)
			fmt.Printf("[METRIC] ENC.appsink  n=%d p50=%d p95=%d p99=%d max=%d burst<8ms=%d\n",
				len(g.appsinkIntervalUs), p50, p95, p99, mx, burst)
			g.appsinkIntervalUs = g.appsinkIntervalUs[:0]
			g.appsinkLastLog = now
		}
	}

	sample := sink.PullSample()
	if sample == nil {
		return gst.FlowOK
	}
	// PullSample is transfer-full and GetBuffer takes a ref of its own. Both are
	// released here rather than by the GC: on the zero-copy path each buffer holds
	// CUDA memory imported from a DMA-BUF, so a deferred release means a
	// /dev/nvidia0 fd and megabytes of GPU memory held for an unbounded time —
	// long enough to keep the whole CUDA context alive after the pipeline is gone.
	// Deferred in acquisition order so the LIFO unwind is unmap, buffer, sample.
	defer releaseSample(sample)

	buffer := sample.GetBuffer()
	if buffer == nil {
		return gst.FlowOK
	}
	defer releaseBuffer(buffer)

	// Map buffer to read data
	mapInfo := buffer.Map(gst.MapRead)
	if mapInfo == nil {
		return gst.FlowOK
	}
	defer buffer.Unmap()

	// Copy the data (buffer is only valid during this callback)
	data := make([]byte, len(mapInfo.Bytes()))
	copy(data, mapInfo.Bytes())

	// Get presentation timestamp (ClockTime is nanoseconds, convert to microseconds)
	// ClockTime.AsDuration() returns *time.Duration (nil if invalid/GST_CLOCK_TIME_NONE)
	// PTS = 0 is valid for the first frame, only nil is invalid
	ptsDur := buffer.PresentationTimestamp().AsDuration()
	var pts uint64
	var ptsNs int64
	if ptsDur != nil {
		pts = uint64(ptsDur.Microseconds())
		ptsNs = int64(*ptsDur) // Duration in nanoseconds
	} else {
		// No buffer PTS (GST_CLOCK_TIME_NONE). This happens in real capture paths:
		// ext-image-copy-capture provides no compositor timestamp, and the zerocopy
		// path's set_pts() in pipewiresrc/imp.rs is silently skipped when the pooled
		// GstBuffer is not uniquely owned (buffer.get_mut() == None). Without a PTS
		// the browser feeds timestamp=0 into every EncodedVideoChunk, so every
		// decoded VideoFrame has timestamp 0 and the client's PlayoutScheduler can
		// neither order/dedupe frames nor measure timing — bursty/out-of-order
		// delivery (e.g. under CPU load) then shows stale/out-of-order frames.
		//
		// Synthesize a monotonic wall-clock PTS. The appsink callback runs in
		// pipeline (capture) order, so time.Now() here is monotonic and reflects
		// true frame order; UnixMicro() is the same unit/scale the zerocopy path
		// emits when its set_pts DOES run (wall-clock microseconds), so the client's
		// drift math stays consistent. Note: a *valid* PTS of 0 (first frame of a
		// running-time stream) keeps ptsDur != nil and is intentionally left alone.
		pts = uint64(time.Now().UnixMicro())
	}

	// Check if this is a keyframe
	// GST_BUFFER_FLAG_DELTA_UNIT is set for non-keyframes
	isKeyframe := !buffer.HasFlags(gst.BufferFlagDeltaUnit)

	// Calculate capture wall clock time for encoder latency measurement
	// There are two cases:
	// 1. pipewirezerocopysrc: PTS is wall clock nanoseconds since epoch (very large, ~1.7e18 for 2024)
	// 2. native pipewiresrc with realtime clock: baseTimeNs + PTS = wall clock
	// 3. Fallback: use time.Now() (appsink receive time)
	var captureTime time.Time
	// Check if PTS looks like wall clock (> year 2020 in nanoseconds = 1.577e18)
	const minWallClockNs = int64(1577836800000000000) // 2020-01-01 00:00:00 UTC
	if ptsNs > minWallClockNs {
		// PTS is already wall clock nanoseconds (from pipewirezerocopysrc)
		captureTime = time.Unix(0, ptsNs)
	} else if g.useRealtimeClock && g.baseTimeNs > 0 && ptsNs >= 0 {
		// PTS is running time, convert using base_time (from native pipewiresrc with realtime clock)
		captureTimeNs := int64(g.baseTimeNs) + ptsNs
		captureTime = time.Unix(0, captureTimeNs)
	} else {
		// Fallback: use current time (only measures Go processing time, not encoder latency)
		captureTime = time.Now()
	}

	frame := VideoFrame{
		Data:       data,
		PTS:        pts,
		IsKeyframe: isKeyframe,
		Timestamp:  captureTime,
	}

	// Non-blocking send to avoid blocking the GStreamer thread
	g.framesReceived.Add(1)
	select {
	case g.frameCh <- frame:
	default:
		// Drop frame if channel is full (low latency preference)
		dropped := g.framesDropped.Add(1)
		received := g.framesReceived.Load()
		// Log every 100th drop to avoid log spam
		if dropped <= 10 || dropped%100 == 0 {
			fmt.Printf("[GST_PIPELINE] Frame dropped (channel full): %d dropped / %d received (%.1f%%)\n",
				dropped, received, float64(dropped)*100.0/float64(received))
		}
	}

	return gst.FlowOK
}

// watchBus monitors the GStreamer bus for errors and EOS
func (g *GstPipeline) watchBus(ctx context.Context) {
	// Signals Stop() that nothing is polling the bus any more, so it is safe to
	// release it.
	defer close(g.busDone)

	bus := g.bus
	if bus == nil {
		return
	}

	for g.running.Load() {
		select {
		case <-ctx.Done():
			g.stop(true)
			return
		default:
		}

		// Poll with timeout to allow context checking
		// ClockTime is in nanoseconds, so 100ms = 100_000_000ns
		msg := bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
		if msg == nil {
			continue
		}

		// Explicitly free the message before returning or looping. Without this,
		// the *Message sits on the GC heap until a finalizer eventually runs
		// gst_message_unref — and if the pipeline has been torn down in the
		// meantime, the C side aborts ("terminate called without an active
		// exception") because the message references elements that are no
		// longer in a valid state. Deterministic Unref keeps the C-side
		// lifetimes tight.
		//
		// CRITICAL: bus.TimedPop returns a *Message wrapped via go-gst's
		// FromGstMessageUnsafeFull — the wrapper does NOT take an extra ref
		// (the C call already transferred one) but DOES install a finalizer
		// that calls gst_message_unref. If we just call msg.Unref() we drop
		// the only ref to zero and free the C struct, then later the GC
		// finalizer runs gst_message_unref a second time on freed memory —
		// either tripping the "REFCOUNT_VALUE > 0" assertion or, worse,
		// corrupting the heap once GStreamer reuses the slot. Disarm the
		// finalizer first so only our explicit Unref decrements the refcount.
		g.handleBusMessage(msg)
		runtime.SetFinalizer(msg, nil)
		msg.Unref()
		if !g.running.Load() {
			return
		}
	}
}

// handleBusMessage processes a single bus message. Returns after Stop() if the
// message is EOS or fatal Error; the caller is responsible for unref'ing the
// message and observing g.running to break the loop.
func (g *GstPipeline) handleBusMessage(msg *gst.Message) {
	switch msg.Type() {
	case gst.MessageEOS:
		g.stop(true)
	case gst.MessageError:
		gerr := msg.ParseError()
		if gerr != nil {
			// Log error with full debug info - helps diagnose pipeline failures
			errMsg := gerr.Error()
			debugStr := gerr.DebugString()
			fmt.Printf("[GST_PIPELINE] Error: %s\n", errMsg)
			if debugStr != "" {
				fmt.Printf("[GST_PIPELINE] Debug: %s\n", debugStr)
			}
			// Log the element that produced the error
			srcName := msg.Source()
			if srcName != "" {
				fmt.Printf("[GST_PIPELINE] Source: %s\n", srcName)
			}

			// Create a user-friendly error message for common failures, with the
			// raw technical detail appended so it can be surfaced in the UI.
			userErr := g.createUserFriendlyError(errMsg, debugStr, srcName)
			// Non-blocking send to error channel (only first error matters)
			select {
			case g.errorCh <- userErr:
				fmt.Printf("[GST_PIPELINE] Error sent to error channel: %s\n", userErr.Error())
			default:
				// Channel full - first error already captured
			}
		}
		g.stop(true)
	case gst.MessageWarning:
		gwarn := msg.ParseWarning()
		if gwarn != nil {
			fmt.Printf("[GST_PIPELINE] Warning: %s\n", gwarn.Error())
			if debugStr := gwarn.DebugString(); debugStr != "" {
				fmt.Printf("[GST_PIPELINE] Warning Debug: %s\n", debugStr)
			}
		}
	case gst.MessageStateChanged:
		// Could log state changes if needed for debugging
	}
}

// Frames returns a channel that receives video frames.
// The channel is closed when the pipeline stops.
func (g *GstPipeline) Frames() <-chan VideoFrame {
	return g.frameCh
}

// Stop stops the pipeline and closes the frame channel.
//
// Every GStreamer object this pipeline acquired is released here, explicitly and
// in reverse acquisition order, so the C-side objects — and with them the CUDA
// context, NVENC session and DMA-BUF fds — are gone by the time Stop() returns.
// Nothing is left to a GC finalizer: see the ownership notes at the top of this
// file for why that is not an option for GPU-backed objects.
func (g *GstPipeline) Stop() {
	g.stop(false)
}

// stop is the implementation of Stop. fromBusGoroutine is true when called from
// watchBus (on EOS, a fatal error, or context cancellation); in that case we must
// not wait for watchBus to finish, because we *are* watchBus.
func (g *GstPipeline) stop(fromBusGoroutine bool) {
	g.stopOnce.Do(func() {
		// Only decrement active count if Start() had succeeded (set running=true).
		// Without this check, Stop() on a pipeline that failed to start would
		// drive the counter negative.
		wasRunning := g.running.Swap(false)

		// watchBus polls the bus with a 100ms timeout and exits once running is
		// false. Wait for it before releasing the bus, otherwise the poll would
		// touch freed memory.
		if !fromBusGoroutine && g.busDone != nil {
			select {
			case <-g.busDone:
			case <-time.After(5 * time.Second):
				fmt.Printf("[GST_PIPELINE] WARNING: bus watcher for %s did not exit within 5s, releasing bus anyway\n", g.pipelineID)
			}
		}

		if g.pipeline != nil {
			// SetState(Null) is async — child elements (nvh264enc, pipewiresrc, etc.)
			// may still be in PLAYING when it returns. We must wait for the state
			// change to propagate before Unref, otherwise NVIDIA's encoder lib
			// calls abort() when disposed in PLAYING state.
			// Use a 5s timeout to avoid deadlock if the pipeline is stuck.
			g.pipeline.SetState(gst.StateNull)
			ret, _ := g.pipeline.GetState(gst.StateNull, gst.ClockTime(5*time.Second))
			if ret != gst.StateChangeSuccess {
				// Pipeline is stuck — Unref in this state would crash (nvh264enc
				// abort()s when disposed while PLAYING). Exit the process and let
				// the restart loop in start-desktop-bridge.sh bring us back clean.
				fmt.Printf("[GST_PIPELINE] FATAL: pipeline stuck (GetState returned %v after 5s), exiting to let restart loop recover\n", ret)
				os.Exit(1)
			}
		}

		// Clear the appsink callbacks. go-gst stashes the SinkCallbacks struct in a
		// process-global map (gopointer.Save) and only drops it when GStreamer
		// invokes the GDestroyNotify — which happens when the callbacks are replaced
		// or the appsink is disposed. Replacing them here releases the entry, and
		// with it the closure over this *GstPipeline.
		if g.appsink != nil {
			g.appsink.SetCallbacks(&app.SinkCallbacks{})
		}

		// Release the bus. Flushing first drops any messages still queued; every one
		// of them holds a reference on the element that posted it, which on a real
		// capture pipeline means nvh264enc and pipewirezerocopysrc.
		if g.bus != nil {
			g.bus.SetFlushing(true)
			releaseGObject(gobjectOf(g.bus.Object))
			g.bus = nil
		}

		// Release the appsink reference taken by GetElementByName (transfer-full).
		if g.appsinkElem != nil {
			releaseGObject(gobjectOf(g.appsinkElem.Object))
			g.appsinkElem = nil
		}
		g.appsink = nil

		// Release the forced clock, if we created one.
		if g.realtimeClock != nil {
			releaseGObject(gobjectOf(g.realtimeClock.Object))
			g.realtimeClock = nil
		}

		// Finally the pipeline itself. gst_parse_launch returns a floating
		// reference that go-gst ref-sinks, so this is the last one and dropping it
		// disposes the pipeline and every element in it.
		if g.pipeline != nil {
			releaseGObject(gobjectOf(g.pipeline.Object))
			g.pipeline = nil
		}

		if wasRunning {
			remaining := activePipelineCount.Add(-1)
			fmt.Printf("[GST_PIPELINE] Pipeline %s stopped (active pipelines: %d)\n", g.pipelineID, remaining)
		} else {
			fmt.Printf("[GST_PIPELINE] Pipeline %s cleaned up (was never started)\n", g.pipelineID)
		}

		close(g.frameCh)
	})
}

// IsRunning returns whether the pipeline is currently running.
func (g *GstPipeline) IsRunning() bool {
	return g.running.Load()
}

// GetFrameStats returns frame receive and drop counts for diagnostics.
func (g *GstPipeline) GetFrameStats() (received, dropped uint64) {
	return g.framesReceived.Load(), g.framesDropped.Load()
}

// Errors returns a channel that receives pipeline errors.
// Only fatal errors are sent (e.g., GPU OOM, encoder failures).
// The channel is buffered with size 1 - only the first error is captured.
func (g *GstPipeline) Errors() <-chan error {
	return g.errorCh
}

// createUserFriendlyError converts GStreamer error messages into user-friendly
// text, then appends a compact technical-detail block after a blank-line
// delimiter ("\n\n"). The frontend renders the first paragraph prominently and
// everything after the delimiter as smaller, monospaced technical detail, so the
// underlying failure (element, raw error, GStreamer debug string) is diagnosable
// from the UI instead of being flattened to a generic sentence.
func (g *GstPipeline) createUserFriendlyError(errMsg, debugStr, srcElement string) error {
	friendly := friendlyVideoError(errMsg)

	// Assemble the technical detail: source element + raw error, plus the
	// GStreamer debug string, which carries the element path and reason code
	// (e.g. "gst_base_src_loop (): ... reason error (-5)") or — once the
	// zero-copy source posts a descriptive error — the concrete cause such as a
	// first-frame timeout.
	tech := strings.TrimSpace(errMsg)
	if srcElement != "" {
		tech = srcElement + ": " + tech
	}
	if d := strings.TrimSpace(debugStr); d != "" {
		tech = tech + "\n" + d
	}
	const maxTech = 600
	if len(tech) > maxTech {
		tech = tech[:maxTech] + "…"
	}
	if tech == "" {
		return fmt.Errorf("%s", friendly)
	}
	return fmt.Errorf("%s\n\n%s", friendly, tech)
}

// friendlyVideoError maps common GStreamer/NVENC errors to a short human-readable
// sentence. The technical detail is appended separately by createUserFriendlyError.
func friendlyVideoError(errMsg string) string {
	switch {
	case containsIgnoreCase(errMsg, "NV_ENC_ERR_OUT_OF_MEMORY") || containsIgnoreCase(errMsg, "out of memory"):
		return "GPU out of memory - too many sessions running. Please close some browser tabs or stop unused sessions."
	case containsIgnoreCase(errMsg, "NV_ENC_ERR_NO_ENCODE_DEVICE"):
		return "No GPU encoder available. The GPU may be in use by another process."
	case containsIgnoreCase(errMsg, "NV_ENC_ERR"):
		return "GPU encoder error. Try closing other sessions."
	case containsIgnoreCase(errMsg, "Could not get EOS"):
		return "Video pipeline stopped unexpectedly. Please try reconnecting."
	case containsIgnoreCase(errMsg, "Resource not found"):
		return "Video source not available. The session may have ended."
	case containsIgnoreCase(errMsg, "Permission denied"):
		return "Permission denied accessing video source."
	default:
		// Includes the first-frame-timeout case and the generic "Internal data
		// stream error" — the specific cause rides along in the technical detail.
		return "Video streaming error. Please try reconnecting."
	}
}

// containsIgnoreCase is a case-insensitive substring check
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// CheckGstElement checks if a GStreamer element is available.
// Returns true if the element factory exists.
func CheckGstElement(element string) bool {
	InitGStreamer()
	factory := gst.Find(element)
	return factory != nil
}

// gpuEncoderElements lists GStreamer elements that require a CUDA/GPU context to load.
var gpuEncoderElements = []string{
	"nvh264enc", "nvh265enc", "nvav1enc",
	"nvautogpuh264enc", "nvautogpuh265enc", "nvautogpuav1enc",
}

// diagnoseGPUEncoderFailure checks whether a pipeline parse failure is actually caused
// by a GPU encoder element failing to load (e.g., CUDA context creation failure from
// BAR1 memory exhaustion). Returns a user-friendly error if so, nil otherwise.
func diagnoseGPUEncoderFailure(pipelineStr string, parseErr error) error {
	for _, elemName := range gpuEncoderElements {
		if !strings.Contains(pipelineStr, elemName) {
			continue
		}
		// Pipeline references a GPU encoder — try to instantiate it directly.
		// gst_element_factory_make triggers CUDA context init; if that fails
		// (OOM, BAR1 exhaustion, driver issue), the element returns nil.
		testElem, err := gst.NewElement(elemName)
		if err != nil {
			return fmt.Errorf("GPU encoder %q is unavailable — CUDA context creation failed. "+
				"This usually means GPU BAR1 memory is exhausted from too many concurrent "+
				"desktop sessions (each session's compositor and apps consume BAR1 slots). "+
				"Stop unused sessions to free GPU resources. "+
				"(GStreamer error: %v)", elemName, parseErr)
		}
		// Element created fine — issue is something else (e.g., genuinely bad property name).
		// Clean up the test element. SetState alone is not enough: the element holds
		// a CUDA context, and this runs on every parse failure — i.e. exactly when
		// the GPU is already under pressure — so leaking it here compounds the very
		// problem being diagnosed.
		testElem.SetState(gst.StateNull)
		releaseGObject(gobjectOf(testElem.Object))
		return nil
	}
	return nil
}
