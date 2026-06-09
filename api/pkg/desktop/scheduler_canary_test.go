package desktop

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerCanaryEmpty(t *testing.T) {
	c := NewSchedulerCanary()
	s := c.Stats()
	if s.Samples != 0 || s.P50Ms != 0 || s.P99Ms != 0 || s.MaxMs != 0 {
		t.Fatalf("expected zero stats on empty canary, got %+v", s)
	}
}

func TestSchedulerCanaryPercentiles(t *testing.T) {
	c := NewSchedulerCanary()
	for i := 0; i < 100; i++ {
		c.record(time.Duration(i) * time.Millisecond)
	}
	s := c.Stats()
	if s.Samples != 100 {
		t.Errorf("expected 100 samples, got %d", s.Samples)
	}
	if s.MaxMs != 99 {
		t.Errorf("expected max=99ms, got %d", s.MaxMs)
	}
	if s.P50Ms != 50 {
		t.Errorf("expected p50=50ms, got %d", s.P50Ms)
	}
	if s.P99Ms != 99 {
		t.Errorf("expected p99=99ms, got %d", s.P99Ms)
	}
}

func TestSchedulerCanaryRingBufferEviction(t *testing.T) {
	c := NewSchedulerCanary()
	for i := 0; i < canarySampleCount; i++ {
		c.record(0)
	}
	for i := 0; i < 10; i++ {
		c.record(time.Duration(100+i) * time.Millisecond)
	}
	s := c.Stats()
	if s.Samples != canarySampleCount {
		t.Errorf("expected %d samples after fill, got %d", canarySampleCount, s.Samples)
	}
	if s.MaxMs < 100 {
		t.Errorf("expected max >= 100ms after injecting large samples, got %d", s.MaxMs)
	}
}

func TestSchedulerCanaryUint16Clamp(t *testing.T) {
	c := NewSchedulerCanary()
	c.record(70 * time.Second) // far exceeds uint16 ms range
	s := c.Stats()
	if s.MaxMs != 65535 {
		t.Errorf("expected max clamped to 65535ms, got %d", s.MaxMs)
	}
}

func TestSchedulerCanaryStartStop(t *testing.T) {
	c := NewSchedulerCanary()
	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	cancel()
	s := c.Stats()
	if s.Samples == 0 {
		t.Errorf("expected some samples after 120ms of running canary, got 0")
	}
	// Give the goroutine a moment to exit and confirm no panic.
	time.Sleep(20 * time.Millisecond)
}
