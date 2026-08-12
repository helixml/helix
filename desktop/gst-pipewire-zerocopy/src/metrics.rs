//! Measure-only pipeline instrumentation.
//!
//! Process-global histograms (there is exactly one capture stream per
//! desktop-bridge process), logged via `eprintln!` every ~5s with the
//! `[METRIC]` prefix. NOTHING here changes pipeline behaviour — it only
//! observes. The goal is to localise where frame-delivery bursts form
//! (a burst = an inter-frame interval below ~8ms, which is physically
//! impossible as a freshly-rendered frame at 60Hz and therefore means a
//! queue piled up and then drained).

use parking_lot::Mutex;
use std::time::Instant;

/// Rolling window of samples. 600 ≈ 10s at 60fps.
const WINDOW: usize = 600;
/// Emit a log line at most this often.
const LOG_INTERVAL_SECS: u64 = 5;
/// Intervals strictly below this are counted as "bursts" (pileup drain).
const BURST_THRESHOLD_US: u32 = 8_000;

/// Fixed-capacity ring buffer of microsecond samples with percentile + burst
/// summarisation. `const`-constructible so it can live in a `static`.
pub struct Hist {
    buf: [u32; WINDOW],
    len: usize,
    idx: usize,
}

impl Hist {
    pub const fn new() -> Self {
        Self { buf: [0; WINDOW], len: 0, idx: 0 }
    }

    pub fn record(&mut self, v_us: u32) {
        self.buf[self.idx] = v_us;
        self.idx = (self.idx + 1) % WINDOW;
        if self.len < WINDOW {
            self.len += 1;
        }
    }

    pub fn clear(&mut self) {
        self.len = 0;
        self.idx = 0;
    }

    /// (n, p50, p95, p99, max, burst_count, avg) — all µs except counts.
    pub fn summary(&self) -> (usize, u32, u32, u32, u32, usize, u32) {
        if self.len == 0 {
            return (0, 0, 0, 0, 0, 0, 0);
        }
        let mut s: Vec<u32> = self.buf[..self.len].to_vec();
        s.sort_unstable();
        let n = self.len;
        let pct = |p: usize| s[(n * p / 100).min(n - 1)];
        let sum: u64 = s.iter().map(|&x| x as u64).sum();
        // sorted ascending → all sub-threshold samples are at the front
        let burst = s.iter().take_while(|&&x| x < BURST_THRESHOLD_US).count();
        (n, pct(50), pct(95), pct(99), s[n - 1], burst, (sum / n as u64) as u32)
    }

    fn fmt_us(&self) -> String {
        let (n, p50, p95, p99, max, burst, avg) = self.summary();
        format!(
            "n={} avg={} p50={} p95={} p99={} max={} burst<8ms={} ({}us)",
            n,
            avg / 1000,
            p50 / 1000,
            p95 / 1000,
            p99 / 1000,
            max / 1000,
            burst,
            avg
        )
    }
}

/// Producer-side (PipeWire thread) metrics, all updated from the single
/// `.process()` callback (plus `process_dmabuf_to_cuda`, same thread).
pub struct ProducerMetrics {
    /// Point A: inter-arrival of frames from Mutter/PipeWire at `.process()`.
    arrival: Hist,
    last_arrival: Option<Instant>,
    /// How long we hold Mutter's DMA-BUF (dequeue → queue_raw_buffer),
    /// which includes the synchronous CUDA copy. Long/variable = we may be
    /// starving Mutter's buffer pool.
    hold: Hist,
    /// CUDA stage breakdown (EGL import / register / copy / whole fn).
    egl: Hist,
    cuda_reg: Hist,
    copy: Hist,
    cuda_total: Hist,
    /// Bounded(8) channel to the GStreamer thread.
    chan_depth_max: usize,
    chan_depth_sum: u64,
    chan_depth_n: u64,
    chan_drops: u64,
    last_log: Option<Instant>,
}

impl ProducerMetrics {
    pub const fn new() -> Self {
        Self {
            arrival: Hist::new(),
            last_arrival: None,
            hold: Hist::new(),
            egl: Hist::new(),
            cuda_reg: Hist::new(),
            copy: Hist::new(),
            cuda_total: Hist::new(),
            chan_depth_max: 0,
            chan_depth_sum: 0,
            chan_depth_n: 0,
            chan_drops: 0,
            last_log: None,
        }
    }

