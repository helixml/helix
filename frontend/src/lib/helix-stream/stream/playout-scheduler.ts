// Single-cadence adaptive playout scheduler for decoded video frames.
//
// The network (especially WiFi) delivers an evenly-sent stream in bursts (a
// ~80ms gap, then a catch-up burst of frames). Presenting each decoded frame as
// it arrives renders those bursts as judder — and, because decode + a 4K
// drawImage can span several display repaints, replays stale frames as a visible
// "old frame" flicker during drags. This scheduler is the SOLE authority over
// what reaches the canvas: exactly one frame is presented per display repaint,
// chosen to be the freshest appropriate frame, with the rest dropped.
//
// It is adaptive: an idle stream with real arrival jitter builds a small depth
// buffer (smooth passive video, at the cost of latency == depth); any input
// collapses the depth to 0 (low keypress/drag-to-photon latency). At depth 0 it
// presents the newest queued frame each repaint — coalescing bursts with no
// replay. Depth tracks measured receive-interval jitter, not a fixed number.
//
// There is exactly one place that calls `present()` (the rAF tick); interaction
// and discontinuities only mutate state. That single present path is what keeps
// a stale frame from sneaking onto the canvas via a second cadence.

export type PlayoutState = "smoothing" | "interactive" | "idle"

interface QueuedFrame {
  frame: VideoFrame
  ptsUs: number
  arrivalMs: number
}

function closeFrame(frame: VideoFrame) {
  try {
    frame.close()
  } catch {
    /* already closed */
  }
}

export class PlayoutScheduler {
  private queue: QueuedFrame[] = []
  private raf: number | null = null
  private lastTickMs = 0
  private lastPresentMs = 0
  private lastInputMs = 0
  private targetFrames = 0 // desired buffer depth (frames); 0 = low-latency path
  private prevTargetFrames = 0
  private prerolling = false // true while (re)building the buffer to depth
  private decayAccumMs = 0 // accumulator for slow target decay
  private nominalIntervalMs = 1000 / 60 // measured median frame interval (frames<->ms + pacing)
  private depthMs = 0 // effective depth (ms); for stats
  private disposed = false

  private readonly MAX_DELAY_MS = 120 // cap on buffer depth / added latency
  private readonly IDLE_RAMP_MS = 2500 // engage the buffer only after this much input-idle
  private readonly MAX_QUEUE = 30 // safety cap on queued (decoded) frames
  private readonly DEPTH_SLACK = 2 // tolerate target+SLACK depth before a catch-up drop
  private readonly TARGET_DECAY_MS = 2000 // shrink the buffer by at most 1 frame per this interval

  /**
   * @param present  draws exactly one frame to the canvas and closes it (the sole present path)
   * @param onDrop   called once per frame dropped without presenting (for stats)
   * @param getReceiveSamples  recent WebSocket inter-arrival intervals (ms) for jitter-based depth
   */
  constructor(
    private readonly present: (frame: VideoFrame) => void,
    private readonly onDrop: () => void,
    private readonly getReceiveSamples: () => number[],
  ) {}

  /** Entry point from the decoder output callback. */
  push(frame: VideoFrame): void {
    if (this.disposed) {
      closeFrame(frame)
      return
    }
    this.queue.push({ frame, ptsUs: frame.timestamp, arrivalMs: performance.now() })
    while (this.queue.length > this.MAX_QUEUE) {
      const old = this.queue.shift()
      if (old) {
        closeFrame(old.frame)
        this.onDrop()
      }
    }
    this.ensureRunning()
  }

  /** Keyboard/mouse/touch activity: collapse the depth buffer to 0 immediately so
   *  the frame reflecting this input is presented with minimal latency. The next
   *  tick presents the newest queued frame and drops the rest — not drawing here
   *  keeps a single present path. */
  notifyInteraction(): void {
    this.lastInputMs = performance.now()
    if (this.targetFrames > 0 || this.queue.length > 1) {
      this.targetFrames = 0
      this.prevTargetFrames = 0
      this.prerolling = false
      this.decayAccumMs = 0
      this.ensureRunning()
    }
  }

  /** Drop all buffered frames and reset (close/reconfigure/discontinuity). */
  clear(): void {
    for (const item of this.queue) closeFrame(item.frame)
    this.queue = []
    this.targetFrames = 0
    this.prevTargetFrames = 0
    this.prerolling = false
    this.decayAccumMs = 0
    this.depthMs = 0
    if (this.raf !== null) {
      cancelAnimationFrame(this.raf)
      this.raf = null
    }
  }

  /** Permanent teardown — no further frames will be presented. */
  dispose(): void {
    this.disposed = true
    this.clear()
  }

