package sessionresources

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func sample(session string, at time.Time, cpuUsec uint64) types.SessionResourceSample {
	return types.SessionResourceSample{
		SessionID: session, Timestamp: at.UnixMilli(), CPUUsageUsec: cpuUsec, MemoryBytes: 1 << 30,
		Filesystems: []types.SessionFilesystemSample{{Paths: "/var/lib/docker", Dedicated: true, SizeBytes: 100, UsedBytes: 40}},
	}
}

func TestStoreBuffersFlushesAndCompacts(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "node-a")
	start := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	s.Record([]types.SessionResourceSample{sample("ses_a", start, 0), sample("ses_b", start, 0)})
	got, err := s.Read("ses_a", start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil || len(got) != 1 {
		t.Fatalf("unflushed read = %v, %v", got, err)
	}

	for i := 1; i <= compactAt+2; i++ {
		if err := s.Flush(start); err != nil {
			t.Fatal(err)
		}
		s.Record([]types.SessionResourceSample{sample("ses_a", start.Add(time.Duration(i)*30*time.Second), uint64(i)*1e6)})
	}
	// Last sample stays buffered; the read merges it with the files.
	got, err = s.Read("ses_a", start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != compactAt+3 || got[0].Filesystems[0].UsedBytes != 40 {
		t.Fatalf("got %d samples, want %d: first %+v", len(got), compactAt+3, got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp <= got[i-1].Timestamp {
			t.Fatalf("samples not ordered at %d", i)
		}
	}
	files, err := chunks(filepath.Join(dir, "2026-11-18", "ses_a"))
	if err != nil || len(files) >= compactAt {
		t.Fatalf("compaction left %d files: %v", len(files), err)
	}
	if b, _ := s.Read("ses_b", start, start); len(b) != 1 {
		t.Fatalf("other session = %v", b)
	}
}

func TestStoreDropsDuplicatesFromInterruptedCompaction(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "node-a")
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	s.Record([]types.SessionResourceSample{sample("ses_a", at, 5)})
	if err := s.Flush(at); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(dir, "2026-11-18", "ses_a")
	files, _ := chunks(sessionDir)
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "copy-merged.parquet"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Read("ses_a", at, at); err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestStorePrunesExpiredDays(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "node-a")
	old := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	s.Record([]types.SessionResourceSample{sample("ses_a", old, 1)})
	if err := s.Flush(old); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(old.AddDate(0, 0, Days+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-10-01")); !os.IsNotExist(err) {
		t.Fatalf("expired day kept: %v", err)
	}
}

func TestStoreRejectsPathLikeSessionIDs(t *testing.T) {
	s := New(t.TempDir(), "node-a")
	for _, id := range []string{"../ses_a", "ses_a/../../x", "", "sbx_1"} {
		if _, err := s.Read(id, time.Now(), time.Now()); err == nil {
			t.Fatalf("Read(%q) accepted", id)
		}
	}
}

func TestSummarize(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	mk := func(sec int, cpu, throttled, ioPressure, write, oom uint64) types.SessionResourceSample {
		return types.SessionResourceSample{
			SessionID: "ses_a", Timestamp: at.Add(time.Duration(sec) * time.Second).UnixMilli(),
			CPUUsageUsec: cpu, CPUThrottledUsec: throttled, IOPressureUsec: ioPressure, IOWriteBytes: write, OOMKills: oom,
			CPULimitMillicores: 2000, MemoryLimitBytes: 4 << 30, MemoryBytes: uint64(sec) << 20, MemoryPeakBytes: 3 << 30,
		}
	}
	samples := []types.SessionResourceSample{
		mk(0, 0, 0, 0, 0, 0),
		mk(30, 30e6, 3e6, 15e6, 30e6, 0), // 1 core, 10% throttled, 50% IO stall, 1 MB/s
		mk(60, 90e6, 3e6, 15e6, 30e6, 1), // 2 cores, one OOM kill
		mk(90, 5e6, 0, 0, 0, 0),          // container restarted: counters reset
		mk(120, 5e6, 0, 0, 0, 0),         // idle
	}
	r := Summarize("ses_a", samples, 2)
	s := r.Summary
	if s.Samples != 5 || s.CPULimitCores != 2 || s.MemoryLimitBytes != 4<<30 || s.MemoryPeakBytes != 3<<30 || s.MemoryMaxBytes != 120<<20 {
		t.Fatalf("limits/peaks: %+v", s)
	}
	if s.CPUCoresMax != 2 || s.CPUCoresP50 != 1 || s.OOMKills != 1 || s.IOWriteBytes != 30e6 {
		t.Fatalf("totals: %+v", s)
	}
	// 3s throttled and 15s IO stalled over the 90s covered by valid intervals.
	if s.CPUThrottledPct < 3.33 || s.CPUThrottledPct > 3.34 || s.IOPressurePct < 16.66 || s.IOPressurePct > 16.67 {
		t.Fatalf("percentages: %+v", s)
	}
	if len(r.Points) != 2 {
		t.Fatalf("points: %+v", r.Points)
	}
	if p := r.Points[0]; p.CPUCores != 1.5 || p.IOWriteBps != 0.5e6 || p.OOMKills != 1 || p.MemoryBytes != 60<<20 {
		t.Fatalf("first point: %+v", p)
	}
	if p := r.Points[1]; p.CPUCores != 0 || p.Timestamp != samples[4].Timestamp {
		t.Fatalf("second point: %+v", p)
	}
}
