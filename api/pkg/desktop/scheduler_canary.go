package desktop

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	canaryTickInterval = 16667 * time.Microsecond
	canarySampleCount  = 300
)

// SchedulerStats reports the observed scheduling jitter of the in-process
// 60Hz canary, used as a proxy for how well the kernel scheduler is treating
// userspace tasks in this container.
type SchedulerStats struct {
	P50Ms   uint16
	P99Ms   uint16
	MaxMs   uint16
	Samples int
}

// SchedulerCanary measures kernel-scheduler-induced jitter by waking on a
// fixed 60Hz cadence and recording how much later than the target wake-up it
// actually ran. The deviation captures CPU contention and preemption from
// co-tenant work — surfaced in Stats for Nerds as evidence before reaching
// for chrt/cgroups.
type SchedulerCanary struct {
	mu      sync.Mutex
	samples []time.Duration
	idx     int
	filled  bool
}

func NewSchedulerCanary() *SchedulerCanary {
	return &SchedulerCanary{
		samples: make([]time.Duration, canarySampleCount),
	}
}

// Start launches the canary goroutine; it exits when ctx is cancelled.
func (c *SchedulerCanary) Start(ctx context.Context) {
	go c.run(ctx)
}

func (c *SchedulerCanary) run(ctx context.Context) {
	ticker := time.NewTicker(canaryTickInterval)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			lateness := now.Sub(last) - canaryTickInterval
			if lateness < 0 {
				lateness = 0
			}
			c.record(lateness)
			last = now
		}
	}
}

func (c *SchedulerCanary) record(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples[c.idx] = d
	c.idx = (c.idx + 1) % len(c.samples)
	if c.idx == 0 {
		c.filled = true
	}
}

// Stats returns p50/p99/max lateness over the rolling window.
func (c *SchedulerCanary) Stats() SchedulerStats {
	c.mu.Lock()
	n := len(c.samples)
	if !c.filled {
		n = c.idx
	}
	if n == 0 {
		c.mu.Unlock()
		return SchedulerStats{}
	}
	cp := make([]time.Duration, n)
	copy(cp, c.samples[:n])
	c.mu.Unlock()

	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })

	idx := func(pct int) int {
		i := n * pct / 100
		if i >= n {
			i = n - 1
		}
		return i
	}

	return SchedulerStats{
		P50Ms:   toUint16Ms(cp[idx(50)]),
		P99Ms:   toUint16Ms(cp[idx(99)]),
		MaxMs:   toUint16Ms(cp[n-1]),
		Samples: n,
	}
}

func toUint16Ms(d time.Duration) uint16 {
	ms := d / time.Millisecond
	if ms < 0 {
		return 0
	}
	if ms > 65535 {
		return 65535
	}
	return uint16(ms)
}