  /** Current effective buffer depth in ms (0 while interacting / no jitter). */
  get bufferMs(): number {
    return Math.round(this.depthMs)
  }

  /** Why the buffer is at its current depth — for the stats overlay. */
  get state(): PlayoutState {
    if (this.depthMs > 0) return "smoothing"
    return performance.now() - this.lastInputMs < this.IDLE_RAMP_MS ? "interactive" : "idle"
  }

  /** Number of decoded frames currently queued (diagnostics). */
  get queueLength(): number {
    return this.queue.length
  }

  private ensureRunning(): void {
    if (this.raf === null) {
      this.lastTickMs = performance.now()
      this.raf = requestAnimationFrame(() => this.tick())
    }
  }

  /** Track the median frame interval and grow/shrink the target depth from the
   *  measured receive jitter. Rises immediately to cover new jitter, decays
   *  slowly (peak-hold) so the depth stays stable — a twitchy target oscillates
   *  into periodic stutter. */
  private updateTarget(now: number, dtMs: number): void {
    const interacting = now - this.lastInputMs < this.IDLE_RAMP_MS
    const s = this.getReceiveSamples()
    let raw = 0
    if (s.length >= 5) {
      const sorted = [...s].sort((a, b) => a - b)
      const at = (q: number) => sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))]
      const median = at(0.5) || this.nominalIntervalMs
      this.nominalIntervalMs = median
      if (!interacting && s.length >= 30) {
        const jitter = at(0.99) - median // worst-case late arrival above the cadence
        // Deadband: only build a buffer for meaningful jitter (> ~half a frame),
        // so a clean connection never engages (and so never stutters).
        if (jitter >= median * 0.5) {
          const maxFrames = Math.max(1, Math.floor(this.MAX_DELAY_MS / median))
          raw = Math.max(1, Math.min(maxFrames, Math.round(jitter / median)))
        }
      }
    }
    if (raw >= this.targetFrames) {
      this.targetFrames = raw
      this.decayAccumMs = 0
    } else {
      this.decayAccumMs += dtMs
      if (this.decayAccumMs >= this.TARGET_DECAY_MS) {
        this.targetFrames = Math.max(raw, this.targetFrames - 1)
        this.decayAccumMs = 0
      }
    }
  }

  /** The single per-repaint present authority. At depth 0 it presents the newest
   *  queued frame and drops the rest (coalescing bursts); at depth >0 it presents
   *  the oldest, paced to the source rate, with a bounded catch-up drop. Tied to
   *  the display refresh (rAF), so there is no PTS phase-walk against vsync. */
  private tick(): void {
    this.raf = null
    if (this.disposed) {
      this.clear()
      return
    }
    const now = performance.now()
    const dtMs = Math.max(0, now - this.lastTickMs)
    this.lastTickMs = now

    this.updateTarget(now, dtMs)
    const target = this.targetFrames
    if (target > this.prevTargetFrames) this.prerolling = true
    this.prevTargetFrames = target

    const q = this.queue
    if (q.length === 0) {
      // Underflow: rebuild before resuming so we ride over the next stall too.
      // Loop stops here; restarts on the next push().
      if (target > 0) this.prerolling = true
      this.depthMs = target * this.nominalIntervalMs
      return
    }

    if (target === 0) {
      // Low-latency path: present the newest frame this repaint, drop the rest.
      const newest = q.pop() as QueuedFrame
      for (const item of q) {
        closeFrame(item.frame)
        this.onDrop()
      }
      q.length = 0
      this.prerolling = false
      this.present(newest.frame)
      this.lastPresentMs = now
    } else {
      if (this.prerolling && q.length > target) this.prerolling = false
      // Steady state: present one (oldest) frame, paced to the source rate (once
      // per source-frame regardless of the panel's refresh rate). While
      // prerolling we hold to let the buffer fill (a one-time pause when it
      // engages or after an underflow, not a per-frame cost).
      const paceReady = now - this.lastPresentMs >= this.nominalIntervalMs * 0.75
      if (!this.prerolling && paceReady) {
        const item = q.shift() as QueuedFrame
        this.present(item.frame)
        this.lastPresentMs = now
        // Catch-up: bound latency if the queue ran deeper than target+slack
        // (clock drift, or a burst after a stall). Rare single drops.
        while (q.length > target + this.DEPTH_SLACK) {
          const old = q.shift() as QueuedFrame
          closeFrame(old.frame)
          this.onDrop()
        }
      }
    }

    this.depthMs = (this.prerolling ? target : this.queue.length) * this.nominalIntervalMs
    if (this.queue.length > 0) {
      this.raf = requestAnimationFrame(() => this.tick())
    }
  }
}