    /// Record one frame arriving at `.process()` (Point A interval).
    pub fn record_arrival(&mut self) {
        let now = Instant::now();
        if let Some(last) = self.last_arrival {
            self.arrival.record(now.duration_since(last).as_micros() as u32);
        }
        self.last_arrival = Some(now);
    }

    pub fn record_hold(&mut self, us: u32) {
        self.hold.record(us);
    }

    pub fn record_cuda(&mut self, egl_us: u32, reg_us: u32, copy_us: u32, total_us: u32) {
        self.egl.record(egl_us);
        self.cuda_reg.record(reg_us);
        self.copy.record(copy_us);
        self.cuda_total.record(total_us);
    }

    /// Record the channel state at send time. `dropped` = try_send hit a full
    /// channel and the frame was discarded.
    pub fn record_chan(&mut self, depth: usize, dropped: bool) {
        self.chan_depth_max = self.chan_depth_max.max(depth);
        self.chan_depth_sum += depth as u64;
        self.chan_depth_n += 1;
        if dropped {
            self.chan_drops += 1;
        }
    }

    /// Log + reset if the interval elapsed. Cheap to call every frame.
    pub fn maybe_log(&mut self) {
        let now = Instant::now();
        let due = match self.last_log {
            Some(t) => now.duration_since(t).as_secs() >= LOG_INTERVAL_SECS,
            None => true,
        };
        if !due {
            return;
        }
        self.last_log = Some(now);

        let avg_depth = if self.chan_depth_n > 0 {
            self.chan_depth_sum as f64 / self.chan_depth_n as f64
        } else {
            0.0
        };
        eprintln!(
            "[METRIC] A.arrival    {}",
            self.arrival.fmt_us()
        );
        eprintln!("[METRIC] hold_buf     {}", self.hold.fmt_us());
        eprintln!("[METRIC] cuda_total   {}", self.cuda_total.fmt_us());
        eprintln!("[METRIC] cuda.egl     {}", self.egl.fmt_us());
        eprintln!("[METRIC] cuda.reg     {}", self.cuda_reg.fmt_us());
        eprintln!("[METRIC] cuda.copy    {}", self.copy.fmt_us());
        eprintln!(
            "[METRIC] chan(8)      depth_avg={:.2} depth_max={} drops={}",
            avg_depth, self.chan_depth_max, self.chan_drops
        );

        self.arrival.clear();
        self.hold.clear();
        self.egl.clear();
        self.cuda_reg.clear();
        self.copy.clear();
        self.cuda_total.clear();
        self.chan_depth_max = 0;
        self.chan_depth_sum = 0;
        self.chan_depth_n = 0;
        // chan_drops is cumulative on purpose (rare event; keep running total)
    }
}

/// Point B: inter-arrival of frames at the GStreamer `.create()` pull, i.e.
/// after the bounded(8) channel. Compare against Point A: if A is smooth and
/// B bursts, the burst is born in the channel/encoder coupling.
pub struct CreateMetrics {
    interval: Hist,
    last: Option<Instant>,
    last_log: Option<Instant>,
}

impl CreateMetrics {
    pub const fn new() -> Self {
        Self { interval: Hist::new(), last: None, last_log: None }
    }

    pub fn tick(&mut self) {
        let now = Instant::now();
        if let Some(last) = self.last {
            self.interval.record(now.duration_since(last).as_micros() as u32);
        }
        self.last = Some(now);

        let due = match self.last_log {
            Some(t) => now.duration_since(t).as_secs() >= LOG_INTERVAL_SECS,
            None => true,
        };
        if due {
            self.last_log = Some(now);
            eprintln!("[METRIC] B.create     {}", self.interval.fmt_us());
            self.interval.clear();
        }
    }
}

pub static PRODUCER: Mutex<ProducerMetrics> = Mutex::new(ProducerMetrics::new());
pub static CREATE: Mutex<CreateMetrics> = Mutex::new(CreateMetrics::new());
