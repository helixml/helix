package hydra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStartServiceLogTailers_AppendsToBufferWithPrefix verifies the happy
// path: a wrapper writes lines to /var/log/helix-services/<svc>.log,
// hydra's tailer picks them up, and they appear in the LogBuffer with the
// "[<svc>] " prefix the admin UI consumes. ALSO verifies that lines
// written BEFORE the tailer starts are still captured (review #4: the
// cont-init.d wrappers run dockerd/heartbeat before hydra, so their
// startup output is already on disk when the tailer opens the file).
func TestStartServiceLogTailers_AppendsToBufferWithPrefix(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "compose-manager.log")

	// Pre-existing content written BEFORE the tailer starts. The tailer
	// must replay this from the start of the file - it represents the
	// boot-time output the operator most cares about for T-10-style debug.
	preLines := []string{
		"pre-tailer line 1: dockerd starting at t=0",
		"pre-tailer line 2: NVIDIA runtime selected",
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(preLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	buf := NewLogBuffer(64)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartServiceLogTailers(ctx, buf, dir)

	// Give the tailer a moment to open + read existing content.
	time.Sleep(250 * time.Millisecond)

	// Now append more lines like a live producer would.
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()

	liveLines := []string{
		"live line 1: compose-manager polling",
		"live line 2: assignment fetched",
		"live line 3 with UTF-8 char é",
	}
	for _, line := range liveLines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	wantTotal := len(preLines) + len(liveLines)
	deadline := time.Now().Add(3 * time.Second)
	var snap []LogLine
	for time.Now().Before(deadline) {
		snap = buf.Snapshot(1024)
		if len(snap) >= wantTotal {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(snap) < wantTotal {
		t.Fatalf("expected at least %d lines in buffer (pre %d + live %d), got %d: %+v",
			wantTotal, len(preLines), len(liveLines), len(snap), snap)
	}

	allWant := append(append([]string{}, preLines...), liveLines...)
	prefix := "[compose-manager] "
	for i, want := range allWant {
		got := snap[i].Line
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("line %d: expected prefix %q, got %q", i, prefix, got)
		}
		if !strings.Contains(got, want) {
			t.Errorf("line %d: expected to contain %q, got %q", i, want, got)
		}
	}
}

// TestTailServiceLog_PartialLinesAreBufferedAcrossPolls verifies the
// review finding fix: a producer writing "foo " (no newline) followed
// by "bar\n" later should produce ONE LogBuffer entry "foo bar", not
// two fragments. The previous version of the tailer shipped the
// partial on the first tick (bufio.ReadString returns content+EOF for
// a partial), then shipped the rest on the next tick.
func TestTailServiceLog_PartialLinesAreBufferedAcrossPolls(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "compose-manager.log")

	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatalf("seed empty file: %v", err)
	}

	buf := NewLogBuffer(32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartServiceLogTailers(ctx, buf, dir)
	time.Sleep(250 * time.Millisecond)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()

	// First half of the line - NO trailing newline. Producer hasn't
	// flushed yet (think a docker pull progress bar mid-write).
	if _, err := fmt.Fprint(f, "pulling layer abc... "); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	f.Sync()

	// Let the tailer poll once (it will see the partial and buffer it).
	time.Sleep(1 * time.Second)

	// Second half + newline.
	if _, err := fmt.Fprintln(f, "done in 42ms"); err != nil {
		t.Fatalf("write completion: %v", err)
	}
	f.Sync()

	// Wait for the combined line to land in the buffer.
	deadline := time.Now().Add(3 * time.Second)
	var snap []LogLine
	for time.Now().Before(deadline) {
		snap = buf.Snapshot(1024)
		if len(snap) >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(snap) != 1 {
		t.Fatalf("expected exactly 1 buffered line, got %d: %+v", len(snap), snap)
	}
	wantLine := "[compose-manager] pulling layer abc... done in 42ms"
	if snap[0].Line != wantLine {
		t.Fatalf("expected %q, got %q", wantLine, snap[0].Line)
	}
}

// TestStartServiceLogTailers_PicksUpFileAppearingAfterStart verifies the
// rescan path: a wrapper that starts AFTER hydra (e.g. compose-manager
// in 12-start-... when hydra was in 10-start-...) creates its log file
// later in the boot, and the tailer must notice it within the rescan
// interval.
func TestStartServiceLogTailers_PicksUpFileAppearingAfterStart(t *testing.T) {
	dir := t.TempDir()

	buf := NewLogBuffer(64)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartServiceLogTailers(ctx, buf, dir)

	// No log file exists yet. Wait briefly so the initial scan completes
	// without finding anything.
	time.Sleep(250 * time.Millisecond)

	// Now create the file. Rescan interval is 10s; the test waits up to
	// 15s for the new file to be noticed. CI-friendly but slow if the
	// rescan path is broken.
	logPath := filepath.Join(dir, "inference-proxy.log")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create late log file: %v", err)
	}
	defer f.Close()

	want := "late-arriving service line"
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := fmt.Fprintln(f, want); err != nil {
			t.Fatalf("write: %v", err)
		}
		_ = f.Sync()
		snap := buf.Snapshot(1024)
		for _, l := range snap {
			if strings.Contains(l.Line, want) && strings.HasPrefix(l.Line, "[inference-proxy] ") {
				return // pass
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("expected line with prefix [inference-proxy] not found in buffer; snapshot: %+v", buf.Snapshot(1024))
}

// TestStartServiceLogTailers_NilBufferIsNoOp guards against a panic if a
// caller forgets to construct the buffer before calling the tailer.
// hydra/main.go always provides one but defensive.
func TestStartServiceLogTailers_NilBufferIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Must not panic.
	StartServiceLogTailers(ctx, nil, t.TempDir())
}
